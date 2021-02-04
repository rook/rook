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
	"context"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	"k8s.io/client-go/kubernetes"

	"github.com/banzaicloud/k8s-objectmatcher/patch"
	"github.com/coreos/pkg/capnslog"
	"github.com/pkg/errors"
	cephv1 "github.com/rook/rook/pkg/apis/ceph.rook.io/v1"
	rookv1 "github.com/rook/rook/pkg/apis/rook.io/v1"
	"github.com/rook/rook/pkg/clusterd"
	"github.com/rook/rook/pkg/daemon/ceph/client"
	cephclient "github.com/rook/rook/pkg/daemon/ceph/client"
	kms "github.com/rook/rook/pkg/daemon/ceph/osd/kms"
	"github.com/rook/rook/pkg/operator/ceph/cluster/mon"
	osdconfig "github.com/rook/rook/pkg/operator/ceph/cluster/osd/config"
	opconfig "github.com/rook/rook/pkg/operator/ceph/config"
	"github.com/rook/rook/pkg/operator/ceph/controller"
	cephver "github.com/rook/rook/pkg/operator/ceph/version"
	"github.com/rook/rook/pkg/operator/k8sutil"
	apps "k8s.io/api/apps/v1"
	batch "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	v1 "k8s.io/api/core/v1"
	kerrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/version"
)

var (
	logger                                            = capnslog.NewPackageLogger("github.com/rook/rook", "op-osd")
	updateDeploymentAndWait                           = mon.UpdateCephDeploymentAndWait
	cephVolumeRawEncryptionModeMinNautilusCephVersion = cephver.CephVersion{Major: 14, Minor: 2, Extra: 11}
	cephVolumeRawEncryptionModeMinOctopusCephVersion  = cephver.CephVersion{Major: 15, Minor: 2, Extra: 5}
)

const (
	// AppName is the "app" label on osd pods
	AppName = "rook-ceph-osd"
	// FailureDomainKey is the label key whose value is the failure domain of the OSD
	FailureDomainKey                = "failure-domain"
	prepareAppName                  = "rook-ceph-osd-prepare"
	prepareAppNameFmt               = "rook-ceph-osd-prepare-%s"
	osdAppNameFmt                   = "rook-ceph-osd-%d"
	defaultWaitTimeoutForHealthyOSD = 10 * time.Minute
	// OsdIdLabelKey is the OSD label key
	OsdIdLabelKey                  = "ceph-osd-id"
	serviceAccountName             = "rook-ceph-osd"
	portableKey                    = "portable"
	cephOsdPodMinimumMemory uint64 = 2048 // minimum amount of memory in MB to run the pod
	bluestorePVCMetadata           = "metadata"
	bluestorePVCWal                = "wal"
	bluestorePVCData               = "data"
)

// Cluster keeps track of the OSDs
type Cluster struct {
	context      *clusterd.Context
	clusterInfo  *cephclient.ClusterInfo
	rookVersion  string
	spec         cephv1.ClusterSpec
	ValidStorage rookv1.StorageScopeSpec // valid subset of `Storage`, computed at runtime
	kv           *k8sutil.ConfigMapKVStore
}

// New creates an instance of the OSD manager
func New(context *clusterd.Context, clusterInfo *cephclient.ClusterInfo, spec cephv1.ClusterSpec, rookVersion string) *Cluster {
	return &Cluster{
		context:     context,
		clusterInfo: clusterInfo,
		spec:        spec,
		rookVersion: rookVersion,
		kv:          k8sutil.NewConfigMapKVStore(clusterInfo.Namespace, context.Clientset, clusterInfo.OwnerRef),
	}
}

// OSDInfo represent all the properties of a given OSD
type OSDInfo struct {
	ID             int    `json:"id"`
	Cluster        string `json:"cluster"`
	UUID           string `json:"uuid"`
	DevicePartUUID string `json:"device-part-uuid"`
	// BlockPath is the logical Volume path for an OSD created by Ceph-volume with format '/dev/<Volume Group>/<Logical Volume>' or simply /dev/vdb if block mode is used
	BlockPath     string `json:"lv-path"`
	MetadataPath  string `json:"metadata-path"`
	WalPath       string `json:"wal-path"`
	SkipLVRelease bool   `json:"skip-lv-release"`
	Location      string `json:"location"`
	LVBackedPV    bool   `json:"lv-backed-pv"`
	CVMode        string `json:"lv-mode"`
	Store         string `json:"store"`
}

// OrchestrationStatus represents the status of an OSD orchestration
type OrchestrationStatus struct {
	OSDs         []OSDInfo `json:"osds"`
	Status       string    `json:"status"`
	PvcBackedOSD bool      `json:"pvc-backed-osd"`
	Message      string    `json:"message"`
}

type osdProperties struct {
	//crushHostname refers to the hostname or PVC name when the OSD is provisioned on Nodes or PVC block device, respectively.
	crushHostname       string
	devices             []rookv1.Device
	pvc                 v1.PersistentVolumeClaimVolumeSource
	metadataPVC         v1.PersistentVolumeClaimVolumeSource
	walPVC              v1.PersistentVolumeClaimVolumeSource
	pvcSize             string
	selection           rookv1.Selection
	resources           v1.ResourceRequirements
	storeConfig         osdconfig.StoreConfig
	placement           rookv1.Placement
	preparePlacement    *rookv1.Placement
	metadataDevice      string
	portable            bool
	tuneSlowDeviceClass bool
	tuneFastDeviceClass bool
	schedulerName       string
	encrypted           bool
	deviceSetName       string
	// Drive Groups which apply to the node
	driveGroups cephv1.DriveGroupsSpec
}

func (osdProps osdProperties) onPVC() bool {
	return osdProps.pvc.ClaimName != ""
}

func (osdProps osdProperties) onPVCWithMetadata() bool {
	return osdProps.metadataPVC.ClaimName != ""
}

func (osdProps osdProperties) onPVCWithWal() bool {
	return osdProps.walPVC.ClaimName != ""
}

func (osdProps osdProperties) getPreparePlacement() rookv1.Placement {
	// If the osd prepare placement is specified, use it
	if osdProps.preparePlacement != nil {
		return *osdProps.preparePlacement
	}
	// Fall back to use the same placement as requested for the osd daemons
	return osdProps.placement
}

// Start the osd management
func (c *Cluster) Start() error {
	config := c.newProvisionConfig()

	// Validate pod's memory if specified
	err := controller.CheckPodMemory(cephv1.ResourcesKeyOSD, cephv1.GetOSDResources(c.spec.Resources), cephOsdPodMinimumMemory)
	if err != nil {
		return errors.Wrap(err, "failed to check pod memory")
	}
	logger.Infof("start running osds in namespace %s", c.clusterInfo.Namespace)

	if !c.spec.Storage.UseAllNodes && len(c.spec.Storage.Nodes) == 0 && len(c.spec.Storage.VolumeSources) == 0 && len(c.spec.Storage.StorageClassDeviceSets) == 0 && len(c.spec.DriveGroups) == 0 {
		logger.Warningf("useAllNodes is set to false and no nodes, driveGroups, storageClassDevicesets or volumeSources are specified, no OSD pods are going to be created")
	}

	if c.spec.WaitTimeoutForHealthyOSDInMinutes != 0 {
		c.clusterInfo.OsdUpgradeTimeout = c.spec.WaitTimeoutForHealthyOSDInMinutes * time.Minute
	} else {
		c.clusterInfo.OsdUpgradeTimeout = defaultWaitTimeoutForHealthyOSD
	}
	logger.Infof("wait timeout for healthy OSDs during upgrade or restart is %q", c.clusterInfo.OsdUpgradeTimeout)

	// start the jobs to provision the OSD devices
	logger.Info("start provisioning the osds on PVCs, if needed")
	c.startProvisioningOverPVCs(config)

	if len(config.errorMessages) > 0 {
		return errors.Errorf("%d failures encountered while running osds on PVCs in namespace %q. %v",
			len(config.errorMessages), c.clusterInfo.Namespace, strings.Join(config.errorMessages, "\n"))
	}

	logger.Info("start provisioning the osds on nodes, if needed")
	c.startProvisioningOverNodes(config)

	if len(config.errorMessages) > 0 {
		return errors.Errorf("%d failures encountered while running osds on nodes in namespace %q. %v",
			len(config.errorMessages), c.clusterInfo.Namespace, strings.Join(config.errorMessages, "\n"))
	}

	// The following block is used to apply any command(s) required by an upgrade
	// The block below handles the upgrade from Mimic to Nautilus.
	// This should only run before Octopus
	c.applyUpgradeOSDFunctionality()

	logger.Infof("completed running osds in namespace %s", c.clusterInfo.Namespace)
	return nil
}

func (c *Cluster) startProvisioningOverPVCs(config *provisionConfig) {
	// Parsing storageClassDeviceSets and parsing it to volume sources
	c.spec.Storage.VolumeSources = append(c.spec.Storage.VolumeSources, c.prepareStorageClassDeviceSets(config)...)

	c.ValidStorage.VolumeSources = c.spec.Storage.VolumeSources

	// no validVolumeSource is ready to run an osd
	if len(c.spec.Storage.VolumeSources) == 0 && len(c.spec.Storage.StorageClassDeviceSets) == 0 {
		logger.Info("no volume sources defined to configure OSDs on PVCs.")
		return
	}

	// Check k8s version
	k8sVersion, err := k8sutil.GetK8SVersion(c.context.Clientset)
	if err != nil {
		config.addError("error finding Kubernetes version. %v", err)
		return
	}
	if !k8sVersion.AtLeast(version.MustParseSemantic("v1.13.0")) {
		logger.Warningf("skipping OSD on PVC provisioning. Minimum Kubernetes version required: 1.13.0. Actual version: %s", k8sVersion.String())
		return
	}

	existingDeployments, err := c.getExistingOSDDeploymentsOnPVCs()
	if err != nil {
		config.addError("failed to query existing OSD deployments on PVCs. %v", err)
		return
	}

	for _, volume := range c.ValidStorage.VolumeSources {
		// Check whether we need to cancel the orchestration
		if err := controller.CheckForCancelledOrchestration(c.context); err != nil {
			config.addError("%s", err.Error())
			return
		}

		dataSource, dataOK := volume.PVCSources[bluestorePVCData]

		// The data PVC template is required.
		if !dataOK {
			config.addError("failed to create osd for storageClassDeviceSet %q, missing the data template", volume.Name)
			continue
		}

		metadataSource, metadataOK := volume.PVCSources[bluestorePVCMetadata]
		if metadataOK {
			logger.Infof("OSD will have its main bluestore block on %q and its metadata device on %q", dataSource.ClaimName, metadataSource.ClaimName)
		} else {
			logger.Infof("OSD will have its main bluestore block on %q", dataSource.ClaimName)
		}

		walSource, walOK := volume.PVCSources[bluestorePVCWal]
		if walOK {
			logger.Infof("OSD will have its wal device on %q", walSource.ClaimName)
		}

		osdProps := osdProperties{
			crushHostname:    dataSource.ClaimName,
			pvc:              dataSource,
			metadataPVC:      metadataSource,
			walPVC:           walSource,
			resources:        volume.Resources,
			placement:        volume.Placement,
			preparePlacement: volume.PreparePlacement,
			portable:         volume.Portable,
			schedulerName:    volume.SchedulerName,
			encrypted:        volume.Encrypted,
			deviceSetName:    volume.Name,
		}
		osdProps.storeConfig.DeviceClass = volume.CrushDeviceClass

		logger.Debugf("osdProps are %+v", osdProps)

		if osdProps.encrypted {
			// If the deviceSet template has "encrypted" but the Ceph version is not compatible
			if !c.isCephVolumeRawModeSupported() {
				errMsg := fmt.Sprintf("failed to validate storageClassDeviceSet %q. min required ceph version to support encryption is %q or %q", volume.Name, cephVolumeRawEncryptionModeMinNautilusCephVersion.String(), cephVolumeRawEncryptionModeMinOctopusCephVersion.String())
				config.addError(errMsg)
				continue
			}

			// create encryption Kubernetes Secret if the PVC is encrypted
			key, err := generateDmCryptKey()
			if err != nil {
				errMsg := fmt.Sprintf("failed to generate dmcrypt key for osd claim %q. %v", osdProps.pvc.ClaimName, err)
				config.addError(errMsg)
				continue
			}

			// Initialize the KMS code
			kmsConfig := kms.NewConfig(c.context, &c.spec, c.clusterInfo)

			// We could set an env var in the Operator or a global var instead of the API call?
			// Hopefully, the API is cheap and we can always retrieve the token if it has changed...
			if c.spec.Security.KeyManagementService.IsTokenAuthEnabled() {
				err := kms.SetTokenToEnvVar(c.context, c.spec.Security.KeyManagementService.TokenSecretName, kmsConfig.Provider, c.clusterInfo.Namespace)
				if err != nil {
					errMsg := fmt.Sprintf("failed to fetch kms token secret %q", c.spec.Security.KeyManagementService.TokenSecretName)
					config.addError(errMsg)
					continue
				}
			}

			// Generate and store the encrypted key in whatever KMS is configured
			err = kmsConfig.PutSecret(osdProps.pvc.ClaimName, key)
			if err != nil {
				errMsg := fmt.Sprintf("failed to store secret. %v", err)
				config.addError(errMsg)
				continue
			}
		}

		// Skip OSD prepare if deployment already exists for the PVC
		if osdDeployment, ok := existingDeployments[dataSource.ClaimName]; ok {
			logger.Infof("skip OSD prepare pod creation as OSD daemon already exists for %q", osdProps.crushHostname)
			osds, err := c.getOSDInfo(osdDeployment)
			if err != nil {
				config.addError("failed to get osdInfo for pvc %q. %v", osdProps.crushHostname, err)
				continue
			}
			// Update the orchestration status of this pvc to the completed state
			status := OrchestrationStatus{OSDs: osds, Status: OrchestrationStatusCompleted, PvcBackedOSD: true}
			c.updateOSDStatus(osdProps.crushHostname, status)
			continue
		}

		// Update the orchestration status of this pvc to the starting state
		status := OrchestrationStatus{Status: OrchestrationStatusStarting, PvcBackedOSD: true}
		c.updateOSDStatus(osdProps.crushHostname, status)

		job, err := c.makeJob(osdProps, config)
		if err != nil {
			message := fmt.Sprintf("failed to create prepare job for pvc %s: %v", osdProps.crushHostname, err)
			config.addError(message)
			status := OrchestrationStatus{Status: OrchestrationStatusCompleted, Message: message, PvcBackedOSD: true}
			c.updateOSDStatus(osdProps.crushHostname, status)
		}

		if !c.runJob(job, osdProps.crushHostname, config, "provision") {
			status := OrchestrationStatus{
				Status:       OrchestrationStatusCompleted,
				Message:      fmt.Sprintf("failed to start osd provisioning on pvc %s", osdProps.crushHostname),
				PvcBackedOSD: true,
			}
			c.updateOSDStatus(osdProps.crushHostname, status)
		}
	}
	logger.Infof("start osds after provisioning is completed, if needed")
	c.completeProvision(config)
}

func (c *Cluster) getExistingOSDDeploymentsOnPVCs() (map[string]*apps.Deployment, error) {
	ctx := context.TODO()
	listOpts := metav1.ListOptions{LabelSelector: fmt.Sprintf("%s=%s,%s", k8sutil.AppAttr, AppName, OSDOverPVCLabelKey)}

	deployments, err := c.context.Clientset.AppsV1().Deployments(c.clusterInfo.Namespace).List(ctx, listOpts)
	if err != nil {
		return nil, errors.Wrap(err, "failed to query existing OSD deployments")
	}

	result := map[string]*apps.Deployment{}
	for i, deployment := range deployments.Items {
		if pvcID, ok := deployment.Labels[OSDOverPVCLabelKey]; ok {
			result[pvcID] = &deployments.Items[i]
		}
	}

	return result, nil
}

func (c *Cluster) startProvisioningOverNodes(config *provisionConfig) {
	if len(c.spec.DataDirHostPath) == 0 {
		logger.Warningf("skipping osd provisioning where no dataDirHostPath is set")
		return
	}

	// Only provision nodes with either the 'storage' config or the 'driveGroups'; not both
	// If Drive Groups are configured, always use those
	// This should not apply to OSDs on PVCs; those can be configured alongside Drive Groups
	if len(c.spec.DriveGroups) > 0 {
		c.startNodeDriveGroupProvisioners(config)
	} else {
		c.startNodeStorageProvisioners(config)
	}

	logger.Infof("start osds after provisioning is completed, if needed")
	c.completeProvision(config)
}

func (c *Cluster) startNodeStorageProvisioners(config *provisionConfig) {
	logger.Debug("starting provisioning on nodes using storage config")

	if c.spec.Storage.UseAllNodes {
		if len(c.spec.Storage.Nodes) > 0 {
			logger.Warningf("useAllNodes is TRUE, but nodes are specified. NODES in the cluster CR will be IGNORED unless useAllNodes is FALSE.")
		}

		// Get the list of all nodes in the cluster. The placement settings will be applied below.
		hostnameMap, err := k8sutil.GetNodeHostNames(c.context.Clientset)
		if err != nil {
			config.addError("failed to get node hostnames: %v", err)
			return
		}
		c.spec.Storage.Nodes = nil
		for _, hostname := range hostnameMap {
			storageNode := rookv1.Node{
				Name: hostname,
			}
			c.spec.Storage.Nodes = append(c.spec.Storage.Nodes, storageNode)
		}
		logger.Debugf("storage nodes: %+v", c.spec.Storage.Nodes)
	}
	// generally speaking, this finds nodes which are capable of running new osds
	validNodes := k8sutil.GetValidNodes(c.spec.Storage, c.context.Clientset, cephv1.GetOSDPlacement(c.spec.Placement))

	logger.Infof("%d of the %d storage nodes are valid", len(validNodes), len(c.spec.Storage.Nodes))

	c.ValidStorage = *c.spec.Storage.DeepCopy()
	c.ValidStorage.Nodes = validNodes

	// no valid node is ready to run an osd
	if len(validNodes) == 0 {
		logger.Warningf("no valid nodes available to run osds on nodes in namespace %s", c.clusterInfo.Namespace)
		return
	}

	// start with nodes currently in the storage spec
	for _, node := range c.ValidStorage.Nodes {
		// Check whether we need to cancel the orchestration
		if err := controller.CheckForCancelledOrchestration(c.context); err != nil {
			config.addError("%s", err.Error())
			return
		}

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
		c.makeAndRunJob(n.Name, "provision", osdProps, config)
	}
}

func (c *Cluster) startNodeDriveGroupProvisioners(config *provisionConfig) {
	ctx := context.TODO()
	logger.Debug("starting provisioning on nodes using Drive Groups config")

	if c.spec.Storage.UseAllNodes {
		logger.Warningf("The user has specified useAllNodes=true in the storage config. This will be ignored because driveGroups are configured.")
	}
	if len(c.spec.Storage.Nodes) > 0 {
		logger.Warningf("The user has specified nodes in the storage config. This will be ignored because driveGroups are configured.")
	}

	nodes, err := c.context.Clientset.CoreV1().Nodes().List(ctx, metav1.ListOptions{})
	if err != nil {
		config.addError("failed to get all nodes: %v", err)
		return
	}

	// With Drive Groups, we effectively treat it as though the user has specified
	// 'useAllNodes: true` because we want the Drive Groups' placements to determine whether the
	// DGroups are configured on a node
	c.spec.Storage.Nodes = nil

	sanitizedDGs := SanitizeDriveGroups(c.spec.DriveGroups)

	// Drive Groups should considered on every node in the k8s cluster; each drive group's
	// 'placement' should be the selector for placement across all of K8s' nodes and not be affected
	// by node selections in the 'storage' config
	for _, n := range nodes.Items {
		normalizedHostname := k8sutil.GetNormalizedHostname(n)
		storageNode := rookv1.Node{
			Name: normalizedHostname,
		}
		c.spec.Storage.Nodes = append(c.spec.Storage.Nodes, storageNode)
		localnode := n
		groups, err := DriveGroupsWithPlacementMatchingNode(sanitizedDGs, &localnode)
		if err != nil {
			config.addError("failed to determine drive groups with placement matching node %q (hostname: %q): %+v", n.Name, normalizedHostname, err)
			continue
		}

		if len(groups) == 0 {
			logger.Debugf("skipping drive group provisioning on node %q (hostname: %q). no drive groups match node or node is not ready/schedulable", n.Name, normalizedHostname)
			continue
		}

		osdProps := osdProperties{
			crushHostname: normalizedHostname,
			driveGroups:   groups,
		}
		c.makeAndRunJob(normalizedHostname, "provision drive groups", osdProps, config)
	}

	// With Drive Groups, any node *could* be valid, and we need to do this so nodes resolve when
	// starting OSD daemons. Each DGroup's individual placement will determine if the group is valid
	// for a given node.
	c.ValidStorage = *c.spec.Storage.DeepCopy()
}

func (c *Cluster) makeAndRunJob(nodeName, action string, osdProps osdProperties, config *provisionConfig) {
	// update the orchestration status of this node to the starting state
	status := OrchestrationStatus{Status: OrchestrationStatusStarting}
	c.updateOSDStatus(nodeName, status)

	job, err := c.makeJob(osdProps, config)
	if err != nil {
		message := fmt.Sprintf("failed to create OSD %s job for node %q. %v", action, nodeName, err)
		config.addError(message)
		status := OrchestrationStatus{Status: OrchestrationStatusCompleted, Message: message}
		c.updateOSDStatus(nodeName, status)
	}

	if !c.runJob(job, nodeName, config, action) {
		status := OrchestrationStatus{Status: OrchestrationStatusCompleted, Message: fmt.Sprintf("failed to start osd %s job on node %s", action, nodeName)}
		c.updateOSDStatus(nodeName, status)
	}
}

func (c *Cluster) runJob(job *batch.Job, nodeName string, config *provisionConfig, action string) bool {
	if err := k8sutil.RunReplaceableJob(c.context.Clientset, job, false); err != nil {
		if !kerrors.IsAlreadyExists(err) {
			// we failed to create job, update the orchestration status for this node
			message := fmt.Sprintf("failed to create %q job for node %q. %v", action, nodeName, err)
			c.handleOrchestrationFailure(config, nodeName, message)
			return false
		}

		// the job is already in progress so we will let it run to completion
	}

	logger.Infof("osd %s job started for node %s", action, nodeName)
	return true
}

func (c *Cluster) startOSDDaemonsOnPVC(pvcName string, config *provisionConfig, configMap *v1.ConfigMap, status *OrchestrationStatus) {
	ctx := context.TODO()
	osds := status.OSDs
	logger.Infof("starting %d osd daemons on pvc %s", len(osds), pvcName)
	osdProps, err := c.getOSDPropsForPVC(pvcName)
	if err != nil {
		config.addError(fmt.Sprintf("%v", err))
		return
	}

	// start osds
	for _, osd := range osds {
		logger.Debugf("start osd %v", osd)

		// keyring must be generated before deployment creation in order to avoid a race condition resulting
		// in intermittent failure of first-attempt OSD pods.
		_, err := c.generateKeyring(osd.ID)
		if err != nil {
			errMsg := fmt.Sprintf("failed to create keyring for pvc %q, osd %v. %v", osdProps.crushHostname, osd, err)
			config.addError(errMsg)
			continue
		}

		dp, err := c.makeDeployment(osdProps, osd, config)
		if err != nil {
			errMsg := fmt.Sprintf("failed to create deployment for pvc %q. %v", osdProps.crushHostname, err)
			config.addError(errMsg)
			continue
		}

		// Set the deployment hash as an annotation
		err = patch.DefaultAnnotator.SetLastAppliedAnnotation(dp)
		if err != nil {
			errMsg := fmt.Sprintf("failed to set annotation for deployment %q. %v", dp.Name, err)
			config.addError(errMsg)
			continue
		}

		_, createErr := c.context.Clientset.AppsV1().Deployments(c.clusterInfo.Namespace).Create(ctx, dp, metav1.CreateOptions{})
		if createErr != nil {
			if kerrors.IsAlreadyExists(createErr) {
				logger.Infof("deployment for osd %d already exists. updating if needed", osd.ID)
				if err = updateDeploymentAndWait(c.context, c.clusterInfo, dp, opconfig.OsdType, strconv.Itoa(osd.ID), c.spec.SkipUpgradeChecks, c.spec.ContinueUpgradeAfterChecksEvenIfNotHealthy); err != nil {
					logger.Errorf("failed to update osd deployment %d. %v", osd.ID, err)
				}
			} else {
				// we failed to create job, update the orchestration status for this pvc
				logger.Warningf("failed to create osd deployment for pvc %q, osd %v. %v", osdProps.pvc.ClaimName, osd, createErr)
				continue
			}
		}

		if createErr != nil && kerrors.IsAlreadyExists(createErr) {
			if err = updateDeploymentAndWait(c.context, c.clusterInfo, dp, opconfig.OsdType, strconv.Itoa(osd.ID), c.spec.SkipUpgradeChecks, c.spec.ContinueUpgradeAfterChecksEvenIfNotHealthy); err != nil {
				logger.Errorf("failed to update osd deployment %d. %v", osd.ID, err)
			}
		}
		logger.Infof("started deployment for osd %d on pvc", osd.ID)
	}
}

func (c *Cluster) startOSDDaemonsOnNode(nodeName string, config *provisionConfig, configMap *v1.ConfigMap, status *OrchestrationStatus) {
	ctx := context.TODO()
	osds := status.OSDs
	logger.Infof("starting %d osd daemons on node %s", len(osds), nodeName)

	// fully resolve the storage config and resources for this node
	n := c.resolveNode(nodeName)
	if n == nil {
		logger.Errorf("node %q did not resolve to start osds", nodeName)
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
		opconfig.ConditionExport(c.context, c.clusterInfo.NamespacedName(), cephv1.ConditionProgressing, v1.ConditionTrue, "ClusterProgressing", fmt.Sprintf("Processing node %s osd %d", nodeName, osd.ID))

		// keyring must be generated before deployment creation in order to avoid a race condition resulting
		// in intermittent failure of first-attempt OSD pods.
		_, err := c.generateKeyring(osd.ID)
		if err != nil {
			errMsg := fmt.Sprintf("failed to create keyring for node %q, osd %v. %v", n.Name, osd, err)
			config.addError(errMsg)
			continue
		}

		dp, err := c.makeDeployment(osdProps, osd, config)
		if err != nil {
			errMsg := fmt.Sprintf("failed to create deployment for node %s: %v", n.Name, err)
			config.addError(errMsg)
			continue
		}

		// Set the deployment hash as an annotation
		err = patch.DefaultAnnotator.SetLastAppliedAnnotation(dp)
		if err != nil {
			errMsg := fmt.Sprintf("failed to set annotation for deployment %q. %v", dp.Name, err)
			config.addError(errMsg)
			continue
		}

		_, createErr := c.context.Clientset.AppsV1().Deployments(c.clusterInfo.Namespace).Create(ctx, dp, metav1.CreateOptions{})
		if createErr != nil {
			if kerrors.IsAlreadyExists(createErr) {
				logger.Debugf("deployment for osd %d already exists. updating if needed", osd.ID)
				if err = updateDeploymentAndWait(c.context, c.clusterInfo, dp, opconfig.OsdType, strconv.Itoa(osd.ID), c.spec.SkipUpgradeChecks, c.spec.ContinueUpgradeAfterChecksEvenIfNotHealthy); err != nil {
					logger.Errorf("failed to update osd deployment %d. %v", osd.ID, err)
				}
			} else {
				// we failed to create job, update the orchestration status for this pvc
				logger.Warningf("failed to create osd deployment for node %q, osd %+v. %v", n.Name, osd, createErr)
				continue
			}
		} else {
			logger.Infof("created deployment for osd %d", osd.ID)
		}
	}
}

func (c *Cluster) resolveNode(nodeName string) *rookv1.Node {
	// fully resolve the storage config and resources for this node
	rookNode := c.ValidStorage.ResolveNode(nodeName)
	if rookNode == nil {
		return nil
	}
	rookNode.Resources = k8sutil.MergeResourceRequirements(rookNode.Resources, cephv1.GetOSDResources(c.spec.Resources))

	return rookNode
}

func (c *Cluster) getOSDPropsForPVC(pvcName string) (osdProperties, error) {
	for _, volumeSource := range c.ValidStorage.VolumeSources {
		// The data PVC template is required.
		dataSource, dataOK := volumeSource.PVCSources[bluestorePVCData]
		if !dataOK {
			logger.Warningf("failed to find data source daemon for device set %q, missing the data template", volumeSource.Name)
			continue
		}

		if pvcName == dataSource.ClaimName {
			metadataSource, metadataOK := volumeSource.PVCSources[bluestorePVCMetadata]
			if metadataOK {
				logger.Infof("OSD will have its main bluestore block on %q and its metadata device on %q", dataSource.ClaimName, metadataSource.ClaimName)
			} else {
				logger.Infof("OSD will have its main bluestore block on %q", dataSource.ClaimName)
			}

			walSource, walOK := volumeSource.PVCSources[bluestorePVCWal]
			if walOK {
				logger.Infof("OSD will have its wal device on %q", walSource.ClaimName)
			}

			if volumeSource.Resources.Limits == nil && volumeSource.Resources.Requests == nil {
				volumeSource.Resources = cephv1.GetOSDResources(c.spec.Resources)
			}

			osdProps := osdProperties{
				crushHostname:       dataSource.ClaimName,
				pvc:                 dataSource,
				metadataPVC:         metadataSource,
				walPVC:              walSource,
				resources:           volumeSource.Resources,
				placement:           volumeSource.Placement,
				preparePlacement:    volumeSource.PreparePlacement,
				portable:            volumeSource.Portable,
				tuneSlowDeviceClass: volumeSource.TuneSlowDeviceClass,
				tuneFastDeviceClass: volumeSource.TuneFastDeviceClass,
				pvcSize:             volumeSource.Size,
				schedulerName:       volumeSource.SchedulerName,
				encrypted:           volumeSource.Encrypted,
				deviceSetName:       volumeSource.Name,
			}
			// If OSD isn't portable, we're getting the host name either from the osd deployment that was already initialized
			// or from the osd prepare job from initial creation.
			if !volumeSource.Portable {
				var err error
				osdProps.crushHostname, err = c.getPVCHostName(pvcName)
				if err != nil {
					return osdProperties{}, errors.Wrapf(err, "Unable to get crushHostname of non portable pvc %s", pvcName)
				}
			}
			return osdProps, nil
		}
	}
	return osdProperties{}, errors.Errorf("no valid VolumeSource found for pvc %s", pvcName)
}

// getPVCHostName finds the node where an OSD pod should be assigned with a node selector.
// First look for the node selector that was previously used for the OSD, or if a new OSD
// check for the assignment of the OSD prepare job.
func (c *Cluster) getPVCHostName(pvcName string) (string, error) {
	ctx := context.TODO()
	listOpts := metav1.ListOptions{LabelSelector: fmt.Sprintf("%s=%s", OSDOverPVCLabelKey, pvcName)}

	// Check for the existence of the OSD deployment where the node selector was applied
	// in a previous reconcile.
	deployments, err := c.context.Clientset.AppsV1().Deployments(c.clusterInfo.Namespace).List(ctx, listOpts)
	if err != nil {
		return "", errors.Wrapf(err, "failed to get deployment for osd with pvc %q", pvcName)
	}
	for _, d := range deployments.Items {
		selectors := d.Spec.Template.Spec.NodeSelector
		for label, value := range selectors {
			if label == v1.LabelHostname {
				return value, nil
			}
		}
	}

	// Since the deployment wasn't found it must be a new deployment so look at the node
	// assignment of the OSD prepare pod
	pods, err := c.context.Clientset.CoreV1().Pods(c.clusterInfo.Namespace).List(ctx, listOpts)
	if err != nil {
		return "", errors.Wrapf(err, "failed to get pod for osd with pvc %q", pvcName)
	}
	for _, pod := range pods.Items {
		name, err := k8sutil.GetNodeHostName(c.context.Clientset, pod.Spec.NodeName)
		if err != nil {
			logger.Warningf("falling back to node name %s since hostname not found for node", pod.Spec.NodeName)
			name = pod.Spec.NodeName
		}
		if name == "" {
			return "", errors.Errorf("node name not found on the osd pod %q", pod.Name)
		}
		return name, nil //nolint, no need for else statement
	}

	return "", errors.Errorf("node selector not found on deployment for osd with pvc %q", pvcName)
}

func (c *Cluster) getOSDInfo(d *apps.Deployment) ([]OSDInfo, error) {
	container := d.Spec.Template.Spec.Containers[0]
	var osd OSDInfo

	osdID, err := strconv.Atoi(d.Labels[OsdIdLabelKey])
	if err != nil {
		return []OSDInfo{}, errors.Wrap(err, "error parsing ceph-osd-id")
	}
	osd.ID = osdID

	for _, envVar := range d.Spec.Template.Spec.Containers[0].Env {
		if envVar.Name == "ROOK_OSD_UUID" {
			osd.UUID = envVar.Value
		}
		if envVar.Name == "ROOK_BLOCK_PATH" || envVar.Name == "ROOK_LV_PATH" {
			osd.BlockPath = envVar.Value
		}
		if envVar.Name == "ROOK_CV_MODE" {
			osd.CVMode = envVar.Value
		}
		if envVar.Name == "ROOK_LV_BACKED_PV" {
			lvBackedPV, err := strconv.ParseBool(envVar.Value)
			if err != nil {
				return []OSDInfo{}, errors.Wrap(err, "error parsing ROOK_LV_BACKED_PV")
			}
			osd.LVBackedPV = lvBackedPV
		}
		if envVar.Name == osdMetadataDeviceEnvVarName {
			osd.MetadataPath = envVar.Value
		}
		if envVar.Name == osdWalDeviceEnvVarName {
			osd.WalPath = envVar.Value
		}
	}

	// If CVMode is empty, this likely means we upgraded Rook
	// This property did not exist before so we need to initialize it
	if osd.CVMode == "" {
		osd.CVMode = "lvm"
	}

	locationFound := false
	for _, a := range container.Args {
		locationPrefix := "--crush-location="
		if strings.HasPrefix(a, locationPrefix) {
			locationFound = true
			// Extract the same CRUSH location as originally determined by the OSD prepare pod
			// by cutting off the prefix: --crush-location=
			osd.Location = a[len(locationPrefix):]
		}
	}

	if !locationFound {
		location, err := getLocationFromPod(c.context.Clientset, d, client.GetCrushRootFromSpec(&c.spec))
		if err != nil {
			logger.Errorf("failed to get location. %v", err)
		} else {
			osd.Location = location
		}
	}

	if osd.UUID == "" || osd.BlockPath == "" {
		return []OSDInfo{}, errors.Errorf("failed to get required osdInfo. %+v", osd)
	}

	return []OSDInfo{osd}, nil
}

func getLocationFromPod(clientset kubernetes.Interface, d *apps.Deployment, crushRoot string) (string, error) {
	ctx := context.TODO()
	pods, err := clientset.CoreV1().Pods(d.Namespace).List(ctx, metav1.ListOptions{LabelSelector: fmt.Sprintf("%s=%s", OsdIdLabelKey, d.Labels[OsdIdLabelKey])})
	if err != nil || len(pods.Items) == 0 {
		return "", err
	}
	nodeName := pods.Items[0].Spec.NodeName
	hostName, err := k8sutil.GetNodeHostName(clientset, nodeName)
	if err != nil {
		return "", err
	}
	portable, ok := d.GetLabels()[portableKey]
	if ok && portable == "true" {
		pvcName, ok := d.GetLabels()[OSDOverPVCLabelKey]
		if ok {
			hostName = pvcName
		}
	}
	return GetLocationWithNode(clientset, nodeName, crushRoot, hostName)
}

func GetLocationWithNode(clientset kubernetes.Interface, nodeName string, crushRoot, crushHostname string) (string, error) {

	node, err := getNode(clientset, nodeName)
	if err != nil {
		return "", errors.Wrapf(err, "could not get the node for topology labels")
	}

	// If the operator did not pass a host name, look up the hostname label.
	// This happens when the operator doesn't know on what node the osd will be assigned (non-portable PVCs).
	if crushHostname == "" {
		crushHostname, err = k8sutil.GetNodeHostNameLabel(node)
		if err != nil {
			return "", errors.Wrapf(err, "failed to get the host name label for node %q", node.Name)
		}
	}

	// Start with the host name in the CRUSH map
	// Keep the fully qualified host name in the crush map, but replace the dots with dashes to satisfy ceph
	hostName := client.NormalizeCrushName(crushHostname)
	locArgs := []string{fmt.Sprintf("root=%s", crushRoot), fmt.Sprintf("host=%s", hostName)}

	nodeLabels := node.GetLabels()
	UpdateLocationWithNodeLabels(&locArgs, nodeLabels)

	loc := strings.Join(locArgs, " ")
	logger.Infof("CRUSH location=%s", loc)
	return loc, nil
}

// getNode will try to get the node object for the provided nodeName
// it will try using the node's name it's hostname label
func getNode(clientset kubernetes.Interface, nodeName string) (*corev1.Node, error) {
	ctx := context.TODO()
	var node *corev1.Node
	var err error
	// try to find by the node by matching the provided nodeName
	node, err = clientset.CoreV1().Nodes().Get(ctx, nodeName, metav1.GetOptions{})
	if kerrors.IsNotFound(err) {
		listOpts := metav1.ListOptions{LabelSelector: fmt.Sprintf("%q=%q", corev1.LabelHostname, nodeName)}
		nodeList, err := clientset.CoreV1().Nodes().List(ctx, listOpts)
		if err != nil || len(nodeList.Items) < 1 {
			return nil, errors.Wrapf(err, "could not find node %q hostname label", nodeName)
		}
		return &nodeList.Items[0], nil
	} else if err != nil {
		return nil, errors.Wrapf(err, "could not find node %q by name", nodeName)
	}

	return node, nil
}

func UpdateLocationWithNodeLabels(location *[]string, nodeLabels map[string]string) {

	topology := ExtractOSDTopologyFromLabels(nodeLabels)

	keys := make([]string, 0, len(topology))
	for k := range topology {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	for _, topologyType := range keys {
		if topologyType != "host" {
			client.UpdateCrushMapValue(location, topologyType, topology[topologyType])
		}
	}
}

func (c *Cluster) applyUpgradeOSDFunctionality() {
	var osdVersion *cephver.CephVersion

	// Get all the daemons versions
	versions, err := client.GetAllCephDaemonVersions(c.context, c.clusterInfo)
	if err != nil {
		logger.Warningf("failed to get ceph daemons versions; this likely means there are no osds yet. %v", err)
		return
	}

	// If length is one, this clearly indicates that all the osds are running the same version
	// If this is the first time we are creating a cluster length will be 0
	// On an initial OSD bootstrap, by the time we reach this code, the OSDs haven't registered yet
	// Basically, this task is happening too quickly and OSD pods are not running yet.
	// That's not an issue since it's an initial bootstrap and not an update.
	if len(versions.Osd) == 1 {
		for v := range versions.Osd {
			osdVersion, err = cephver.ExtractCephVersion(v)
			if err != nil {
				logger.Warningf("failed to extract ceph version. %v", err)
				return
			}
			// if the version of these OSDs is Octopus then we run the command
			if osdVersion.IsOctopus() {
				err = client.EnableReleaseOSDFunctionality(c.context, c.clusterInfo, "octopus")
				if err != nil {
					logger.Warningf("failed to enable new osd functionality. %v", err)
					return
				}
			}
		}
	}
}
