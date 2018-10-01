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
	"strconv"
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
	batch "k8s.io/api/batch/v1"
	"k8s.io/api/core/v1"
	extensions "k8s.io/api/extensions/v1beta1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
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
	unknownID                    = -1
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
func New(
	context *clusterd.Context,
	namespace,
	version,
	serviceAccount string,
	storageSpec rookalpha.StorageScopeSpec,
	dataDirHostPath string,
	placement rookalpha.Placement,
	hostNetwork bool,
	resources v1.ResourceRequirements,
	ownerRef metav1.OwnerReference,
) *Cluster {

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
		hostnameMap, err := k8sutil.GetNodeHostNames(c.context.Clientset)
		if err != nil {
			logger.Warningf("failed to get node hostnames: %v", err)
			return err
		}
		for nodeName := range allNodeDevices {
			hostname, ok := hostnameMap[nodeName]
			if !ok || nodeName == "" {
				// fall back to the node name if no hostname is set
				logger.Warningf("failed to get hostname for node %s. %+v", nodeName, err)
				hostname = nodeName
			}
			storageNode := rookalpha.Node{
				Name: hostname,
			}
			c.Storage.Nodes = append(c.Storage.Nodes, storageNode)
		}
		logger.Debugf("storage nodes: %+v", c.Storage.Nodes)
	}
	validNodes := k8sutil.GetValidNodes(c.Storage.Nodes, c.context.Clientset, c.placement)
	logger.Infof("%d of the %d storage nodes are valid", len(validNodes), len(c.Storage.Nodes))
	c.Storage.Nodes = validNodes
	// orchestrate individual nodes, starting with any that are still ongoing (in the case that we
	// are resuming a previous orchestration attempt)
	config := newProvisionConfig()
	logger.Infof("checking if orchestration is still in progress")
	c.completeProvisionSkipOSDStart(config)

	// start the jobs to provision the OSD devices and directories
	logger.Infof("start provisioning the osds on nodes, if needed")
	c.startProvisioning(config)

	// start the OSD pods, waiting for the provisioning to be completed
	logger.Infof("start osds after provisioning is completed, if needed")
	c.completeProvision(config)

	// handle the removed nodes and rebalance the PGs
	logger.Infof("checking if any nodes were removed")
	c.handleRemovedNodes(config)

	if len(config.errorMessages) > 0 {
		return fmt.Errorf("%d failures encountered while running osds in namespace %s: %+v",
			len(config.errorMessages), c.Namespace, strings.Join(config.errorMessages, "\n"))
	}

	logger.Infof("completed running osds in namespace %s", c.Namespace)
	return nil
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
		storeConfig := osdconfig.ToStoreConfig(n.Config)
		metadataDevice := osdconfig.MetadataDevice(n.Config)
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

		if !c.updateJob(job, n.Name, config, "provision") {
			if err = discover.FreeDevices(c.context, n.Name, c.Namespace); err != nil {
				logger.Warningf("failed to free devices: %s", err)
			}
		}
	}
}

func (c *Cluster) updateJob(job *batch.Job, nodeName string, config *provisionConfig, action string) bool {
	// check if the job was already created and what its status is
	existingJob, err := c.context.Clientset.Batch().Jobs(c.Namespace).Get(job.Name, metav1.GetOptions{})
	if err != nil && !errors.IsNotFound(err) {
		config.addError("failed to detect %s job for node %s. %+v", action, nodeName, err)
	} else if err == nil {
		// delete the job that already exists from a previous run
		if existingJob.Status.Active > 0 {
			logger.Infof("Found previous %s job for node %s. Status=%+v", action, nodeName, existingJob.Status)
			return true
		}

		logger.Infof("Removing previous %s job for node %s to start a new one", action, nodeName)
		err := c.deleteBatchJob(existingJob.Name)
		if err != nil {
			logger.Warningf("failed to remove job %s. %+v", nodeName, err)
		}
	}

	_, err = c.context.Clientset.Batch().Jobs(c.Namespace).Create(job)
	if err != nil {
		if !errors.IsAlreadyExists(err) {
			// we failed to create job, update the orchestration status for this node
			message := fmt.Sprintf("failed to create %s job for node %s. %+v", action, nodeName, err)
			c.handleOrchestrationFailure(config, nodeName, message)
			return false
		}

		// the job is already in progress so we will let it run to completion
	}

	logger.Infof("osd %s job started for node %s", action, nodeName)
	return true
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

func (c *Cluster) startOSDDaemonsOnNode(nodeName string, config *provisionConfig, configMap *v1.ConfigMap, status *OrchestrationStatus) {

	osds := status.OSDs
	logger.Infof("starting %d osd daemons on node %s", len(osds), nodeName)

	// fully resolve the storage config and resources for this node
	n := c.resolveNode(nodeName)
	if n == nil {
		config.addError("node %s did not resolve to start osds", nodeName)
		return
	}

	storeConfig := osdconfig.ToStoreConfig(n.Config)
	metadataDevice := osdconfig.MetadataDevice(n.Config)

	// start osds
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
		_, err = c.context.Clientset.Extensions().Deployments(c.Namespace).Create(dp)
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
			logger.Infof("deployment for osd %d already exists. updating if needed", osd.ID)
			if err = k8sutil.UpdateDeploymentAndWait(c.context, dp, c.Namespace); err != nil {
				config.addError(fmt.Sprintf("failed to update osd deployment %d. %+v", osd.ID, err))
			}
		}

		logger.Infof("started deployment for osd %d (dir=%t, type=%s)", osd.ID, osd.IsDirectory, storeConfig.StoreType)
	}
}

func (c *Cluster) handleRemovedNodes(config *provisionConfig) {
	// find all removed nodes (if any) and start orchestration to remove them from the cluster
	removedNodes, err := c.findRemovedNodes()
	if err != nil {
		config.addError("failed to find removed nodes: %+v", err)
	}
	logger.Infof("processing %d removed nodes", len(removedNodes))

	for removedNode, osdDeployments := range removedNodes {
		logger.Infof("processing removed node %s", removedNode)
		if err := c.isSafeToRemoveNode(removedNode, osdDeployments); err != nil {
			logger.Warningf("skipping the removal of node %s because it is not safe to do so: %+v", removedNode, err)
			continue
		}

		logger.Infof("removing node %s from the cluster with %d OSDs", removedNode, len(osdDeployments))

		var nodeCrushName string
		errorOnCurrentNode := false
		for _, dp := range osdDeployments {

			logger.Infof("processing removed osd %s", dp.Name)
			id := getIDFromDeployment(dp)
			if id == unknownID {
				config.addError("cannot remove unknown osd %s", dp.Name)
				continue
			}

			// on the first osd, get the crush name of the host
			if nodeCrushName == "" {
				nodeCrushName, err = client.GetCrushHostName(c.context, c.Namespace, id)
				if err != nil {
					config.addError("failed to get crush host name for osd.%d: %+v", id, err)
				}
			}

			if err := removeOSD(c.context, c.Namespace, dp.Name, id); err != nil {
				config.addError("failed to remove osd %d. %+v", id, err)
				errorOnCurrentNode = true
				continue
			}
		}

		if err := c.updateNodeStatus(removedNode, OrchestrationStatus{Status: OrchestrationStatusCompleted}); err != nil {
			config.addError("failed to set orchestration starting status for removed node %s: %+v", removedNode, err)
		}

		if errorOnCurrentNode {
			logger.Warningf("done processing %d osd removals on node %s with an error removing the osds. skipping node cleanup", len(osdDeployments), removedNode)
		} else {
			logger.Infof("succeeded processing %d osd removals on node %s. starting cleanup job on the node.", len(osdDeployments), removedNode)
			c.cleanupRemovedNode(config, removedNode, nodeCrushName)
		}
	}
	logger.Infof("done processing removed nodes")
}

func (c *Cluster) cleanupRemovedNode(config *provisionConfig, nodeName, crushName string) {
	// update the orchestration status of this removed node to the starting state
	if err := c.updateNodeStatus(nodeName, OrchestrationStatus{Status: OrchestrationStatusStarting}); err != nil {
		config.addError("failed to set orchestration starting status for removed node %s: %+v", nodeName, err)
		return
	}

	// trigger orchestration on the removed node by telling it not to use any storage at all.  note that the directories are still passed in
	// so that the pod will be able to mount them and migrate data from them.
	job, err := c.makeJob(nodeName, []rookalpha.Device{}, rookalpha.Selection{DeviceFilter: "none"},
		v1.ResourceRequirements{}, osdconfig.StoreConfig{}, "", "")
	if err != nil {
		message := fmt.Sprintf("failed to create prepare job node %s: %v", nodeName, err)
		config.addError(message)
		status := OrchestrationStatus{Status: OrchestrationStatusCompleted, Message: message}
		if err := c.updateNodeStatus(nodeName, status); err != nil {
			config.addError("failed to update node %s status. %+v", nodeName, err)
		}
		return
	}

	if !c.updateJob(job, nodeName, config, "remove") {
		status := OrchestrationStatus{Status: OrchestrationStatusCompleted, Message: fmt.Sprintf("failed to cleanup osd config on node %s", nodeName)}
		if err := c.updateNodeStatus(nodeName, status); err != nil {
			config.addError("failed to update node %s status. %+v", nodeName, err)
		}
		return
	}

	logger.Infof("waiting for removal cleanup on node %s", nodeName)
	c.completeProvisionSkipOSDStart(config)
	logger.Infof("done waiting for removal cleanup on node %s", nodeName)

	// after the batch job is finished, clean up all the resources related to the node
	if crushName != "" {
		if err := c.cleanUpNodeResources(nodeName, crushName); err != nil {
			config.addError("failed to cleanup node resources for %s", crushName)
		}
	}
}

func (c *Cluster) discoverStorageNodes() (map[string][]*extensions.Deployment, error) {

	listOpts := metav1.ListOptions{LabelSelector: fmt.Sprintf("app=%s", appName)}
	osdDeployments, err := c.context.Clientset.Extensions().Deployments(c.Namespace).List(listOpts)
	if err != nil {
		return nil, fmt.Errorf("failed to list osd deployment: %+v", err)
	}
	discoveredNodes := map[string][]*extensions.Deployment{}
	for _, osdDeployment := range osdDeployments.Items {
		osdPodSpec := osdDeployment.Spec.Template.Spec

		// get the node name from the node selector
		nodeName, ok := osdPodSpec.NodeSelector[apis.LabelHostname]
		if !ok || nodeName == "" {
			return nil, fmt.Errorf("osd deployment %s doesn't have a node name on its node selector: %+v", osdDeployment.Name, osdPodSpec.NodeSelector)
		}

		if _, ok := discoveredNodes[nodeName]; !ok {
			discoveredNodes[nodeName] = []*extensions.Deployment{}
		}

		logger.Debugf("adding osd %s to node %s", osdDeployment.Name, nodeName)
		osdCopy := osdDeployment
		discoveredNodes[nodeName] = append(discoveredNodes[nodeName], &osdCopy)
	}

	return discoveredNodes, nil
}

func (c *Cluster) isSafeToRemoveNode(nodeName string, osdDeployments []*extensions.Deployment) error {
	if err := client.IsClusterClean(c.context, c.Namespace); err != nil {
		// the cluster isn't clean, it's not safe to remove this node
		return err
	}

	// get the current used space on all OSDs in the cluster
	currUsage, err := client.GetOSDUsage(c.context, c.Namespace)
	if err != nil {
		return err
	}

	// sum up the total OSD used space for the node by summing the used space of each OSD on the node
	nodeUsage := int64(0)
	for _, osdDeployment := range osdDeployments {
		id := getIDFromDeployment(osdDeployment)
		if id == unknownID {
			continue
		}

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
			nodeName, display.BytesToString(uint64(nodeUsage)), display.BytesToString(uint64(clusterAvailableBytes)))
	}

	// looks safe to remove the node
	return nil
}

func getIDFromDeployment(deployment *extensions.Deployment) int {
	if idstr, ok := deployment.Labels[osdLabelKey]; ok {
		id, err := strconv.Atoi(idstr)
		if err != nil {
			logger.Errorf("unknown osd id from label %s", idstr)
			return unknownID
		}
		return id
	}
	logger.Errorf("unknown osd id for deployment %s", deployment.Name)
	return unknownID
}

func (c *Cluster) resolveNode(nodeName string) *rookalpha.Node {
	// fully resolve the storage config and resources for this node
	rookNode := c.Storage.ResolveNode(nodeName)
	if rookNode == nil {
		return nil
	}
	rookNode.Resources = k8sutil.MergeResourceRequirements(rookNode.Resources, c.resources)

	// ensure no invalid dirs are specified
	var validDirs []rookalpha.Directory
	for _, dir := range rookNode.Directories {
		if dir.Path == k8sutil.DataDir || dir.Path == c.dataDirHostPath {
			logger.Warningf("skipping directory %s that would conflict with the dataDirHostPath", dir.Path)
			continue
		}
		validDirs = append(validDirs, dir)
	}
	rookNode.Directories = validDirs

	return rookNode
}
