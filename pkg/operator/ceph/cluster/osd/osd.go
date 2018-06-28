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
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/coreos/pkg/capnslog"
	rookalpha "github.com/rook/rook/pkg/apis/rook.io/v1alpha2"
	"github.com/rook/rook/pkg/clusterd"
	"github.com/rook/rook/pkg/daemon/ceph/client"
	osdconfig "github.com/rook/rook/pkg/operator/ceph/cluster/osd/config"
	"github.com/rook/rook/pkg/operator/discover"
	"github.com/rook/rook/pkg/operator/k8sutil"
	"github.com/rook/rook/pkg/util/display"
	"k8s.io/api/core/v1"
	extensions "k8s.io/api/extensions/v1beta1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/kubernetes/pkg/kubelet/apis"
)

var logger = capnslog.NewPackageLogger("github.com/rook/rook", "op-osd")

const (
	appName                      = "rook-ceph-osd"
	prepareAppName               = "rook-ceph-osd-prepare"
	prepareAppNameFmt            = "rook-ceph-osd-prepare-%s"
	osdAppNameFmt                = "rook-ceph-osd-id-%d"
	appNameFmt                   = "rook-ceph-osd-%s"
	osdLabelKey                  = "ceph-osd-id"
	clusterAvailableSpaceReserve = 0.05
	defaultServiceAccountName    = "rook-ceph-cluster"
)

// Cluster keeps track of the OSDs
type Cluster struct {
	context         *clusterd.Context
	Namespace       string
	placement       rookalpha.Placement
	Keyring         string
	Version         string
	Storage         rookalpha.StorageScopeSpec
	dataDirHostPath string
	HostNetwork     bool
	resources       v1.ResourceRequirements
	ownerRef        metav1.OwnerReference
	serviceAccount  string
	kv              *k8sutil.ConfigMapKVStore
}

// New creates an instance of the OSD manager
func New(context *clusterd.Context, namespace, version, serviceAccount string, storageSpec rookalpha.StorageScopeSpec,
	dataDirHostPath string, placement rookalpha.Placement, hostNetwork bool,
	resources v1.ResourceRequirements, ownerRef metav1.OwnerReference) *Cluster {

	if serviceAccount == "" {
		// if the service account was not set, make a best effort with the example service account name since the default is unlikely to be sufficient.
		serviceAccount = defaultServiceAccountName
		logger.Infof("setting the osd pods to use the service account name: %s", serviceAccount)
	}

	return &Cluster{
		context:         context,
		Namespace:       namespace,
		serviceAccount:  serviceAccount,
		placement:       placement,
		Version:         version,
		Storage:         storageSpec,
		dataDirHostPath: dataDirHostPath,
		HostNetwork:     hostNetwork,
		resources:       resources,
		ownerRef:        ownerRef,
		kv:              k8sutil.NewConfigMapKVStore(namespace, context.Clientset, ownerRef),
	}
}

type OSDInfo struct {
	ID             int    `json:"id"`
	DataPath       string `json:"data-path"`
	Config         string `json:"conf"`
	Cluster        string `json:"cluster"`
	KeyringPath    string `json:"keyring-path"`
	UUID           string `json:"uuid"`
	Journal        string `json:"journal"`
	IsFileStore    bool   `json:"is-file-store"`
	IsDirectory    bool   `json:"is-directory"`
	DevicePartUUID string `json:"device-part-uuid"`
}

type OrchestrationStatus struct {
	OSDs    []OSDInfo `json:"osds"`
	Status  string    `json:"status"`
	Message string    `json:"message"`
}

type deploymentPerNode struct {
	node        rookalpha.Node
	deployments []*extensions.Deployment
}

// Start the osd management
func (c *Cluster) Start() error {
	logger.Infof("start running osds in namespace %s", c.Namespace)

	if c.Storage.UseAllNodes == false && len(c.Storage.Nodes) == 0 {
		logger.Warningf("useAllNodes is set to false and no nodes are specified, no OSD pods are going to be created")
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
		// resolve all storage nodes
		c.Storage.Nodes = nil
		rookSystemNS := os.Getenv(k8sutil.PodNamespaceEnvVar)
		allNodeDevices, err := discover.ListDevices(c.context, rookSystemNS, "" /* all nodes */)
		if err != nil {
			logger.Warningf("failed to get storage nodes from namespace %s: %v", rookSystemNS, err)
			return err
		}
		for nodeName := range allNodeDevices {
			storageNode := rookalpha.Node{
				Name: nodeName,
			}
			c.Storage.Nodes = append(c.Storage.Nodes, storageNode)
		}
		logger.Debugf("storage nodes: %+v", c.Storage.Nodes)
	}

	// orchestrate individual nodes, starting with any that are still ongoing (in the case that we
	// are resuming a previous orchestration attempt)
	config := newProvisionConfig()
	logger.Infof("checking if orchestration is still in progress")
	c.completeOSDsForAllNodes(config)

	// start the jobs to provision the OSD devices and directories
	logger.Infof("start provisioning the osds on nodes, if needed")
	c.startProvisioning(config)

	// start the OSD pods, waiting for the provisioning to be completed
	logger.Infof("start osds after provisioning is completed, if needed")
	c.completeOSDsForAllNodes(config)

	// handle the removed nodes and rebalance the PGs
	logger.Infof("checking if any nodes were removed")
	c.handleRemovedNodes(config)

	if len(config.errorMessages) == 0 {
		logger.Infof("completed running osds in namespace %s", c.Namespace)
		// start osd health monitor
		mon := NewMonitor(c.context, c.Namespace)
		go mon.Run()
		return nil
	}

	return fmt.Errorf("%d failures encountered while running osds in namespace %s: %+v",
		len(config.errorMessages), c.Namespace, strings.Join(config.errorMessages, "\n"))
}

func (c *Cluster) startProvisioning(config *provisionConfig) {
	config.devicesToUse = make(map[string][]rookalpha.Device, len(c.Storage.Nodes))

	// start with nodes currently in the storage spec
	for _, node := range c.Storage.Nodes {
		// fully resolve the storage config and resources for this node
		n := c.resolveNode(node.Name)
		if n == nil {
			logger.Warningf("node %s did not resolve", node.Name)
			continue
		}

		storeConfig := osdconfig.ToStoreConfig(n.Config)
		metadataDevice := osdconfig.MetadataDevice(n.Config)
		if n.Name == "" {
			logger.Warningf("skipping node with a blank name! %+v", n)
			continue
		}

		// update the orchestration status of this node to the starting state
		status := OrchestrationStatus{Status: OrchestrationStatusStarting}
		if err := c.updateNodeStatus(n.Name, status); err != nil {
			config.addError("failed to set orchestration starting status for node %s: %+v", n.Name, err)
			continue
		}
		config.devicesToUse[n.Name] = n.Devices
		availDev, deviceErr := discover.GetAvailableDevices(c.context, n.Name, c.Namespace, n.Devices, n.Selection.DeviceFilter, n.Selection.GetUseAllDevices())
		if deviceErr != nil {
			logger.Warningf("failed to get devices for node %s cluster %s: %v", n.Name, c.Namespace, deviceErr)
		} else {
			config.devicesToUse[n.Name] = availDev
			logger.Infof("avail devices for node %s: %+v", n.Name, availDev)
		}
		if len(availDev) == 0 && len(c.dataDirHostPath) == 0 {
			config.addError("empty volumes for node %s", n.Name)
			continue
		}
		// create the job that prepares osds on the node
		job, err := c.makeJob(n.Name, config.devicesToUse[n.Name], n.Selection, n.Resources, storeConfig, metadataDevice, n.Location)
		if err != nil {
			message := fmt.Sprintf("failed to create prepare job node %s: %v", n.Name, err)
			config.addError(message)
			status := OrchestrationStatus{Status: OrchestrationStatusCompleted, Message: message}
			if err := c.updateNodeStatus(n.Name, status); err != nil {
				config.addError("failed to update node %s status. %+v", n.Name, err)
				continue
			}
		}

		// check if the job was already created and what its status is
		existingJob, err := c.context.Clientset.Batch().Jobs(c.Namespace).Get(job.Name, metav1.GetOptions{})
		if err != nil && !errors.IsNotFound(err) {
			config.addError("failed to detect provisioning job for node %s. %+v", n.Name, err)
		} else if err == nil {
			// delete the job that already exists from a previous run
			if existingJob.Status.Active > 0 {
				logger.Infof("Found previous provisioning job for node %s. Status=%+v", n.Name, existingJob.Status)
			} else {
				logger.Infof("Removing previous provisioning job for node %s to start a new one", n.Name)
				err := c.deleteBatchJob(existingJob.Name)
				if err != nil {
					logger.Warningf("failed to remove job %s. %+v", n.Name, err)
				}
			}
		}

		_, err = c.context.Clientset.Batch().Jobs(c.Namespace).Create(job)
		if err != nil {
			if !errors.IsAlreadyExists(err) {
				// we failed to create job, update the orchestration status for this node
				message := fmt.Sprintf("failed to create osd prepare job for node %s. %+v", n.Name, err)
				c.handleOrchestrationFailure(config, *n, message)
				err = discover.FreeDevices(c.context, n.Name, c.Namespace)
				if err != nil {
					logger.Warningf("failed to free devices: %s", err)
				}
				continue
			} else {
				// TODO: if the job already exists, we may need to edit the pod template spec, for example if device filter has changed
				message := fmt.Sprintf("provisioning job already exists for node %s", n.Name)
				config.addError(message)
				status := OrchestrationStatus{Status: OrchestrationStatusCompleted, Message: message}
				if err := c.updateNodeStatus(n.Name, status); err != nil {
					config.addError("failed to update status for node %s. %+v", n.Name, err)
					continue
				}
			}
		} else {
			logger.Infof("osd prepare job started for node %s", n.Name)
		}
	}
}

func (c *Cluster) deleteBatchJob(name string) error {
	propagation := metav1.DeletePropagationForeground
	gracePeriod := int64(0)
	options := &metav1.DeleteOptions{GracePeriodSeconds: &gracePeriod, PropagationPolicy: &propagation}

	if err := c.context.Clientset.Batch().Jobs(c.Namespace).Delete(name, options); err != nil {
		return fmt.Errorf("failed to remove previous provisioning job for node %s. %+v", name, err)
	}

	retries := 20
	sleepInterval := 2 * time.Second
	for i := 0; i < retries; i++ {
		_, err := c.context.Clientset.Batch().Jobs(c.Namespace).Get(name, metav1.GetOptions{})
		if err != nil && errors.IsNotFound(err) {
			logger.Infof("batch job %s deleted", name)
			return nil
		}

		logger.Infof("batch job %s still exists", name)
		time.Sleep(sleepInterval)
	}

	logger.Warningf("gave up waiting for batch job %s to be deleted", name)
	return nil
}

func (c *Cluster) startOSDDaemon(nodeName string, config *provisionConfig, configMap *v1.ConfigMap, status *OrchestrationStatus) bool {

	osds := status.OSDs
	logger.Infof("starting %d osd daemons on node %s: %+v", len(osds), nodeName, osds)

	// fully resolve the storage config and resources for this node
	n := c.resolveNode(nodeName)
	if n == nil {
		config.addError("node %s did not resolve to start osds", nodeName)
		return false
	}

	storeConfig := osdconfig.ToStoreConfig(n.Config)
	metadataDevice := osdconfig.MetadataDevice(n.Config)

	// start osds
	succeeded := 0
	for _, osd := range osds {
		logger.Debugf("start osd %v", osd)
		dp, err := c.makeDeployment(n.Name, config.devicesToUse[n.Name], n.Selection, n.Resources, storeConfig, metadataDevice, n.Location, osd)
		if err != nil {
			errMsg := fmt.Sprintf("nil deployment for node %s: %v", n.Name, err)
			config.addError(errMsg)
			err = discover.FreeDevices(c.context, n.Name, c.Namespace)
			if err != nil {
				logger.Warningf("failed to free devices: %s", err)
			}
			continue
		}
		dp, err = c.context.Clientset.Extensions().Deployments(c.Namespace).Create(dp)
		if err != nil {
			if !errors.IsAlreadyExists(err) {
				// we failed to create job, update the orchestration status for this node
				logger.Warningf("failed to create osd deployment for node %s, osd %v: %+v", n.Name, osd, err)
				err = discover.FreeDevices(c.context, n.Name, c.Namespace)
				if err != nil {
					logger.Warningf("failed to free devices: %s", err)
				}
				continue
			}
			logger.Infof("deployment for osd %d already exists", osd.ID)
		}
		logger.Infof("started deployment for osd %d (dir=%t, type=%s)", osd.ID, osd.IsDirectory, storeConfig.StoreType)
		succeeded++
	}

	return succeeded == len(osds)
}

func (c *Cluster) handleRemovedNodes(config *provisionConfig) {
	// find all removed nodes (if any) and start orchestration to remove them from the cluster
	removedNodes, err := c.findRemovedNodes()
	if err != nil {
		config.addError("failed to find removed nodes: %+v", err)
	}

	for i := range removedNodes {
		n := removedNodes[i]
		storeConfig := osdconfig.ToStoreConfig(n.node.Config)
		metadataDevice := osdconfig.MetadataDevice(n.node.Config)

		if err := c.isSafeToRemoveNode(n.node); err != nil {
			message := fmt.Sprintf("skipping the removal of node %s because it is not safe to do so: %+v", n.node.Name, err)
			c.handleOrchestrationFailure(config, n.node, message)
			continue
		}
		if len(n.node.Name) == 0 {
			continue
		}
		logger.Infof("removing node %s from the cluster", n.node.Name)

		// update the orchestration status of this removed node to the starting state
		if err := c.updateNodeStatus(n.node.Name, OrchestrationStatus{Status: OrchestrationStatusStarting}); err != nil {
			config.addError("failed to set orchestration starting status for removed node %s: %+v", n.node.Name, err)
			continue
		}

		// trigger orchestration on the removed node by telling it not to use any storage at all.  note that the directories are still passed in
		// so that the pod will be able to mount them and migrate data from them.
		job, err := c.makeJob(n.node.Name, nil, rookalpha.Selection{DeviceFilter: "none", Directories: n.node.Directories}, v1.ResourceRequirements{}, storeConfig, metadataDevice, "")
		if err != nil {
			message := fmt.Sprintf("failed to create osd job for removed node %s. %+v", n.node.Name, err)
			c.handleOrchestrationFailure(config, n.node, message)
			continue
		}
		job, err = c.context.Clientset.Batch().Jobs(c.Namespace).Update(job)
		if err != nil {
			message := fmt.Sprintf("failed to update osd job for removed node %s. %+v", n.node.Name, err)
			c.handleOrchestrationFailure(config, n.node, message)
			continue
		} else {
			logger.Infof("osd job updated for node %s", n.node.Name)
		}
		for _, dp := range n.deployments {
			// delete the pod associated with the deployment so that it will be restarted with the new template
			if err := c.deleteOSDPod(dp); err != nil {
				message := fmt.Sprintf("failed to find and delete OSD pod for deployments %s. %+v", dp.Name, err)
				c.handleOrchestrationFailure(config, n.node, message)
				continue
			}

			// wait for the removed node's orchestration to be completed
			if ok := c.completeOSDsForNodeRemoval(config, n.node.Name); !ok {
				continue
			}

			// orchestration of the removed node completed, we can delete the deployment now
			if err := c.context.Clientset.Extensions().Deployments(c.Namespace).Delete(dp.Name, &metav1.DeleteOptions{}); err != nil {
				config.addError("failed to delete deployment %s: %+v", dp.Name, err)
				continue
			}
		}
	}
}

func (c *Cluster) discoverStorageNodes() ([]deploymentPerNode, error) {
	var discoveredNodes []deploymentPerNode

	listOpts := metav1.ListOptions{LabelSelector: fmt.Sprintf("app=%s", appName)}
	osdDeployments, err := c.context.Clientset.Extensions().Deployments(c.Namespace).List(listOpts)
	if err != nil {
		return nil, fmt.Errorf("failed to list osd deployment: %+v", err)
	}
	discoveredNodes = make([]deploymentPerNode, len(osdDeployments.Items))
	for i, osdDeployment := range osdDeployments.Items {
		osdPodSpec := osdDeployment.Spec.Template.Spec

		// get the node name from the node selector
		nodeName, ok := osdPodSpec.NodeSelector[apis.LabelHostname]
		if !ok || nodeName == "" {
			return nil, fmt.Errorf("osd deployment %s doesn't have a node name on its node selector: %+v", osdDeployment.Name, osdPodSpec.NodeSelector)
		}
		// get the osd container
		osdContainer, err := k8sutil.GetMatchingContainer(osdPodSpec.Containers, appName)
		if err != nil {
			return nil, err
		}

		// populate the discovered node with the properties discovered from its running artifacts.
		// note that we are populating just a subset here, the minimum subset needed to be able
		// to remove the node if needed.  As we support updating more properties in the future (as
		// opposed to simply removing the whole node) we'll need to discover and populate them here too.
		node := rookalpha.Node{
			Name: nodeName,
			Selection: rookalpha.Selection{
				Directories: getDirectoriesFromContainer(osdContainer),
			},
			Location: rookalpha.GetLocationFromContainer(osdContainer),
			Config:   getConfigFromContainer(osdContainer),
		}
		found := false
		for _, n := range discoveredNodes {
			if nodeName == n.node.Name {
				n.deployments = append(n.deployments, &osdDeployment)
				found = true
				break
			}
		}
		if !found {
			discoveredNodes[i].node = node
			discoveredNodes[i].deployments = append(discoveredNodes[i].deployments, &osdDeployment)
		}
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

func (c *Cluster) deleteOSDPod(dp *extensions.Deployment) error {
	if dp == nil {
		return nil
	}
	// list all OSD pods first
	opts := metav1.ListOptions{LabelSelector: fields.OneTermEqualSelector(k8sutil.AppAttr, appName).String()}
	pods, err := c.context.Clientset.CoreV1().Pods(c.Namespace).List(opts)
	if err != nil {
		return err
	}

	// iterate over all the OSD pods, looking for a match to the given deployment and the pod's owner
	var pod *v1.Pod
	for i := range pods.Items {
		p := &pods.Items[i]
		for _, owner := range p.OwnerReferences {
			if owner.Name == dp.Name && owner.UID == dp.UID {
				// the owner of this pod matches the name and UID of the given deployment
				pod = p
				break
			}
		}

		if pod != nil {
			break
		}
	}

	if pod == nil {
		return fmt.Errorf("pod for deployment %s not found", dp.Name)
	}

	err = c.context.Clientset.CoreV1().Pods(c.Namespace).Delete(pod.Name, &metav1.DeleteOptions{})
	return err
}

func (c *Cluster) getOSDsForNode(node rookalpha.Node) ([]int, error) {

	// load all the OSD dirs/devices for the given node
	dirMap, err := osdconfig.LoadOSDDirMap(c.kv, node.Name)
	if err != nil && !errors.IsNotFound(err) {
		return nil, err
	}
	scheme, err := osdconfig.LoadScheme(c.kv, osdconfig.GetConfigStoreName(node.Name))
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

func (c *Cluster) resolveNode(nodeName string) *rookalpha.Node {
	// fully resolve the storage config and resources for this node
	rookNode := c.Storage.ResolveNode(nodeName)
	if rookNode == nil {
		return nil
	}
	rookNode.Resources = k8sutil.MergeResourceRequirements(rookNode.Resources, c.resources)
	return rookNode
}
