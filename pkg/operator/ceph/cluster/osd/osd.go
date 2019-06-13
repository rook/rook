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
	"strconv"
	"strings"

	"github.com/coreos/pkg/capnslog"
	cephv1 "github.com/rook/rook/pkg/apis/ceph.rook.io/v1"
	rookalpha "github.com/rook/rook/pkg/apis/rook.io/v1alpha2"
	"github.com/rook/rook/pkg/clusterd"
	"github.com/rook/rook/pkg/daemon/ceph/client"
	cephconfig "github.com/rook/rook/pkg/daemon/ceph/config"
	osdconfig "github.com/rook/rook/pkg/operator/ceph/cluster/osd/config"
	opspec "github.com/rook/rook/pkg/operator/ceph/spec"
	cephver "github.com/rook/rook/pkg/operator/ceph/version"
	"github.com/rook/rook/pkg/operator/k8sutil"
	"github.com/rook/rook/pkg/util/display"
	apps "k8s.io/api/apps/v1"
	batch "k8s.io/api/batch/v1"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var logger = capnslog.NewPackageLogger("github.com/rook/rook", "op-osd")

const (
	appName                             = "rook-ceph-osd"
	prepareAppName                      = "rook-ceph-osd-prepare"
	prepareAppNameFmt                   = "rook-ceph-osd-prepare-%s"
	legacyAppNameFmt                    = "rook-ceph-osd-id-%d"
	osdAppNameFmt                       = "rook-ceph-osd-%d"
	osdLabelKey                         = "ceph-osd-id"
	clusterAvailableSpaceReserve        = 0.05
	serviceAccountName                  = "rook-ceph-osd"
	unknownID                           = -1
	cephOsdPodMinimumMemory      uint64 = 4096 // minimum amount of memory in MB to run the pod
)

// Cluster keeps track of the OSDs
type Cluster struct {
	clusterInfo     *cephconfig.ClusterInfo
	context         *clusterd.Context
	Namespace       string
	placement       rookalpha.Placement
	annotations     rookalpha.Annotations
	Keyring         string
	rookVersion     string
	cephVersion     cephv1.CephVersionSpec
	DesiredStorage  rookalpha.StorageScopeSpec // user-defined storage scope spec
	ValidStorage    rookalpha.StorageScopeSpec // valid subset of `Storage`, computed at runtime
	dataDirHostPath string
	HostNetwork     bool
	resources       v1.ResourceRequirements
	ownerRef        metav1.OwnerReference
	kv              *k8sutil.ConfigMapKVStore
}

// New creates an instance of the OSD manager
func New(
	clusterInfo *cephconfig.ClusterInfo,
	context *clusterd.Context,
	namespace string,
	rookVersion string,
	cephVersion cephv1.CephVersionSpec,
	storageSpec rookalpha.StorageScopeSpec,
	dataDirHostPath string,
	placement rookalpha.Placement,
	annotations rookalpha.Annotations,
	hostNetwork bool,
	resources v1.ResourceRequirements,
	ownerRef metav1.OwnerReference,
) *Cluster {
	return &Cluster{
		clusterInfo:     clusterInfo,
		context:         context,
		Namespace:       namespace,
		placement:       placement,
		annotations:     annotations,
		rookVersion:     rookVersion,
		cephVersion:     cephVersion,
		DesiredStorage:  storageSpec,
		dataDirHostPath: dataDirHostPath,
		HostNetwork:     hostNetwork,
		resources:       resources,
		ownerRef:        ownerRef,
		kv:              k8sutil.NewConfigMapKVStore(namespace, context.Clientset, ownerRef),
	}
}

type OSDInfo struct {
	ID                  int    `json:"id"`
	DataPath            string `json:"data-path"`
	Config              string `json:"conf"`
	Cluster             string `json:"cluster"`
	KeyringPath         string `json:"keyring-path"`
	UUID                string `json:"uuid"`
	Journal             string `json:"journal"`
	IsFileStore         bool   `json:"is-file-store"`
	IsDirectory         bool   `json:"is-directory"`
	DevicePartUUID      string `json:"device-part-uuid"`
	CephVolumeInitiated bool   `json:"ceph-volume-initiated"`
}

type OrchestrationStatus struct {
	OSDs    []OSDInfo `json:"osds"`
	Status  string    `json:"status"`
	Message string    `json:"message"`
}

// Start the osd management
func (c *Cluster) Start() error {
	// Validate pod's memory if specified
	// This is valid for both Filestore and Bluestore
	err := opspec.CheckPodMemory(c.resources, cephOsdPodMinimumMemory)
	if err != nil {
		return fmt.Errorf("%v", err)
	}

	logger.Infof("start running osds in namespace %s", c.Namespace)

	if c.DesiredStorage.UseAllNodes == false && len(c.DesiredStorage.Nodes) == 0 {
		logger.Warningf("useAllNodes is set to false and no nodes are specified, no OSD pods are going to be created")
	}

	if c.DesiredStorage.UseAllNodes {
		// Get the list of all nodes in the cluster. The placement settings will be applied below.
		hostnameMap, err := k8sutil.GetNodeHostNames(c.context.Clientset)
		if err != nil {
			logger.Warningf("failed to get node hostnames: %v", err)
			return err
		}
		c.DesiredStorage.Nodes = nil
		for _, hostname := range hostnameMap {
			storageNode := rookalpha.Node{
				Name: hostname,
			}
			c.DesiredStorage.Nodes = append(c.DesiredStorage.Nodes, storageNode)
		}
		logger.Debugf("storage nodes: %+v", c.DesiredStorage.Nodes)
	}
	// generally speaking, this finds nodes which are capable of running new osds
	validNodes := k8sutil.GetValidNodes(c.DesiredStorage.Nodes, c.context.Clientset, c.placement)

	// no valid node is ready to run an osd
	if len(validNodes) == 0 {
		logger.Warningf("no valid node available to run an osd in namespace %s. "+
			"Rook will not create any new OSD nodes and will skip checking for removed nodes since "+
			"removing all OSD nodes without destroying the Rook cluster is unlikely to be intentional", c.Namespace)
		return nil
	}
	logger.Infof("%d of the %d storage nodes are valid", len(validNodes), len(c.DesiredStorage.Nodes))
	c.ValidStorage.Nodes = validNodes

	// start the jobs to provision the OSD devices and directories
	config := newProvisionConfig()
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

	// The following block is used to apply any command(s) required by an upgrade
	// The block below handles the upgrade from Mimic to Nautilus.
	if c.clusterInfo.CephVersion.IsAtLeastNautilus() {
		versions, err := client.GetCephVersions(c.context)
		if err != nil {
			logger.Warningf("failed to get ceph daemons versions. this likely means there are no osds yet. %+v", err)
		} else {
			// If length is one, this clearly indicates that all the osds are running the same version
			logger.Infof("len of version.Osd is %d", len(versions.Osd))
			// If this is the first time we are creating a cluster length will be 0
			// On an initial OSD boostrap, by the time we reach this code, the OSDs haven't registered yet
			// Basically, this task is happening too quickly and OSD pods are not running yet.
			// That's not an issue since it's an initial bootstrap and not an update.
			if len(versions.Osd) == 1 {
				for v := range versions.Osd {
					logger.Infof("v is %s", v)
					osdVersion, err := cephver.ExtractCephVersion(v)
					if err != nil {
						return fmt.Errorf("failed to extract ceph version. %+v", err)
					}
					logger.Infof("osdVersion is: %v", osdVersion)
					// if the version of these OSDs is Nautilus then we run the command
					if osdVersion.IsAtLeastNautilus() {
						client.EnableNautilusOSD(c.context)
					}
				}
			}
		}
	}

	logger.Infof("completed running osds in namespace %s", c.Namespace)
	return nil
}

func (c *Cluster) startProvisioning(config *provisionConfig) {
	if len(c.dataDirHostPath) == 0 {
		logger.Warningf("skipping osd provisioning where no dataDirHostPath is set")
		return
	}

	// start with nodes currently in the storage spec
	for _, node := range c.ValidStorage.Nodes {
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

		// create the job that prepares osds on the node
		storeConfig := osdconfig.ToStoreConfig(n.Config)
		metadataDevice := osdconfig.MetadataDevice(n.Config)
		job, err := c.makeJob(n.Name, n.Devices, n.Selection, n.Resources, storeConfig, metadataDevice, n.Location)
		if err != nil {
			message := fmt.Sprintf("failed to create prepare job node %s: %v", n.Name, err)
			config.addError(message)
			status := OrchestrationStatus{Status: OrchestrationStatusCompleted, Message: message}
			if err := c.updateNodeStatus(n.Name, status); err != nil {
				config.addError("failed to update node %s status. %+v", n.Name, err)
				continue
			}
		}

		if !c.runJob(job, n.Name, config, "provision") {
			status := OrchestrationStatus{Status: OrchestrationStatusCompleted, Message: fmt.Sprintf("failed to start osd provisioning on node %s", n.Name)}
			if err := c.updateNodeStatus(n.Name, status); err != nil {
				config.addError("failed to update node %s status. %+v", n.Name, err)
			}
		}
	}
}

func (c *Cluster) runJob(job *batch.Job, nodeName string, config *provisionConfig, action string) bool {
	if err := k8sutil.RunReplaceableJob(c.context.Clientset, job, false); err != nil {
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
		dp, err := c.makeDeployment(n.Name, n.Selection, n.Resources, storeConfig, metadataDevice, n.Location, osd)
		if err != nil {
			errMsg := fmt.Sprintf("failed to create deployment for node %s: %v", n.Name, err)
			config.addError(errMsg)
			continue
		}

		_, err = c.context.Clientset.AppsV1().Deployments(c.Namespace).Create(dp)
		if err != nil {
			if !errors.IsAlreadyExists(err) {
				// we failed to create job, update the orchestration status for this node
				logger.Warningf("failed to create osd deployment for node %s, osd %v: %+v", n.Name, osd, err)
				continue
			}
			logger.Infof("deployment for osd %d already exists. updating if needed", osd.ID)
			if _, err = k8sutil.UpdateDeploymentAndWait(c.context, dp, c.Namespace); err != nil {
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

			if err := c.removeOSD(dp.Name, id); err != nil {
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

	if !c.runJob(job, nodeName, config, "remove") {
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

// discover nodes which currently have osds scheduled on them. Return a mapping of
// node names -> a list of osd deployments on the node
func (c *Cluster) discoverStorageNodes() (map[string][]*apps.Deployment, error) {

	listOpts := metav1.ListOptions{LabelSelector: fmt.Sprintf("app=%s", appName)}
	osdDeployments, err := c.context.Clientset.AppsV1().Deployments(c.Namespace).List(listOpts)
	if err != nil {
		return nil, fmt.Errorf("failed to list osd deployment: %+v", err)
	}
	discoveredNodes := map[string][]*apps.Deployment{}
	for _, osdDeployment := range osdDeployments.Items {
		osdPodSpec := osdDeployment.Spec.Template.Spec

		// get the node name from the node selector
		nodeName, ok := osdPodSpec.NodeSelector[v1.LabelHostname]
		if !ok || nodeName == "" {
			return nil, fmt.Errorf("osd deployment %s doesn't have a node name on its node selector: %+v", osdDeployment.Name, osdPodSpec.NodeSelector)
		}

		if _, ok := discoveredNodes[nodeName]; !ok {
			discoveredNodes[nodeName] = []*apps.Deployment{}
		}

		logger.Debugf("adding osd %s to node %s", osdDeployment.Name, nodeName)
		osdCopy := osdDeployment
		discoveredNodes[nodeName] = append(discoveredNodes[nodeName], &osdCopy)
	}

	return discoveredNodes, nil
}

func (c *Cluster) isSafeToRemoveNode(nodeName string, osdDeployments []*apps.Deployment) error {
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

func getIDFromDeployment(deployment *apps.Deployment) int {
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
	rookNode := c.DesiredStorage.ResolveNode(nodeName)
	if rookNode == nil {
		return nil
	}
	rookNode.Resources = k8sutil.MergeResourceRequirements(rookNode.Resources, c.resources)

	return rookNode
}
