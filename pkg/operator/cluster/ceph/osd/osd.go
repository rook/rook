/*
Copyright 2016 The Rook Authors. All rights reserved.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

	http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

// Package osd for the Ceph OSDs.
package osd

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/coreos/pkg/capnslog"
	rookalpha "github.com/rook/rook/pkg/apis/rook.io/v1alpha1"
	"github.com/rook/rook/pkg/clusterd"
	"github.com/rook/rook/pkg/daemon/ceph/client"
	"github.com/rook/rook/pkg/operator/cluster/ceph/osd/config"
	"github.com/rook/rook/pkg/operator/k8sutil"
	"github.com/rook/rook/pkg/util/display"
	"k8s.io/api/core/v1"
	extensions "k8s.io/api/extensions/v1beta1"
	"k8s.io/api/rbac/v1beta1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/client-go/kubernetes"
	"k8s.io/kubernetes/pkg/api"
	"k8s.io/kubernetes/pkg/kubelet/apis"
)

var logger = capnslog.NewPackageLogger("github.com/rook/rook", "op-osd")

const (
	OrchestrationStatusMapName       = "rook-ceph-osd-orchestration-status"
	OrchestrationStatusStarting      = "starting"
	OrchestrationStatusComputingDiff = "computingDiff"
	OrchestrationStatusOrchestrating = "orchestrating"
	OrchestrationStatusCompleted     = "completed"
	OrchestrationStatusFailed        = "failed"
	appName                          = "rook-ceph-osd"
	appNameFmt                       = "rook-ceph-osd-%s"
	clusterAvailableSpaceReserve     = 0.05
)

var clusterAccessRules = []v1beta1.PolicyRule{
	{
		APIGroups: []string{""},
		Resources: []string{"configmaps"},
		Verbs:     []string{"get", "list", "watch", "create", "update", "delete"},
	},
}

// Cluster keeps track of the OSDs
type Cluster struct {
	context         *clusterd.Context
	Namespace       string
	placement       rookalpha.Placement
	Keyring         string
	Version         string
	Storage         rookalpha.StorageSpec
	dataDirHostPath string
	HostNetwork     bool
	resources       v1.ResourceRequirements
	ownerRef        metav1.OwnerReference
}

// New creates an instance of the OSD manager
func New(context *clusterd.Context, namespace, version string, storageSpec rookalpha.StorageSpec, dataDirHostPath string,
	placement rookalpha.Placement, hostNetwork bool, resources v1.ResourceRequirements, ownerRef metav1.OwnerReference) *Cluster {

	return &Cluster{
		context:         context,
		Namespace:       namespace,
		placement:       placement,
		Version:         version,
		Storage:         storageSpec,
		dataDirHostPath: dataDirHostPath,
		HostNetwork:     hostNetwork,
		resources:       resources,
		ownerRef:        ownerRef,
	}
}

type OrchestrationStatus struct {
	Status  string `json:"status"`
	Message string `json:"message"`
}

// Start the osd management
func (c *Cluster) Start() error {
	logger.Infof("start running osds in namespace %s", c.Namespace)

	// create the artifacts for the osd to work with RBAC enabled
	err := k8sutil.MakeRole(c.context.Clientset, c.Namespace, appName, clusterAccessRules, c.ownerRef)
	if err != nil {
		logger.Warningf("failed to init RBAC for OSDs. %+v", err)
	}

	if c.Storage.UseAllNodes == false && len(c.Storage.Nodes) == 0 {
		logger.Warningf("useAllNodes is set to false and no nodes are specified, no OSD pods are going to be created")
	}

	// ensure the orchestration status map is created
	if err := makeOrchestrationStatusMap(c.context.Clientset, c.Namespace); err != nil {
		return fmt.Errorf("failed to make OSD orchestration status config map: %+v", err)
	}

	// disable scrubbing during orchestration and ensure it gets enabled again afterwards
	if o, err := client.DisableScrubbing(c.context, c.Namespace); err != nil {
		logger.Warningf("failed to disable scrubbing: %+v. %s", err, o)
	}
	defer func() {
		if o, err := client.EnableScrubbing(c.context, c.Namespace); err != nil {
			logger.Warningf("failed to enable scrubbing: %+v. %s", err, o)
		}
	}()

	if c.Storage.UseAllNodes {
		// make a daemonset for all nodes in the cluster
		ds := c.makeDaemonSet(c.Storage.Selection, c.Storage.Config)
		_, err := c.context.Clientset.Extensions().DaemonSets(c.Namespace).Create(ds)
		if err != nil {
			if !errors.IsAlreadyExists(err) {
				return fmt.Errorf("failed to create osd daemon set. %+v", err)
			}
			logger.Infof("osd daemon set already exists")
		} else {
			logger.Infof("osd daemon set started")
		}

		return nil
	}

	// orchestrate individual nodes, starting with any that are still ongoing (in the case that we
	// are resuming a previous orchestration attempt)
	if inProgressNode, status := c.findInProgressNode(); inProgressNode != "" {
		logger.Infof("resuming orchestration of in progress node %s, status: %+v", inProgressNode, status)
		if err := c.waitForCompletion(inProgressNode); err != nil {
			logger.Warningf("failed waiting for in progress node %s, will continue with orchestration.  %+v", inProgressNode, err)
		}
	}

	errorMessages := make([]string, 0)

	// start with nodes currently in the storage spec
	for i := range c.Storage.Nodes {
		// fully resolve the storage config and resources for this node
		n := c.Storage.ResolveNode(c.Storage.Nodes[i].Name)
		resources := k8sutil.MergeResourceRequirements(c.Storage.Nodes[i].Resources, c.resources)

		// update the orchestration status of this node to the starting state
		status := OrchestrationStatus{Status: OrchestrationStatusStarting}
		if err := UpdateOrchestrationStatusMap(c.context.Clientset, c.Namespace, n.Name, status); err != nil {
			errorMessages = append(errorMessages, fmt.Sprintf("failed to set orchestration starting status for node %s: %+v", n.Name, err))
			continue
		}

		// create the deployment that will run the OSDs for this node
		rs := c.makeDeployment(n.Name, n.Devices, n.Selection, resources, n.Config)
		_, err := c.context.Clientset.Extensions().Deployments(c.Namespace).Create(rs)
		if err != nil {
			if !errors.IsAlreadyExists(err) {
				// we failed to create the deployment, update the orchestration status for this node
				message := fmt.Sprintf("failed to create osd deployment for node %s. %+v", n.Name, err)
				c.handleOrchestrationFailure(*n, message, &errorMessages)
				continue
			} else {
				// TODO: if the deployment set already exists, we may need to edit the pod template spec, for example if device filter has changed
				message := fmt.Sprintf("osd deploymental ready exists for node %s", n.Name)
				logger.Info(message)
				status := OrchestrationStatus{Status: OrchestrationStatusCompleted, Message: message}
				if err := UpdateOrchestrationStatusMap(c.context.Clientset, c.Namespace, n.Name, status); err != nil {
					errorMessages = append(errorMessages, fmt.Sprintf("failed to set orchestration status for node %s, status: %+v: %+v", n.Name, status, err))
					continue
				}
			}
		} else {
			logger.Infof("osd deployment started for node %s", n.Name)
		}

		// wait for the current node's orchestration to be completed
		if err := c.waitForCompletion(n.Name); err != nil {
			errorMessages = append(errorMessages, err.Error())
			continue
		}
	}

	// find all removed nodes (if any) and start orchestration to remove them from the cluster
	removedNodes, err := c.findRemovedNodes()
	if err != nil {
		return fmt.Errorf("failed to find removed nodes: %+v", err)
	}

	for i := range removedNodes {
		n := removedNodes[i]
		if err := c.isSafeToRemoveNode(n); err != nil {
			message := fmt.Sprintf("skipping the removal of node %s because it is not safe to do so: %+v", n.Name, err)
			c.handleOrchestrationFailure(n, message, &errorMessages)
			continue
		}

		logger.Infof("removing node %s from the cluster", n.Name)

		// update the orchestration status of this removed node to the starting state
		if err := UpdateOrchestrationStatusMap(c.context.Clientset, c.Namespace, n.Name, OrchestrationStatus{Status: OrchestrationStatusStarting}); err != nil {
			errorMessages = append(errorMessages, fmt.Sprintf("failed to set orchestration starting status for removed node %s: %+v", n.Name, err))
			continue
		}

		// trigger orchestration on the removed node by telling it not to use any storage at all.  note that the directories are still passed in
		// so that the pod will be able to mount them and migrate data from them.
		d := c.makeDeployment(n.Name, nil, rookalpha.Selection{DeviceFilter: "none", Directories: n.Directories}, v1.ResourceRequirements{}, rookalpha.Config{})
		d, err := c.context.Clientset.Extensions().Deployments(c.Namespace).Update(d)
		if err != nil {
			message := fmt.Sprintf("failed to update osd deployment for removed node %s. %+v", n.Name, err)
			c.handleOrchestrationFailure(n, message, &errorMessages)
			continue
		} else {
			logger.Infof("osd deployment updated for node %s", n.Name)
		}

		// delete the replicaset associated with the deployment so that it will be restarted with the new template
		if err := c.deleteOSDReplicaSet(d); err != nil {
			message := fmt.Sprintf("failed to find and delete OSD replicaset for deployment %s. %+v", d.Name, err)
			c.handleOrchestrationFailure(n, message, &errorMessages)
			continue
		}

		// wait for the removed node's orchestration to be completed
		if err := c.waitForCompletion(n.Name); err != nil {
			errorMessages = append(errorMessages, err.Error())
			continue
		}

		// orchestration of the removed node completed, we can delete the deployment now
		if err := c.context.Clientset.Extensions().Deployments(c.Namespace).Delete(d.Name, &metav1.DeleteOptions{}); err != nil {
			errorMessages = append(errorMessages, fmt.Sprintf("failed to delete deployment %s: %+v", d.Name, err))
			continue
		}
	}

	if len(errorMessages) == 0 {
		logger.Infof("completed running osds in namespace %s", c.Namespace)
		return nil
	}

	return fmt.Errorf("%d failures encountered while running osds in namespace %s: %+v",
		len(errorMessages), c.Namespace, strings.Join(errorMessages, "\n"))
}

func UpdateOrchestrationStatusMap(clientset kubernetes.Interface, namespace string, node string, status OrchestrationStatus) error {
	cm, err := clientset.CoreV1().ConfigMaps(namespace).Get(OrchestrationStatusMapName, metav1.GetOptions{})
	if err != nil {
		if !errors.IsNotFound(err) {
			return err
		}

		// the status map doesn't exist yet, make it now
		if err := makeOrchestrationStatusMap(clientset, namespace); err != nil {
			return err
		}

		// refresh our local copy of the status map
		cm, err = clientset.CoreV1().ConfigMaps(namespace).Get(OrchestrationStatusMapName, metav1.GetOptions{})
		if err != nil {
			return err
		}
	}

	if cm.Data == nil {
		cm.Data = make(map[string]string)
	}

	// update the status map with the given status now
	s, _ := json.Marshal(status)
	cm.Data[node] = string(s)
	cm, err = clientset.CoreV1().ConfigMaps(namespace).Update(cm)
	if err != nil {
		return fmt.Errorf("failed to update OSD orchestration status for node %s, status %+v.  %+v", node, status, err)
	}

	return nil
}

func makeOrchestrationStatusMap(clientset kubernetes.Interface, namespace string) error {
	cm, err := clientset.CoreV1().ConfigMaps(namespace).Get(OrchestrationStatusMapName, metav1.GetOptions{})
	if err != nil {
		if !errors.IsNotFound(err) {
			return err
		}

		// the orchestration status map doesn't exist yet, create it now
		cm = &v1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{
				Name:      OrchestrationStatusMapName,
				Namespace: namespace,
			},
			Data: make(map[string]string),
		}

		cm, err = clientset.CoreV1().ConfigMaps(namespace).Create(cm)
		if err != nil {
			return fmt.Errorf("failed to create OSD orchestration status map %s: %+v", OrchestrationStatusMapName, err)
		}
	}

	return nil
}

func (c *Cluster) handleOrchestrationFailure(n rookalpha.Node, message string, errorMessages *[]string) {
	logger.Warning(message)
	status := OrchestrationStatus{Status: OrchestrationStatusFailed, Message: message}
	UpdateOrchestrationStatusMap(c.context.Clientset, c.Namespace, n.Name, status)
	*errorMessages = append(*errorMessages, message)
}

func isStatusCompleted(status OrchestrationStatus) bool {
	return status.Status == OrchestrationStatusCompleted || status.Status == OrchestrationStatusFailed
}

func parseOrchestrationStatus(data map[string]string, node string) *OrchestrationStatus {
	if data == nil {
		return nil
	}

	statusRaw, ok := data[node]
	if !ok {
		return nil
	}

	// we have status for this node, unmarshal it
	var status OrchestrationStatus
	if err := json.Unmarshal([]byte(statusRaw), &status); err != nil {
		logger.Warningf("failed to unmarshal orchestration status for node %s. status: %s. %+v", node, statusRaw, err)
		return nil
	}

	return &status
}

func (c *Cluster) findInProgressNode() (string, *OrchestrationStatus) {
	cm, err := c.context.Clientset.CoreV1().ConfigMaps(c.Namespace).Get(OrchestrationStatusMapName, metav1.GetOptions{})
	if err != nil {
		return "", nil
	}

	if len(cm.Data) == 0 {
		// no orchestration status available, no in progress node
		return "", nil
	}

	for node, statusRaw := range cm.Data {
		var status OrchestrationStatus
		if err := json.Unmarshal([]byte(statusRaw), &status); err != nil {
			logger.Warningf("failed to unmarshal orchestration status for node %s. status: %s. %+v", node, statusRaw, err)
			continue
		}

		if !isStatusCompleted(status) {
			// found an in progress node, return it
			return node, &status
		}
	}

	// didn't find any in progress nodes
	return "", nil
}

func (c *Cluster) waitForCompletion(node string) error {
	// check the status map to see if the node is already completed before we start watching
	cm, err := c.context.Clientset.CoreV1().ConfigMaps(c.Namespace).Get(OrchestrationStatusMapName, metav1.GetOptions{})
	if err != nil {
		if !errors.IsNotFound(err) {
			return err
		}
		// the status map doesn't exist yet, watching below is still an OK thing to do
	} else {
		// the status map exists, check the current value for the node to see if it's completed
		status := parseOrchestrationStatus(cm.Data, node)
		if status != nil {
			if status.Status == OrchestrationStatusCompleted {
				return nil
			} else if status.Status == OrchestrationStatusFailed {
				return fmt.Errorf("orchestration for node %s failed: %+v", node, status)
			}
		}
	}

	// start watching for changes on the orchestration status map
	var startingVersion string
	if cm != nil {
		startingVersion = cm.ResourceVersion
	}
	opts := metav1.ListOptions{
		FieldSelector:   fields.OneTermEqualSelector(api.ObjectNameField, OrchestrationStatusMapName).String(),
		ResourceVersion: startingVersion,
	}

	for {
		w, err := c.context.Clientset.CoreV1().ConfigMaps(c.Namespace).Watch(opts)
		if err != nil {
			return fmt.Errorf("failed to start watch on %s: %+v", OrchestrationStatusMapName, err)
		}
		defer w.Stop()

		for {
			select {
			case e, ok := <-w.ResultChan():
				if !ok {
					logger.Warning("orchestration status config map result channel closed, restarting watch.")
					<-time.After(100 * time.Millisecond)
					w.Stop()
					break
				}

				if e.Type == watch.Modified {
					statusMap := e.Object.(*v1.ConfigMap)
					status := parseOrchestrationStatus(statusMap.Data, node)
					if status == nil {
						continue
					}

					if status.Status == OrchestrationStatusCompleted {
						return nil
					} else if status.Status == OrchestrationStatusFailed {
						return fmt.Errorf("orchestration for node %s failed: %+v", node, status)
					}
				}

			case <-time.After(time.Minute):
				// log every so often while we are waiting
				logger.Infof("waiting on orchestration status update from node %s", node)
			}
		}
	}
}

func IsRemovingNode(devices string) bool {
	return devices == "none"
}

func (c *Cluster) findRemovedNodes() ([]rookalpha.Node, error) {
	var removedNodes []rookalpha.Node

	// first discover the storage nodes that are still running
	discoveredNodes, err := c.discoverStorageNodes()
	if err != nil {
		return nil, fmt.Errorf("failed to discover storage nodes: %+v", err)
	}

	for i, discoveredNode := range discoveredNodes {
		found := false
		for _, newNode := range c.Storage.Nodes {
			// discovered storage node still exists in the current storage spec, move on to next discovered node
			if discoveredNode.Name == newNode.Name {
				found = true
				break
			}
		}

		if !found {
			// the discovered storage node was not found in the current storage spec, add it to the removed nodes set
			removedNodes = append(removedNodes, discoveredNodes[i])
		}
	}

	return removedNodes, nil
}

func (c *Cluster) discoverStorageNodes() ([]rookalpha.Node, error) {
	var discoveredNodes []rookalpha.Node

	listOpts := metav1.ListOptions{LabelSelector: fmt.Sprintf("app=%s", appName)}
	osdDeployments, err := c.context.Clientset.Extensions().Deployments(c.Namespace).List(listOpts)
	if err != nil {
		return nil, fmt.Errorf("failed to list osd deployments: %+v", err)
	}

	discoveredNodes = make([]rookalpha.Node, len(osdDeployments.Items))
	for i, osdDeployment := range osdDeployments.Items {
		osdPodSpec := osdDeployment.Spec.Template.Spec

		// get the node name from the node selector
		nodeName, ok := osdPodSpec.NodeSelector[apis.LabelHostname]
		if !ok || nodeName == "" {
			return nil, fmt.Errorf("osd deployment %s doesn't have a node name on its node selector: %+v", osdDeployment.Name, osdPodSpec.NodeSelector)
		}

		// get the osd container, there should be exactly 1
		if len(osdPodSpec.Containers) != 1 {
			return nil, fmt.Errorf("osd pod spec should have exactly 1 container: %+v", osdPodSpec.Containers)
		}
		osdContainer := osdPodSpec.Containers[0]

		// populate the discovered node with the properties discovered from its running artifacts.
		// note that we are populating just a subset here, the minimum subset needed to be able
		// to remove the node if needed.  As we support updating more properties in the future (as
		// opposed to simply removing the whole node) we'll need to discover and populate them here too.
		node := rookalpha.Node{
			Name: nodeName,
			Selection: rookalpha.Selection{
				Directories: getDirectoriesFromContainer(osdContainer),
			},
		}

		discoveredNodes[i] = node
	}

	return discoveredNodes, nil
}

func (c *Cluster) isSafeToRemoveNode(node rookalpha.Node) error {
	if err := client.IsClusterClean(c.context, c.Namespace); err != nil {
		// the cluster isn't clean, it's not safe to remove this node
		return err
	}

	// get the current used space on all OSDs in the cluster
	currUsage, err := client.GetOSDUsage(c.context, c.Namespace)
	if err != nil {
		return err
	}

	// get the set of OSD IDs that are on the given node
	osdsForNode, err := c.getOSDsForNode(node)
	if err != nil {
		return err
	}

	// sum up the total OSD used space for the node by summing the used space of each OSD on the node
	nodeUsage := int64(0)
	for _, id := range osdsForNode {
		osdUsage := currUsage.ByID(id)
		if osdUsage != nil {
			osdKB, err := osdUsage.UsedKB.Int64()
			if err != nil {
				logger.Warningf("osd.%d has invalid usage %+v: %+v", id, osdUsage.UsedKB, err)
				continue
			}

			nodeUsage += osdKB * 1024
		}
	}

	// check to see if there is sufficient space left in the cluster to absorb all the migrated data from the node to be removed
	clusterUsage, err := client.Usage(c.context, c.Namespace)
	if err != nil {
		return err
	}
	clusterAvailableBytes, err := clusterUsage.Stats.TotalAvailBytes.Int64()
	if err != nil {
		return err
	}
	clusterTotalBytes, err := clusterUsage.Stats.TotalBytes.Int64()
	if err != nil {
		return err
	}

	if (clusterAvailableBytes - nodeUsage) < int64((float64(clusterTotalBytes) * clusterAvailableSpaceReserve)) {
		// the remaining available space in the cluster after the space that this node is using gets moved elsewhere
		// would be less than the cluster available space reserve, it's not safe to remove this node
		return fmt.Errorf("insufficient available space in the cluster to remove node %s. node usage: %s, cluster available: %s",
			node.Name, display.BytesToString(uint64(nodeUsage)), display.BytesToString(uint64(clusterAvailableBytes)))
	}

	// looks safe to remove the node
	return nil
}

func (c *Cluster) deleteOSDReplicaSet(d *extensions.Deployment) error {
	// list all OSD replicaset first
	opts := metav1.ListOptions{LabelSelector: fields.OneTermEqualSelector(k8sutil.AppAttr, appName).String()}
	replicaSets, err := c.context.Clientset.Extensions().ReplicaSets(c.Namespace).List(opts)
	if err != nil {
		return err
	}

	// iterate over all the OSD replicaSets, looking for a match to the given deployment and the replicaSet's owner
	var replicaSet *extensions.ReplicaSet
	for i := range replicaSets.Items {
		rs := &replicaSets.Items[i]
		for _, owner := range rs.OwnerReferences {
			if owner.Name == d.Name && owner.UID == d.UID {
				// the owner of this replica set matches the name and UID of the given deployment
				replicaSet = rs
				break
			}
		}

		if replicaSet != nil {
			break
		}
	}

	if replicaSet == nil {
		return fmt.Errorf("replicaSet for deployment %s not found", d.Name)
	}

	err = c.context.Clientset.Extensions().ReplicaSets(c.Namespace).Delete(replicaSet.Name, &metav1.DeleteOptions{})
	return err
}

func (c *Cluster) getOSDsForNode(node rookalpha.Node) ([]int, error) {
	kv := k8sutil.NewConfigMapKVStore(c.Namespace, c.context.Clientset, c.ownerRef)

	// load all the OSD dirs/devices for the given node
	dirMap, err := config.LoadOSDDirMap(kv, node.Name)
	if err != nil && !errors.IsNotFound(err) {
		return nil, err
	}
	scheme, err := config.LoadScheme(kv, config.GetConfigStoreName(node.Name))
	if err != nil && !errors.IsNotFound(err) {
		return nil, err
	}

	// loop through all the known OSDs for the node and collect their IDs into the result
	osdsForNode := make([]int, len(dirMap)+len(scheme.Entries))
	idNum := 0

	for _, id := range dirMap {
		osdsForNode[idNum] = id
		idNum++
	}
	for _, entry := range scheme.Entries {
		osdsForNode[idNum] = entry.ID
		idNum++
	}

	return osdsForNode, nil
}

func getDirectoriesFromContainer(osdContainer v1.Container) []rookalpha.Directory {
	var dirsArg string
	for _, envVar := range osdContainer.Env {
		if envVar.Name == dataDirsEnvVarName {
			dirsArg = envVar.Value
		}
	}

	var dirsList []string
	if dirsArg != "" {
		dirsList = strings.Split(dirsArg, ",")
	}

	dirs := make([]rookalpha.Directory, len(dirsList))
	for dirNum, dir := range dirsList {
		dirs[dirNum] = rookalpha.Directory{Path: dir}
	}

	return dirs
}
