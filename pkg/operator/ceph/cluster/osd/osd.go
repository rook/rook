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
	"github.com/rook/rook/pkg/operator/ceph/cluster/mon"
	osdconfig "github.com/rook/rook/pkg/operator/ceph/cluster/osd/config"
	opconfig "github.com/rook/rook/pkg/operator/ceph/config"
	opspec "github.com/rook/rook/pkg/operator/ceph/spec"
	cephver "github.com/rook/rook/pkg/operator/ceph/version"
	"github.com/rook/rook/pkg/operator/k8sutil"
	"github.com/rook/rook/pkg/util/display"
	apps "k8s.io/api/apps/v1"
	batch "k8s.io/api/batch/v1"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/version"
)

var (
	logger                  = capnslog.NewPackageLogger("github.com/rook/rook", "op-osd")
	updateDeploymentAndWait = mon.UpdateCephDeploymentAndWait
)

const (
	// AppName is the "app" label on osd pods
	AppName = "rook-ceph-osd"
	// FailureDomainKey is the label key whose value is the failure domain of the OSD
	FailureDomainKey                    = "failure-domain"
	prepareAppName                      = "rook-ceph-osd-prepare"
	prepareAppNameFmt                   = "rook-ceph-osd-prepare-%s"
	legacyAppNameFmt                    = "rook-ceph-osd-id-%d"
	osdAppNameFmt                       = "rook-ceph-osd-%d"
	OsdIdLabelKey                       = "ceph-osd-id"
	clusterAvailableSpaceReserve        = 0.05
	serviceAccountName                  = "rook-ceph-osd"
	unknownID                           = -1
	portableKey                         = "portable"
	cephOsdPodMinimumMemory      uint64 = 2048 // minimum amount of memory in MB to run the pod
)

// Cluster keeps track of the OSDs
type Cluster struct {
	clusterInfo       *cephconfig.ClusterInfo
	context           *clusterd.Context
	Namespace         string
	placement         rookalpha.Placement
	annotations       rookalpha.Annotations
	Keyring           string
	rookVersion       string
	cephVersion       cephv1.CephVersionSpec
	DesiredStorage    rookalpha.StorageScopeSpec // user-defined storage scope spec
	ValidStorage      rookalpha.StorageScopeSpec // valid subset of `Storage`, computed at runtime
	dataDirHostPath   string
	Network           cephv1.NetworkSpec
	resources         v1.ResourceRequirements
	prepareResources  v1.ResourceRequirements
	ownerRef          metav1.OwnerReference
	kv                *k8sutil.ConfigMapKVStore
	isUpgrade         bool
	skipUpgradeChecks bool
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
	network cephv1.NetworkSpec,
	resources v1.ResourceRequirements,
	prepareResources v1.ResourceRequirements,
	ownerRef metav1.OwnerReference,
	isUpgrade bool,
	skipUpgradeChecks bool,
) *Cluster {
	return &Cluster{
		clusterInfo:       clusterInfo,
		context:           context,
		Namespace:         namespace,
		placement:         placement,
		annotations:       annotations,
		rookVersion:       rookVersion,
		cephVersion:       cephVersion,
		DesiredStorage:    storageSpec,
		dataDirHostPath:   dataDirHostPath,
		Network:           network,
		resources:         resources,
		prepareResources:  prepareResources,
		ownerRef:          ownerRef,
		kv:                k8sutil.NewConfigMapKVStore(namespace, context.Clientset, ownerRef),
		isUpgrade:         isUpgrade,
		skipUpgradeChecks: skipUpgradeChecks,
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
	//LVPath is the logical Volume path for an OSD created by Ceph-volume with format '/dev/<Volume Group>/<Logical Volume>'
	LVPath        string `json:"lv-path"`
	SkipLVRelease bool   `json:"skip-lv-release"`
}

type OrchestrationStatus struct {
	OSDs         []OSDInfo `json:"osds"`
	Status       string    `json:"status"`
	PvcBackedOSD bool      `json:"pvc-backed-osd"`
	Message      string    `json:"message"`
}

type osdProperties struct {
	//crushHostname refers to the hostname or PVC name when the OSD is provisioned on Nodes or PVC block device, respectively.
	crushHostname  string
	devices        []rookalpha.Device
	pvc            v1.PersistentVolumeClaimVolumeSource
	selection      rookalpha.Selection
	resources      v1.ResourceRequirements
	storeConfig    osdconfig.StoreConfig
	placement      rookalpha.Placement
	metadataDevice string
	location       string
	portable       bool
}

// Start the osd management
func (c *Cluster) Start() error {
	config := c.newProvisionConfig()

	// Validate pod's memory if specified
	// This is valid for both Filestore and Bluestore
	err := opspec.CheckPodMemory(c.resources, cephOsdPodMinimumMemory)
	if err != nil {
		return fmt.Errorf("%v", err)
	}

	logger.Infof("start running osds in namespace %s", c.Namespace)

	if c.DesiredStorage.UseAllNodes == false && len(c.DesiredStorage.Nodes) == 0 && len(c.DesiredStorage.VolumeSources) == 0 && len(c.DesiredStorage.StorageClassDeviceSets) == 0 {
		logger.Warningf("useAllNodes is set to false and no nodes, storageClassDevicesets or volumeSources are specified, no OSD pods are going to be created")
	}

	// start the jobs to provision the OSD devices and directories

	logger.Infof("start provisioning the osds on pvcs, if needed")
	c.startProvisioningOverPVCs(config)

	logger.Infof("start provisioning the osds on nodes, if needed")
	c.startProvisioningOverNodes(config)

	if len(config.errorMessages) > 0 {
		return fmt.Errorf("%d failures encountered while running osds in namespace %s: %+v",
			len(config.errorMessages), c.Namespace, strings.Join(config.errorMessages, "\n"))
	}

	// The following block is used to apply any command(s) required by an upgrade
	// The block below handles the upgrade from Mimic to Nautilus.
	if c.clusterInfo.CephVersion.IsAtLeastNautilus() {
		versions, err := client.GetAllCephDaemonVersions(c.context, c.clusterInfo.Name)
		if err != nil {
			logger.Warningf("failed to get ceph daemons versions; this likely means there are no osds yet. %+v", err)
		} else {
			// If length is one, this clearly indicates that all the osds are running the same version
			// If this is the first time we are creating a cluster length will be 0
			// On an initial OSD boostrap, by the time we reach this code, the OSDs haven't registered yet
			// Basically, this task is happening too quickly and OSD pods are not running yet.
			// That's not an issue since it's an initial bootstrap and not an update.
			if len(versions.Osd) == 1 {
				for v := range versions.Osd {
					osdVersion, err := cephver.ExtractCephVersion(v)
					if err != nil {
						return fmt.Errorf("failed to extract ceph version. %+v", err)
					}
					// if the version of these OSDs is Nautilus then we run the command
					if osdVersion.IsAtLeastNautilus() {
						client.EnableNautilusOSD(c.context, c.Namespace)
					}
				}
			}
		}
	}

	logger.Infof("completed running osds in namespace %s", c.Namespace)
	return nil
}

func (c *Cluster) startProvisioningOverPVCs(config *provisionConfig) {
	// Parsing storageClassDeviceSets and parsing it to volume sources
	c.DesiredStorage.VolumeSources = append(c.DesiredStorage.VolumeSources, c.prepareStorageClassDeviceSets(config)...)

	c.ValidStorage.VolumeSources = c.DesiredStorage.VolumeSources

	// no validVolumeSource is ready to run an osd
	if len(c.DesiredStorage.VolumeSources) == 0 && len(c.DesiredStorage.StorageClassDeviceSets) == 0 {
		logger.Info("no volume sources defined to configure OSDs on PVCs.")
		return
	}

	//check k8s version
	k8sVersion, err := k8sutil.GetK8SVersion(c.context.Clientset)
	if err != nil {
		config.addError("error finding Kubernetes version. %+v", err)
		return
	}
	if !k8sVersion.AtLeast(version.MustParseSemantic("v1.13.0")) {
		logger.Warningf("skipping OSD on PVC provisioning. Minimum Kubernetes version required: 1.13.0. Actual version: %s", k8sVersion.String())
		return
	}

	for _, volume := range c.ValidStorage.VolumeSources {
		osdProps := osdProperties{
			crushHostname: volume.PersistentVolumeClaimSource.ClaimName,
			pvc:           volume.PersistentVolumeClaimSource,
			resources:     volume.Resources,
			placement:     volume.Placement,
			portable:      volume.Portable,
		}

		// update the orchestration status of this pvc to the starting state
		status := OrchestrationStatus{Status: OrchestrationStatusStarting, PvcBackedOSD: true}
		if err := c.updateOSDStatus(osdProps.crushHostname, status); err != nil {
			config.addError("failed to set orchestration starting status for pvc %s: %+v", osdProps.crushHostname, err)
			continue
		}

		//Skip OSD prepare if deployment already exists for the PVC
		listOpts := metav1.ListOptions{LabelSelector: fmt.Sprintf("%s=%s,%s=%s",
			k8sutil.AppAttr, AppName,
			OSDOverPVCLabelKey, volume.PersistentVolumeClaimSource.ClaimName,
		)}

		osdDeployments, err := c.context.Clientset.AppsV1().Deployments(c.Namespace).List(listOpts)
		if err != nil {
			config.addError("failed to check if OSD daemon exists for pvc %q. %+v", osdProps.crushHostname, err)
			continue
		}

		if len(osdDeployments.Items) != 0 {
			logger.Infof("skip OSD prepare pod creation as OSD daemon already exists for %q", osdProps.crushHostname)
			osds, err := getOSDInfo(&osdDeployments.Items[0])
			if err != nil {
				config.addError("failed to get osdInfo for pvc %q. %+v", osdProps.crushHostname, err)
				continue
			}
			// update the orchestration status of this pvc to the completed state
			status = OrchestrationStatus{OSDs: osds, Status: OrchestrationStatusCompleted, PvcBackedOSD: true}
			if err := c.updateOSDStatus(osdProps.crushHostname, status); err != nil {
				config.addError("failed to update pvc %q status. %+v", osdProps.crushHostname, err)
				continue
			}
			continue
		}

		job, err := c.makeJob(osdProps, config)
		if err != nil {
			message := fmt.Sprintf("failed to create prepare job for pvc %s: %v", osdProps.crushHostname, err)
			config.addError(message)
			status := OrchestrationStatus{Status: OrchestrationStatusCompleted, Message: message, PvcBackedOSD: true}
			if err := c.updateOSDStatus(osdProps.crushHostname, status); err != nil {
				config.addError("failed to update pvc %q status. %+v", osdProps.crushHostname, err)
				continue
			}
		}

		if !c.runJob(job, osdProps.crushHostname, config, "provision") {
			status := OrchestrationStatus{
				Status:       OrchestrationStatusCompleted,
				Message:      fmt.Sprintf("failed to start osd provisioning on pvc %s", osdProps.crushHostname),
				PvcBackedOSD: true,
			}
			if err := c.updateOSDStatus(osdProps.crushHostname, status); err != nil {
				config.addError("failed to update osd %s status. %+v", osdProps.crushHostname, err)
			}
		}
	}
	logger.Infof("start osds after provisioning is completed, if needed")
	c.completeProvision(config)
}

func (c *Cluster) startProvisioningOverNodes(config *provisionConfig) {
	if len(c.dataDirHostPath) == 0 {
		logger.Warningf("skipping osd provisioning where no dataDirHostPath is set")
		return
	}

	if c.DesiredStorage.UseAllNodes {
		// Get the list of all nodes in the cluster. The placement settings will be applied below.
		hostnameMap, err := k8sutil.GetNodeHostNames(c.context.Clientset)
		if err != nil {
			config.addError("failed to get node hostnames: %v", err)
			return
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
	validNodes := k8sutil.GetValidNodes(c.DesiredStorage, c.context.Clientset, c.placement)

	logger.Infof("%d of the %d storage nodes are valid", len(validNodes), len(c.DesiredStorage.Nodes))

	c.ValidStorage = *c.DesiredStorage.DeepCopy()
	c.ValidStorage.Nodes = validNodes

	// no valid node is ready to run an osd
	if len(validNodes) == 0 {
		logger.Warningf("no valid nodes available to run an osd in namespace %s. "+
			"Rook will not create any new OSD nodes and will skip checking for removed nodes since "+
			"removing all OSD nodes without destroying the Rook cluster is unlikely to be intentional", c.Namespace)
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
		if err := c.updateOSDStatus(n.Name, status); err != nil {
			config.addError("failed to set orchestration starting status for node %s: %+v", n.Name, err)
			continue
		}

		// create the job that prepares osds on the node
		storeConfig := osdconfig.ToStoreConfig(n.Config)
		metadataDevice := osdconfig.MetadataDevice(n.Config)
		osdProps := osdProperties{
			crushHostname:  n.Name,
			devices:        n.Devices,
			selection:      n.Selection,
			resources:      n.Resources,
			storeConfig:    storeConfig,
			metadataDevice: metadataDevice,
		}
		job, err := c.makeJob(osdProps, config)
		if err != nil {
			message := fmt.Sprintf("failed to create prepare job node %s: %v", n.Name, err)
			config.addError(message)
			status := OrchestrationStatus{Status: OrchestrationStatusCompleted, Message: message}
			if err := c.updateOSDStatus(n.Name, status); err != nil {
				config.addError("failed to update node %s status. %+v", n.Name, err)
				continue
			}
		}

		if !c.runJob(job, n.Name, config, "provision") {
			status := OrchestrationStatus{Status: OrchestrationStatusCompleted, Message: fmt.Sprintf("failed to start osd provisioning on node %s", n.Name)}
			if err := c.updateOSDStatus(n.Name, status); err != nil {
				config.addError("failed to update node %s status. %+v", n.Name, err)
			}
		}
	}
	logger.Infof("start osds after provisioning is completed, if needed")
	c.completeProvision(config)

	// start the OSD pods, waiting for the provisioning to be completed
	// handle the removed nodes and rebalance the PGs
	logger.Infof("checking if any nodes were removed")
	c.handleRemovedNodes(config)

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

func (c *Cluster) startOSDDaemonsOnPVC(pvcName string, config *provisionConfig, configMap *v1.ConfigMap, status *OrchestrationStatus) {
	osds := status.OSDs
	logger.Infof("starting %d osd daemons on pvc %s", len(osds), pvcName)
	conf := make(map[string]string)
	storeConfig := osdconfig.ToStoreConfig(conf)
	osdProps, err := c.getOSDPropsForPVC(pvcName)
	if err != nil {
		config.addError(fmt.Sprintf("%+v", err))
		return
	}

	// start osds
	for _, osd := range osds {
		logger.Debugf("start osd %v", osd)

		// keyring must be generated before deployment creation in order to avoid a race condition resulting
		// in intermittent failure of first-attempt OSD pods.
		keyring, err := c.generateKeyring(osd.ID)
		if err != nil {
			errMsg := fmt.Sprintf("failed to create keyring for pvc %s, osd %v: %+v", osdProps.crushHostname, osd, err)
			config.addError(errMsg)
			continue
		}

		dp, err := c.makeDeployment(osdProps, osd, config)
		if err != nil {
			errMsg := fmt.Sprintf("failed to create deployment for pvc %s: %v", osdProps.crushHostname, err)
			config.addError(errMsg)
			continue
		}

		createdDeployment, createErr := c.context.Clientset.AppsV1().Deployments(c.Namespace).Create(dp)
		if createErr != nil {
			if !errors.IsAlreadyExists(createErr) {
				// we failed to create job, update the orchestration status for this pvc
				logger.Warningf("failed to create osd deployment for pvc %s, osd %v: %+v", osdProps.pvc.ClaimName, osd, createErr)
				continue
			}
			logger.Infof("deployment for osd %d already exists. updating if needed", osd.ID)
			createdDeployment, err = c.context.Clientset.AppsV1().Deployments(c.Namespace).Get(dp.Name, metav1.GetOptions{})
			if err != nil {
				logger.Warningf("failed to get existing OSD deployment %q for update: %+v", dp.Name, err)
				continue
			}
		}

		err = c.associateKeyring(keyring, createdDeployment)
		if err != nil {
			errMsg := fmt.Sprintf("failed to associate keyring for pvc %s, osd %v: %+v", osdProps.pvc.ClaimName, osd, err)
			config.addError(errMsg)
		}

		if createErr != nil && errors.IsAlreadyExists(createErr) {
			// Always invoke ceph version before an upgrade so we are sure to be up-to-date
			daemon := string(opconfig.OsdType)
			var cephVersionToUse cephver.CephVersion

			// If this is not an upgrade there is no need to check the ceph version
			if c.isUpgrade {
				currentCephVersion, err := client.LeastUptodateDaemonVersion(c.context, c.clusterInfo.Name, daemon)
				if err != nil {
					logger.Warningf("failed to retrieve current ceph %s version. %+v", daemon, err)
					logger.Debug("could not detect ceph version during update, this is likely an initial bootstrap, proceeding with c.clusterInfo.CephVersion")
					cephVersionToUse = c.clusterInfo.CephVersion
				} else {
					logger.Debugf("current cluster version for osds before upgrading is: %+v", currentCephVersion)
					cephVersionToUse = currentCephVersion
				}
			}

			if err = updateDeploymentAndWait(c.context, dp, c.Namespace, daemon, strconv.Itoa(osd.ID), cephVersionToUse, c.isUpgrade, c.skipUpgradeChecks); err != nil {
				config.addError(fmt.Sprintf("failed to update osd deployment %d. %+v", osd.ID, err))
			}
		}
		logger.Infof("started deployment for osd %d (dir=%t, type=%s)", osd.ID, osd.IsDirectory, storeConfig.StoreType)
	}
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

	osdProps := osdProperties{
		crushHostname:  n.Name,
		devices:        n.Devices,
		selection:      n.Selection,
		resources:      n.Resources,
		storeConfig:    storeConfig,
		metadataDevice: metadataDevice,
	}

	// start osds
	for _, osd := range osds {
		logger.Debugf("start osd %v", osd)

		// keyring must be generated before deployment creation in order to avoid a race condition resulting
		// in intermittent failure of first-attempt OSD pods.
		keyring, err := c.generateKeyring(osd.ID)
		if err != nil {
			errMsg := fmt.Sprintf("failed to create keyring for node %s, osd %v: %+v", n.Name, osd, err)
			config.addError(errMsg)
			continue
		}

		dp, err := c.makeDeployment(osdProps, osd, config)
		if err != nil {
			errMsg := fmt.Sprintf("failed to create deployment for node %s: %v", n.Name, err)
			config.addError(errMsg)
			continue
		}

		createdDeployment, createErr := c.context.Clientset.AppsV1().Deployments(c.Namespace).Create(dp)
		if createErr != nil {
			if !errors.IsAlreadyExists(createErr) {
				// we failed to create job, update the orchestration status for this node
				logger.Warningf("failed to create osd deployment for node %s, osd %v: %+v", n.Name, osd, createErr)
				continue
			}
			logger.Infof("deployment for osd %d already exists. updating if needed", osd.ID)
			createdDeployment, err = c.context.Clientset.AppsV1().Deployments(c.Namespace).Get(dp.Name, metav1.GetOptions{})
			if err != nil {
				logger.Warningf("failed to get existing OSD deployment %s for update: %+v", dp.Name, err)
				continue
			}
		}

		err = c.associateKeyring(keyring, createdDeployment)
		if err != nil {
			errMsg := fmt.Sprintf("failed to associate keyring for node %s, osd %v: %+v", n.Name, osd, err)
			config.addError(errMsg)
		}

		if createErr != nil && errors.IsAlreadyExists(createErr) {
			// Always invoke ceph version before an upgrade so we are sure to be up-to-date
			daemon := string(opconfig.OsdType)
			var cephVersionToUse cephver.CephVersion

			// If this is not an upgrade there is no need to check the ceph version
			if c.isUpgrade {
				currentCephVersion, err := client.LeastUptodateDaemonVersion(c.context, c.clusterInfo.Name, daemon)
				if err != nil {
					logger.Warningf("failed to retrieve current ceph %s version. %+v", daemon, err)
					logger.Debug("could not detect ceph version during update, this is likely an initial bootstrap, proceeding with c.clusterInfo.CephVersion")
					cephVersionToUse = c.clusterInfo.CephVersion
				} else {
					logger.Debugf("current cluster version for osds before upgrading is: %+v", currentCephVersion)
					cephVersionToUse = currentCephVersion
				}
			}

			if err = updateDeploymentAndWait(c.context, dp, c.Namespace, daemon, strconv.Itoa(osd.ID), cephVersionToUse, c.isUpgrade, c.skipUpgradeChecks); err != nil {
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
		return
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

		if err := c.updateOSDStatus(removedNode, OrchestrationStatus{Status: OrchestrationStatusCompleted}); err != nil {
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
	if err := c.updateOSDStatus(nodeName, OrchestrationStatus{Status: OrchestrationStatusStarting}); err != nil {
		config.addError("failed to set orchestration starting status for removed node %s: %+v", nodeName, err)
		return
	}

	// trigger orchestration on the removed node by telling it not to use any storage at all.  note that the directories are still passed in
	// so that the pod will be able to mount them and migrate data from them.
	osdProps := osdProperties{
		crushHostname: nodeName,
		devices:       []rookalpha.Device{},
		selection:     rookalpha.Selection{DeviceFilter: "none"},
		resources:     v1.ResourceRequirements{},
		storeConfig:   osdconfig.StoreConfig{},
	}
	job, err := c.makeJob(osdProps, config)
	if err != nil {
		message := fmt.Sprintf("failed to create prepare job node %s: %v", nodeName, err)
		config.addError(message)
		status := OrchestrationStatus{Status: OrchestrationStatusCompleted, Message: message}
		if err := c.updateOSDStatus(nodeName, status); err != nil {
			config.addError("failed to update node %s status. %+v", nodeName, err)
		}
		return
	}

	if !c.runJob(job, nodeName, config, "remove") {
		status := OrchestrationStatus{Status: OrchestrationStatusCompleted, Message: fmt.Sprintf("failed to cleanup osd config on node %s", nodeName)}
		if err := c.updateOSDStatus(nodeName, status); err != nil {
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

	listOpts := metav1.ListOptions{LabelSelector: fmt.Sprintf("app=%s", AppName)}
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
			logger.Debugf("skipping osd %s because osd deployment %s doesn't have a node name on its node selector: %+v", osdDeployment.Name, osdDeployment.Name, osdPodSpec.NodeSelector)
			continue
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
	if err := client.IsClusterCleanError(c.context, c.Namespace); err != nil {
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
	if idstr, ok := deployment.Labels[OsdIdLabelKey]; ok {
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
	rookNode := c.ValidStorage.ResolveNode(nodeName)
	if rookNode == nil {
		return nil
	}
	rookNode.Resources = k8sutil.MergeResourceRequirements(rookNode.Resources, c.resources)

	return rookNode
}

func (c *Cluster) getOSDPropsForPVC(pvcName string) (osdProperties, error) {
	for _, volumeSource := range c.ValidStorage.VolumeSources {
		if pvcName == volumeSource.PersistentVolumeClaimSource.ClaimName {
			osdProps := osdProperties{
				crushHostname: volumeSource.PersistentVolumeClaimSource.ClaimName,
				pvc:           volumeSource.PersistentVolumeClaimSource,
				resources:     volumeSource.Resources,
				placement:     volumeSource.Placement,
				portable:      volumeSource.Portable,
			}
			// If OSD isn't portable, we're getting the host name of the pod where the osd prepare job pod prepared the OSD.
			if !volumeSource.Portable {
				var err error
				osdProps.crushHostname, err = c.getPVCHostName(pvcName)
				if err != nil {
					return osdProperties{}, fmt.Errorf("Unable to get crushHostname of non portable pvc %s. %+v", pvcName, err)
				}
			}
			return osdProps, nil
		}
	}
	return osdProperties{}, fmt.Errorf("No valid VolumeSource found for pvc %s", pvcName)
}

func (c *Cluster) getPVCHostName(pvcName string) (string, error) {
	listOpts := metav1.ListOptions{LabelSelector: fmt.Sprintf("%s=%s", OSDOverPVCLabelKey, pvcName)}
	podList, err := c.context.Clientset.CoreV1().Pods(c.Namespace).List(listOpts)
	if err != nil {
		return "", err
	}
	for _, pod := range podList.Items {
		name, err := k8sutil.GetNodeHostName(c.context.Clientset, pod.Spec.NodeName)
		if err != nil {
			logger.Warningf("falling back to node name %s since hostname not found for node", pod.Spec.NodeName)
			name = pod.Spec.NodeName
		}
		return name, nil
	}
	return "", err
}

func getOSDInfo(d *apps.Deployment) ([]OSDInfo, error) {
	container := d.Spec.Template.Spec.Containers[0]
	var osd OSDInfo

	osdID, err := strconv.Atoi(d.Labels[OsdIdLabelKey])
	if err != nil {
		return []OSDInfo{}, fmt.Errorf("error parsing ceph-osd-id. %+v", err)
	}
	osd.ID = osdID

	for _, envVar := range d.Spec.Template.Spec.Containers[0].Env {
		if envVar.Name == "ROOK_OSD_UUID" {
			osd.UUID = envVar.Value
		}
		if envVar.Name == "ROOK_LV_PATH" {
			osd.LVPath = envVar.Value
		}
	}

	for i, a := range container.Args {
		if strings.HasPrefix(a, "--setuser-match-path") {
			if len(container.Args) >= i+1 {
				osd.DataPath = container.Args[i+1]
				break
			}
		}
	}

	osd.CephVolumeInitiated = true

	if osd.DataPath == "" || osd.UUID == "" || osd.LVPath == "" {
		return []OSDInfo{}, fmt.Errorf("failed to get required osdInfo. %+v", osd)
	}

	return []OSDInfo{osd}, nil
}
