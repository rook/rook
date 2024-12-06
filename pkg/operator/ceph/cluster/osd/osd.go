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
	"bufio"
	"context"
	"fmt"
	"reflect"
	"sort"
	"strconv"
	"strings"
	"time"

	cephv1 "github.com/rook/rook/pkg/apis/ceph.rook.io/v1"
	"github.com/rook/rook/pkg/clusterd"
	cephclient "github.com/rook/rook/pkg/daemon/ceph/client"
	osdconfig "github.com/rook/rook/pkg/operator/ceph/cluster/osd/config"
	"github.com/rook/rook/pkg/operator/ceph/cluster/osd/topology"
	"github.com/rook/rook/pkg/operator/ceph/controller"
	"github.com/rook/rook/pkg/operator/ceph/reporting"
	cephver "github.com/rook/rook/pkg/operator/ceph/version"
	"github.com/rook/rook/pkg/operator/k8sutil"
	"github.com/rook/rook/pkg/util"

	"github.com/coreos/pkg/capnslog"
	"github.com/pkg/errors"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	kerrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/client-go/kubernetes"
)

var (
	logger                   = capnslog.NewPackageLogger("github.com/rook/rook", "op-osd")
	waitForHealthyPGInterval = 10 * time.Second
	waitForHealthyPGTimeout  = 15 * time.Minute
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
	deviceClass                    = "device-class"
	osdStore                       = "osd-store"
	deviceType                     = "device-type"
	encrypted                      = "encrypted"
)

// Cluster keeps track of the OSDs
type Cluster struct {
	context        *clusterd.Context
	clusterInfo    *cephclient.ClusterInfo
	rookVersion    string
	spec           cephv1.ClusterSpec
	ValidStorage   cephv1.StorageScopeSpec // valid subset of `Storage`, computed at runtime
	kv             *k8sutil.ConfigMapKVStore
	deviceSets     []deviceSet
	migrateOSD     *OSDInfo
	deprecatedOSDs map[string][]int
}

// New creates an instance of the OSD manager
func New(context *clusterd.Context, clusterInfo *cephclient.ClusterInfo, spec cephv1.ClusterSpec, rookVersion string) *Cluster {
	return &Cluster{
		context:     context,
		clusterInfo: clusterInfo,
		spec:        spec,
		rookVersion: rookVersion,
		kv:          k8sutil.NewConfigMapKVStore(clusterInfo.Namespace, context.Clientset, clusterInfo.OwnerInfo),
	}
}

// OSDInfo represent all the properties of a given OSD
type OSDInfo struct {
	ID             int    `json:"id"`
	Cluster        string `json:"cluster"`
	UUID           string `json:"uuid"`
	DevicePartUUID string `json:"device-part-uuid"`
	DeviceClass    string `json:"device-class"`
	// BlockPath is the logical Volume path for an OSD created by Ceph-volume with format '/dev/<Volume Group>/<Logical Volume>' or simply /dev/vdb if block mode is used
	BlockPath     string `json:"lv-path"`
	MetadataPath  string `json:"metadata-path"`
	WalPath       string `json:"wal-path"`
	SkipLVRelease bool   `json:"skip-lv-release"`
	Location      string `json:"location"`
	LVBackedPV    bool   `json:"lv-backed-pv"`
	CVMode        string `json:"lv-mode"`
	Store         string `json:"store"`
	// Ensure the OSD daemon has affinity with the same topology from the OSD prepare pod
	TopologyAffinity string `json:"topologyAffinity"`
	Encrypted        bool   `json:"encrypted"`
	ExportService    bool   `json:"exportService"`
	NodeName         string `json:"nodeName"`
	PVCName          string `json:"pvcName"`
	DeviceType       string `json:"device-type"`
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
	devices             []cephv1.Device
	pvc                 corev1.PersistentVolumeClaimVolumeSource
	metadataPVC         corev1.PersistentVolumeClaimVolumeSource
	walPVC              corev1.PersistentVolumeClaimVolumeSource
	pvcSize             string
	selection           cephv1.Selection
	resources           corev1.ResourceRequirements
	storeConfig         osdconfig.StoreConfig
	placement           cephv1.Placement
	preparePlacement    *cephv1.Placement
	metadataDevice      string
	portable            bool
	tuneSlowDeviceClass bool
	tuneFastDeviceClass bool
	schedulerName       string
	encrypted           bool
	deviceSetName       string
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

func (osdProps osdProperties) getPreparePlacement() cephv1.Placement {
	// If the osd prepare placement is specified, use it
	if osdProps.preparePlacement != nil {
		return *osdProps.preparePlacement
	}
	// Fall back to use the same placement as requested for the osd daemons
	return osdProps.placement
}

func (c *Cluster) validateOSDSettings() error {
	// Validate pod's memory if specified
	for resourceKey, resourceValue := range c.spec.Resources {
		if strings.HasPrefix(resourceKey, cephv1.ResourcesKeyOSD) {
			err := controller.CheckPodMemory(resourceKey, resourceValue, cephOsdPodMinimumMemory)
			if err != nil {
				return errors.Wrap(err, "failed to check pod memory")
			}
		}
	}
	deviceSetNames := map[string]bool{}
	for _, deviceSet := range c.spec.Storage.StorageClassDeviceSets {
		if deviceSetNames[deviceSet.Name] {
			return errors.Errorf("device set %q name is duplicated, OSDs cannot be configured", deviceSet.Name)
		}
		deviceSetNames[deviceSet.Name] = true
	}
	return nil
}

// Start the osd management
func (c *Cluster) Start() error {
	namespace := c.clusterInfo.Namespace
	config := c.newProvisionConfig()
	errs := newProvisionErrors()

	if err := c.validateOSDSettings(); err != nil {
		return err
	}
	logger.Infof("start running osds in namespace %q", namespace)

	if !c.spec.Storage.UseAllNodes && len(c.spec.Storage.Nodes) == 0 && len(c.spec.Storage.StorageClassDeviceSets) == 0 {
		logger.Warningf("useAllNodes is set to false and no nodes, storageClassDevicesets or volumeSources are specified, no OSD pods are going to be created")
	}

	if c.spec.WaitTimeoutForHealthyOSDInMinutes != 0 {
		c.clusterInfo.OsdUpgradeTimeout = c.spec.WaitTimeoutForHealthyOSDInMinutes * time.Minute
	} else {
		c.clusterInfo.OsdUpgradeTimeout = defaultWaitTimeoutForHealthyOSD
	}
	logger.Infof("wait timeout for healthy OSDs during upgrade or restart is %q", c.clusterInfo.OsdUpgradeTimeout)

	osdsToSkipReconcile, err := controller.GetDaemonsToSkipReconcile(c.clusterInfo.Context, c.context, c.clusterInfo.Namespace, OsdIdLabelKey, AppName)
	if err != nil {
		logger.Warningf("failed to get osds to skip reconcile. %v", err)
	}

	migrationConfig, err := c.startOSDMigration()
	if err != nil {
		return errors.Wrapf(err, "failed to start OSD migration")
	}

	// prepare for updating existing OSDs
	updateQueue, deployments, err := c.getOSDUpdateInfo(errs)
	if err != nil {
		return errors.Wrapf(err, "failed to get information about currently-running OSD Deployments in namespace %q", namespace)
	}

	if migrationConfig != nil {
		if len(migrationConfig.osds) != 0 {
			// prevent upgrade of OSDs that require migration
			updateQueue.Remove(migrationConfig.getOSDIds())
		}
	}

	logger.Debugf("%d of %d OSD Deployments need update", updateQueue.Len(), deployments.Len())
	updateConfig := c.newUpdateConfig(config, updateQueue, deployments, osdsToSkipReconcile)

	// prepare for creating new OSDs
	statusConfigMaps := sets.New[string]()

	logger.Info("start provisioning the OSDs on PVCs, if needed")
	pvcConfigMaps, err := c.startProvisioningOverPVCs(config, errs)
	if err != nil {
		return err
	}
	statusConfigMaps = statusConfigMaps.Union(pvcConfigMaps)

	logger.Info("start provisioning the OSDs on nodes, if needed")
	nodeConfigMaps, err := c.startProvisioningOverNodes(config, errs)
	if err != nil {
		return err
	}
	statusConfigMaps = statusConfigMaps.Union(nodeConfigMaps)

	createConfig := c.newCreateConfig(config, statusConfigMaps, deployments)

	// do the update and create operations
	err = c.updateAndCreateOSDs(createConfig, updateConfig, errs)
	if err != nil {
		return errors.Wrapf(err, "failed to update/create OSDs")
	}

	if errs.len() > 0 {
		return errors.Errorf("%d failures encountered while running osds on nodes in namespace %q. %s",
			errs.len(), namespace, errs.asMessages())
	}

	// clean up status configmaps that might be dangling from previous reconciles
	// for example, if the storage spec changed from or a node failed in a previous failed reconcile
	c.deleteAllStatusConfigMaps()

	// The following block is used to apply any command(s) required by an upgrade
	c.applyUpgradeOSDFunctionality()

	err = c.reconcileKeyRotationCronJob()
	if err != nil {
		return errors.Wrapf(err, "failed to reconcile key rotation cron jobs")
	}

	err = c.postReconcileUpdateOSDProperties(updateConfig.osdDesiredState)
	if err != nil {
		return errors.Wrap(err, "failed post reconcile of osd properties")
	}

	err = c.updateCephStorageStatus()
	if err != nil {
		return errors.Wrapf(err, "failed to update ceph storage status")
	}

	logger.Infof("finished running OSDs in namespace %q", namespace)
	return nil
}

func (c *Cluster) startOSDMigration() (*migrationConfig, error) {
	if !c.isMigrationRequested() {
		logger.Debug("no OSD migration is requested")
		return nil, nil
	}

	logger.Info("osd migration is requested")

	// start migration only if PGs are active+clean
	pgsHealhty, err := c.waitForHealthyPGs()
	if err != nil {
		return nil, errors.Wrapf(err, "failed to wait for pgs to be healthy")
	}

	if !pgsHealhty {
		return nil, errors.Wrapf(err, "failed to start migration due to unhealthy PGs")
	}

	// skip migration if previously migrated OSD is not up yet.
	migrationComplete, err := isLastOSDMigrationComplete(c)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to check if the last migration was successful or not")
	}

	if !migrationComplete {
		return nil, errors.Wrapf(err, "migration of the last OSD is not complete")
	}

	migrationConfig, err := c.newMigrationConfig()
	if err != nil {
		return nil, errors.Wrapf(err, "failed to get new OSD migration config")
	}

	// delete deployment of the osd that needs migration
	if migrationConfig != nil && len(migrationConfig.osds) > 0 {
		osdToMigrate := migrationConfig.getOSDToMigrate()
		logger.Infof("deleting OSD.%d deployment for migration ", osdToMigrate.ID)
		err = c.deleteOSDDeployment(osdToMigrate.ID)
		if err != nil {
			return nil, errors.Wrapf(err, "failed to delete deployment for osd.%d that needs migration %q", osdToMigrate.ID, c.clusterInfo.Namespace)
		}
		err = saveMigrationConfig(c.context, c.clusterInfo, osdToMigrate.ID)
		if err != nil {
			return nil, errors.Wrapf(err, "failed to save migrated OSD ID %din the config map", osdToMigrate.ID)
		}
		c.migrateOSD = osdToMigrate
	}

	return migrationConfig, nil
}

func (c *Cluster) isMigrationRequested() bool {
	// check for OSDUpdateStoreConfirmation as well for backwards compatibility
	if c.spec.Storage.Migration.Confirmation == OSDMigrationConfirmation || c.spec.Storage.Store.UpdateStore == OSDUpdateStoreConfirmation {
		return true
	}
	return false
}

func (c *Cluster) postReconcileUpdateOSDProperties(desiredOSDs map[int]*OSDInfo) error {
	osdUsage, err := cephclient.GetOSDUsage(c.context, c.clusterInfo)
	if err != nil {
		return errors.Wrap(err, "failed to get osd usage")
	}
	logger.Debugf("post processing osd properties with %d actual osds from ceph osd df and %d existing osds found during reconcile", len(osdUsage.OSDNodes), len(desiredOSDs))
	for _, actualOSD := range osdUsage.OSDNodes {
		if c.spec.Storage.AllowOsdCrushWeightUpdate {
			_, err := cephclient.ResizeOsdCrushWeight(actualOSD, c.context, c.clusterInfo)
			if err != nil {
				// Log the error and allow other updates to continue
				logger.Errorf("failed to resize osd crush weight on cluster in namespace %s: %v", c.clusterInfo.Namespace, err)
			}
		}

		desiredOSD, ok := desiredOSDs[actualOSD.ID]
		if !ok {
			continue
		}
		if err := c.updateDeviceClassIfChanged(actualOSD.ID, desiredOSD.DeviceClass, actualOSD.DeviceClass); err != nil {
			// Log the error and allow other updates to continue
			logger.Errorf("failed to update device class on cluster in namespace %s: %v", c.clusterInfo.Namespace, err)
		}
	}

	return nil
}

func (c *Cluster) updateDeviceClassIfChanged(osdID int, desiredDeviceClass, actualDeviceClass string) error {
	if !c.spec.Storage.AllowDeviceClassUpdate {
		// device class updates are not allowed by default
		return nil
	}
	if desiredDeviceClass != "" && desiredDeviceClass != actualDeviceClass {
		logger.Infof("updating osd.%d device class from %q to %q", osdID, actualDeviceClass, desiredDeviceClass)
		err := cephclient.SetDeviceClass(c.context, c.clusterInfo, osdID, desiredDeviceClass)
		if err != nil {
			return errors.Wrapf(err, "failed to set device class on osd %d", osdID)
		}
		return nil
	}
	logger.Debugf("no device class change needed for osd.%d. desired=%q, actual=%q", osdID, desiredDeviceClass, actualDeviceClass)
	return nil
}

func (c *Cluster) getExistingOSDDeploymentsOnPVCs() (sets.Set[string], error) {
	listOpts := metav1.ListOptions{LabelSelector: fmt.Sprintf("%s=%s,%s", k8sutil.AppAttr, AppName, OSDOverPVCLabelKey)}

	deployments, err := c.context.Clientset.AppsV1().Deployments(c.clusterInfo.Namespace).List(c.clusterInfo.Context, listOpts)
	if err != nil {
		return nil, errors.Wrap(err, "failed to query existing OSD deployments")
	}

	result := sets.New[string]()
	for _, deployment := range deployments.Items {
		if pvcID, ok := deployment.Labels[OSDOverPVCLabelKey]; ok {
			result.Insert(pvcID)
		}
	}

	return result, nil
}

func deploymentOnNode(c *Cluster, osd *OSDInfo, nodeName string, config *provisionConfig) (*appsv1.Deployment, error) {
	osdLongName := fmt.Sprintf("OSD %d on node %q", osd.ID, nodeName)

	osdProps, err := c.getOSDPropsForNode(nodeName, osd.DeviceClass)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to generate config for %s", osdLongName)
	}

	d, err := c.makeDeployment(osdProps, osd, config)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to generate deployment for %s", osdLongName)
	}

	err = setOSDProperties(c, osdProps, osd)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to prepare deployment for %s", osdLongName)
	}

	return d, nil
}

func deploymentOnPVC(c *Cluster, osd *OSDInfo, pvcName string, config *provisionConfig) (*appsv1.Deployment, error) {
	osdLongName := fmt.Sprintf("OSD %d on PVC %q", osd.ID, pvcName)

	osdProps, err := c.getOSDPropsForPVC(pvcName)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to generate config for %s", osdLongName)
	}

	d, err := c.makeDeployment(osdProps, osd, config)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to generate deployment for %s", osdLongName)
	}

	err = setOSDProperties(c, osdProps, osd)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to prepare deployment for %s", osdLongName)
	}

	return d, nil
}

// setOSDProperties is used to configure an OSD with parameters which can not be set via explicit
// command-line arguments.
func setOSDProperties(c *Cluster, osdProps osdProperties, osd *OSDInfo) error {
	// OSD's 'primary-affinity' has to be configured via command which goes through mons
	if osdProps.storeConfig.PrimaryAffinity != "" {
		return cephclient.SetPrimaryAffinity(c.context, c.clusterInfo, osd.ID, osdProps.storeConfig.PrimaryAffinity)
	}
	return nil
}

func (c *Cluster) resolveNode(nodeName, deviceClass string) *cephv1.Node {
	// fully resolve the storage config and resources for this node
	rookNode := c.ValidStorage.ResolveNode(nodeName)
	if rookNode == nil {
		return nil
	}
	rookNode.Resources = k8sutil.MergeResourceRequirements(rookNode.Resources, cephv1.GetOSDResources(c.spec.Resources, deviceClass))

	return rookNode
}

func (c *Cluster) getOSDPropsForNode(nodeName, deviceClass string) (osdProperties, error) {
	// fully resolve the storage config and resources for this node
	n := c.resolveNode(nodeName, deviceClass)
	if n == nil {
		return osdProperties{}, errors.Errorf("failed to resolve node %q", nodeName)
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

	return osdProps, nil
}

func (c *Cluster) getOSDPropsForPVC(pvcName string) (osdProperties, error) {
	for _, deviceSet := range c.deviceSets {
		// The data PVC template is required.
		dataSource, dataOK := deviceSet.PVCSources[bluestorePVCData]
		if !dataOK {
			logger.Warningf("failed to find data source daemon for device set %q, missing the data template", deviceSet.Name)
			continue
		}

		if pvcName == dataSource.ClaimName {
			metadataSource, metadataOK := deviceSet.PVCSources[bluestorePVCMetadata]
			if metadataOK {
				logger.Infof("OSD will have its main bluestore block on %q and its metadata device on %q", dataSource.ClaimName, metadataSource.ClaimName)
			} else {
				logger.Infof("OSD will have its main bluestore block on %q", dataSource.ClaimName)
			}

			walSource, walOK := deviceSet.PVCSources[bluestorePVCWal]
			if walOK {
				logger.Infof("OSD will have its wal device on %q", walSource.ClaimName)
			}

			if deviceSet.Resources.Limits == nil && deviceSet.Resources.Requests == nil {
				deviceSet.Resources = cephv1.GetOSDResources(c.spec.Resources, deviceSet.CrushDeviceClass)
			}

			osdProps := osdProperties{
				crushHostname:       dataSource.ClaimName,
				pvc:                 dataSource,
				metadataPVC:         metadataSource,
				walPVC:              walSource,
				resources:           deviceSet.Resources,
				placement:           deviceSet.Placement,
				preparePlacement:    deviceSet.PreparePlacement,
				portable:            deviceSet.Portable,
				tuneSlowDeviceClass: deviceSet.TuneSlowDeviceClass,
				tuneFastDeviceClass: deviceSet.TuneFastDeviceClass,
				pvcSize:             deviceSet.Size,
				schedulerName:       deviceSet.SchedulerName,
				encrypted:           deviceSet.Encrypted,
				deviceSetName:       deviceSet.Name,
			}
			osdProps.storeConfig.InitialWeight = deviceSet.CrushInitialWeight
			osdProps.storeConfig.PrimaryAffinity = deviceSet.CrushPrimaryAffinity
			osdProps.storeConfig.DeviceClass = deviceSet.CrushDeviceClass

			// If OSD isn't portable, we're getting the host name either from the osd deployment that was already initialized
			// or from the osd prepare job from initial creation.
			if !deviceSet.Portable {
				var err error
				osdProps.crushHostname, err = c.getPVCHostName(pvcName)
				if err != nil {
					return osdProperties{}, errors.Wrapf(err, "failed to get crushHostname of non-portable PVC %q", pvcName)
				}
			}
			return osdProps, nil
		}
	}
	return osdProperties{}, errors.Errorf("failed to find valid VolumeSource for PVC %q", pvcName)
}

// getPVCHostName finds the node where an OSD pod should be assigned with a node selector.
// First look for the node selector that was previously used for the OSD, or if a new OSD
// check for the assignment of the OSD prepare job.
func (c *Cluster) getPVCHostName(pvcName string) (string, error) {
	listOpts := metav1.ListOptions{LabelSelector: fmt.Sprintf("%s=%s", OSDOverPVCLabelKey, pvcName)}

	// Check for the existence of the OSD deployment where the node selector was applied
	// in a previous reconcile.
	deployments, err := c.context.Clientset.AppsV1().Deployments(c.clusterInfo.Namespace).List(c.clusterInfo.Context, listOpts)
	if err != nil {
		return "", errors.Wrapf(err, "failed to get deployment for osd with pvc %q", pvcName)
	}
	for _, d := range deployments.Items {
		selectors := d.Spec.Template.Spec.NodeSelector
		for label, value := range selectors {
			if label == corev1.LabelHostname {
				return value, nil
			}
		}
	}

	// Since the deployment wasn't found it must be a new deployment so look at the node
	// assignment of the OSD prepare pod
	pods, err := c.context.Clientset.CoreV1().Pods(c.clusterInfo.Namespace).List(c.clusterInfo.Context, listOpts)
	if err != nil {
		return "", errors.Wrapf(err, "failed to get pod for osd with pvc %q", pvcName)
	}
	for _, pod := range pods.Items {
		name, err := k8sutil.GetNodeHostName(c.clusterInfo.Context, c.context.Clientset, pod.Spec.NodeName)
		if err != nil {
			logger.Warningf("falling back to node name %s since hostname not found for node", pod.Spec.NodeName)
			name = pod.Spec.NodeName
		}
		if name == "" {
			return "", errors.Errorf("node name not found on the osd pod %q", pod.Name)
		}
		//nolint SA4004 (go-staticcheck)
		return name, nil
	}

	return "", errors.Errorf("node selector not found on deployment for osd with pvc %q", pvcName)
}

func getOSDID(d *appsv1.Deployment) (int, error) {
	osdID, err := strconv.Atoi(d.Labels[OsdIdLabelKey])
	if err != nil {
		// add a question to the user AFTER the error text to help them recover from user error
		return -1, errors.Wrapf(err, "failed to parse label \"ceph-osd-id\" on deployment %q. did a user modify the deployment and remove the label?", d.Name)
	}
	return osdID, nil
}

func (c *Cluster) getOSDInfo(d *appsv1.Deployment) (OSDInfo, error) {
	container := d.Spec.Template.Spec.Containers[0]
	var osd OSDInfo

	osdID, err := getOSDID(d)
	if err != nil {
		return OSDInfo{}, err
	}
	osd.ID = osdID

	isPVC := false

	for _, envVar := range d.Spec.Template.Spec.Containers[0].Env {
		if envVar.Name == "ROOK_NODE_NAME" {
			osd.NodeName = envVar.Value
		}
		if envVar.Name == "ROOK_OSD_UUID" {
			osd.UUID = envVar.Value
		}
		if envVar.Name == "ROOK_PVC_BACKED_OSD" {
			isPVC = true
		}
		if envVar.Name == "ROOK_BLOCK_PATH" || envVar.Name == "ROOK_LV_PATH" {
			osd.BlockPath = envVar.Value
		}
		if envVar.Name == "ROOK_CV_MODE" {
			osd.CVMode = envVar.Value
		}
		if envVar.Name == "ROOK_TOPOLOGY_AFFINITY" {
			osd.TopologyAffinity = envVar.Value
		}
		if envVar.Name == "ROOK_LV_BACKED_PV" {
			lvBackedPV, err := strconv.ParseBool(envVar.Value)
			if err != nil {
				return OSDInfo{}, errors.Wrap(err, "failed to parse ROOK_LV_BACKED_PV")
			}
			osd.LVBackedPV = lvBackedPV
		}
		if envVar.Name == osdMetadataDeviceEnvVarName {
			osd.MetadataPath = envVar.Value
		}
		if envVar.Name == osdWalDeviceEnvVarName {
			osd.WalPath = envVar.Value
		}
		if envVar.Name == osdDeviceClassEnvVarName {
			osd.DeviceClass = envVar.Value
		}
	}

	// Needed for upgrade from v1.5 to v1.6. Rook v1.5 did not set ROOK_BLOCK_PATH for OSDs on nodes
	// where the 'activate' init container was needed.
	if !isPVC && osd.BlockPath == "" {
		osd.BlockPath, err = getBlockPathFromActivateInitContainer(d)
		if err != nil {
			return OSDInfo{}, errors.Wrapf(err, "failed to extract legacy OSD block path from deployment %q", d.Name)
		}
	}

	// If CVMode is empty, this likely means we upgraded Rook
	// This property did not exist before so we need to initialize it
	if osd.CVMode == "" {
		logger.Infof("required CVMode for OSD %d was not found. assuming this is an LVM OSD", osd.ID)
		osd.CVMode = "lvm"
	}

	// if the ROOK_TOPOLOGY_AFFINITY env var was not found in the loop above, detect it from the node
	if isPVC && osd.TopologyAffinity == "" {
		osd.TopologyAffinity, err = getTopologyFromNode(c.clusterInfo.Context, c.context.Clientset, d, osd)
		if err != nil {
			logger.Errorf("failed to get topology affinity for osd %d. %v", osd.ID, err)
		}
	}

	locationFound := false
	osd.Location, locationFound = getOSDLocationFromArgs(container.Args)

	if !locationFound {
		location, _, err := getLocationFromPod(c.clusterInfo.Context, c.context.Clientset, d, cephclient.GetCrushRootFromSpec(&c.spec))
		if err != nil {
			logger.Errorf("failed to get location. %v", err)
		} else {
			osd.Location = location
		}
	}

	if osd.UUID == "" || osd.BlockPath == "" {
		return OSDInfo{}, errors.Errorf("failed to get required osdInfo. %+v", osd)
	}

	osd.Store = d.Labels[osdStore]
	osd.Encrypted = false
	if d.Labels[encrypted] == "true" {
		osd.Encrypted = true
	}

	if isPVC {
		osd.PVCName = d.Labels[OSDOverPVCLabelKey]
	}

	return osd, nil
}

func osdIsOnPVC(d *appsv1.Deployment) bool {
	if _, ok := d.Labels[OSDOverPVCLabelKey]; ok {
		return true
	}
	return false
}

func getNodeOrPVCName(d *appsv1.Deployment) (string, error) {
	if v, ok := d.Labels[OSDOverPVCLabelKey]; ok {
		return v, nil // OSD is on PVC
	}
	for k, v := range d.Spec.Template.Spec.NodeSelector {
		if k == corev1.LabelHostname {
			return v, nil
		}
	}
	return "", errors.Errorf("failed to find node/PVC name for OSD deployment %q: %+v", d.Name, d)
}

// Needed for upgrades from v1.5 to v1.6
func getBlockPathFromActivateInitContainer(d *appsv1.Deployment) (string, error) {
	initContainers := d.Spec.Template.Spec.InitContainers
	for _, c := range initContainers {
		if c.Name != activatePVCOSDInitContainer {
			continue
		}
		if len(c.Command) != 3 {
			return "", errors.Errorf("activate init container has fewer command arguments (%d) than expected (3)", len(c.Command))
		}
		script := c.Command[2]
		varAssignment := "DEVICE=" // this variable assignment is followed by the block path
		scanner := bufio.NewScanner(strings.NewReader(script))
		for scanner.Scan() {
			line := strings.TrimSpace(scanner.Text())
			if strings.HasPrefix(line, varAssignment) {
				device := strings.TrimPrefix(line, varAssignment)
				return device, nil
			}
		}
		if scanner.Err() != nil {
			return "", errors.Wrapf(scanner.Err(), "failed to scan through activate init script for variable assignment %q", varAssignment)
		}
	}
	return "", errors.Errorf("failed to find activate init container")
}

func getLocationFromPod(ctx context.Context, clientset kubernetes.Interface, d *appsv1.Deployment, crushRoot string) (string, string, error) {
	pods, err := clientset.CoreV1().Pods(d.Namespace).List(ctx, metav1.ListOptions{LabelSelector: fmt.Sprintf("%s=%s", OsdIdLabelKey, d.Labels[OsdIdLabelKey])})
	if err != nil || len(pods.Items) == 0 {
		return "", "", err
	}
	nodeName := pods.Items[0].Spec.NodeName
	hostName, err := k8sutil.GetNodeHostName(ctx, clientset, nodeName)
	if err != nil {
		return "", "", err
	}
	portable, ok := d.GetLabels()[portableKey]
	if ok && portable == "true" {
		pvcName, ok := d.GetLabels()[OSDOverPVCLabelKey]
		if ok {
			hostName = pvcName
		}
	}
	return GetLocationWithNode(ctx, clientset, nodeName, crushRoot, hostName)
}

func getTopologyFromNode(ctx context.Context, clientset kubernetes.Interface, d *appsv1.Deployment, osd OSDInfo) (string, error) {
	portable, ok := d.GetLabels()[portableKey]
	if !ok || portable != "true" {
		// osd is not portable, no need to load the topology affinity
		return "", nil
	}
	logger.Infof("detecting topology affinity for osd %d after upgrade", osd.ID)

	// Get the osd pod and its assigned node, then look up the node labels
	pods, err := clientset.CoreV1().Pods(d.Namespace).List(ctx, metav1.ListOptions{LabelSelector: fmt.Sprintf("%s=%s", OsdIdLabelKey, d.Labels[OsdIdLabelKey])})
	if err != nil {
		return "", errors.Wrap(err, "failed to get osd pod")
	}
	if len(pods.Items) == 0 {
		return "", errors.New("an osd pod does not exist")
	}
	nodeName := pods.Items[0].Spec.NodeName
	if nodeName == "" {
		return "", errors.Errorf("osd %d is not assigned to a node, cannot detect topology affinity", osd.ID)
	}
	node, err := getNode(ctx, clientset, nodeName)
	if err != nil {
		return "", errors.Wrap(err, "failed to get the node for topology affinity")
	}
	_, topologyAffinity := topology.ExtractOSDTopologyFromLabels(node.Labels)
	logger.Infof("found osd %d topology affinity at %q", osd.ID, topologyAffinity)
	return topologyAffinity, nil
}

// GetLocationWithNode gets the topology information about the node. The return values are:
//
//	 location: The CRUSH properties for the OSD to apply
//	 topologyAffinity: The label to be applied to the OSD daemon to guarantee it will start in the same
//			topology as the OSD prepare job.
func GetLocationWithNode(ctx context.Context, clientset kubernetes.Interface, nodeName string, crushRoot, crushHostname string) (string, string, error) {
	node, err := getNode(ctx, clientset, nodeName)
	if err != nil {
		return "", "", errors.Wrap(err, "could not get the node for topology labels")
	}

	// If the operator did not pass a host name, look up the hostname label.
	// This happens when the operator doesn't know on what node the osd will be assigned (non-portable PVCs).
	if crushHostname == "" {
		crushHostname, err = k8sutil.GetNodeHostNameLabel(node)
		if err != nil {
			return "", "", errors.Wrapf(err, "failed to get the host name label for node %q", node.Name)
		}
	}

	// Start with the host name in the CRUSH map
	// Keep the fully qualified host name in the crush map, but replace the dots with dashes to satisfy ceph
	hostName := cephclient.NormalizeCrushName(crushHostname)
	locArgs := []string{fmt.Sprintf("root=%s", crushRoot), fmt.Sprintf("host=%s", hostName)}

	nodeLabels := node.GetLabels()
	topologyAffinity := updateLocationWithNodeLabels(&locArgs, nodeLabels)

	loc := strings.Join(locArgs, " ")
	logger.Infof("CRUSH location=%s", loc)
	return loc, topologyAffinity, nil
}

// getNode will try to get the node object for the provided nodeName
// it will try using the node's name it's hostname label
func getNode(ctx context.Context, clientset kubernetes.Interface, nodeName string) (*corev1.Node, error) {
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

func updateLocationWithNodeLabels(location *[]string, nodeLabels map[string]string) string {
	topology, topologyAffinity := topology.ExtractOSDTopologyFromLabels(nodeLabels)

	keys := make([]string, 0, len(topology))
	for k := range topology {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	for _, topologyType := range keys {
		if topologyType != "host" {
			cephclient.UpdateCrushMapValue(location, topologyType, topology[topologyType])
		}
	}
	return topologyAffinity
}

func (c *Cluster) applyUpgradeOSDFunctionality() {
	var osdVersion *cephver.CephVersion

	// Get all the daemons versions
	versions, err := cephclient.GetAllCephDaemonVersions(c.context, c.clusterInfo)
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
			// Ensure the required version of OSDs is set to the current consistent version,
			// enabling the latest osd functionality and also preventing downgrades to a
			// previous major ceph version.
			err = cephclient.EnableReleaseOSDFunctionality(c.context, c.clusterInfo, osdVersion.ReleaseName())
			if err != nil {
				logger.Warningf("failed to enable new osd functionality. %v", err)
				return
			}
		}
	}
}

// deleteOSDDeployment deletes an existing OSD deployment and saves the information in the configmap
func (c *Cluster) deleteOSDDeployment(osdID int) error {
	// Delete the OSD deployment
	deploymentName := fmt.Sprintf("rook-ceph-osd-%d", osdID)
	logger.Infof("removing the OSD deployment %q", deploymentName)
	if err := k8sutil.DeleteDeployment(c.clusterInfo.Context, c.context.Clientset, c.clusterInfo.Namespace, deploymentName); err != nil {
		if err != nil {
			if kerrors.IsNotFound(err) {
				logger.Debugf("osd deployment %q not found. Ignoring since object must be deleted.", deploymentName)
			} else {
				return errors.Wrapf(err, "failed to delete OSD deployment %q.", deploymentName)
			}
		}
	}

	return nil
}

func (c *Cluster) waitForHealthyPGs() (bool, error) {
	waitFunc := func() (done bool, err error) {
		pgHealthMsg, pgClean, err := cephclient.IsClusterClean(c.context, c.clusterInfo, c.spec.DisruptionManagement.PGHealthyRegex)
		if err != nil {
			return false, errors.Wrap(err, "failed to check pg are healthy")
		}
		if pgClean {
			return true, nil
		}
		logger.Infof("waiting for PGs to be healthy. PG status: %q", pgHealthMsg)
		return false, nil
	}

	err := util.RetryWithTimeout(waitFunc, waitForHealthyPGInterval, waitForHealthyPGTimeout, "pgs to be healthy")
	if err != nil {
		return false, err
	}

	return true, nil
}

func (c *Cluster) updateCephStorageStatus() error {
	cephCluster := cephv1.CephCluster{}
	cephClusterStorage := cephv1.CephStorage{}

	deviceClasses, err := cephclient.GetDeviceClasses(c.context, c.clusterInfo)
	if err != nil {
		return errors.Wrap(err, "failed to get osd device classes")
	}

	for _, deviceClass := range deviceClasses {
		cephClusterStorage.DeviceClasses = append(cephClusterStorage.DeviceClasses, cephv1.DeviceClasses{Name: deviceClass})
	}

	osdStore, err := c.getOSDStoreStatus()
	if err != nil {
		return errors.Wrapf(err, "failed to get osd store status")
	}

	cephClusterStorage.OSD = *osdStore

	// Add the status about deprecated OSDs
	cephClusterStorage.DeprecatedOSDs = c.deprecatedOSDs

	// Update pending migration status
	if c.isMigrationRequested() {
		migrationConfig, err := c.newMigrationConfig()
		if err != nil {
			return errors.Wrapf(err, "failed to get osd migration config to update cluster status")
		}
		cephClusterStorage.OSD.MigrationStatus.Pending = len(migrationConfig.osds)
	}

	err = c.context.Client.Get(c.clusterInfo.Context, c.clusterInfo.NamespacedName(), &cephCluster)
	if err != nil {
		if kerrors.IsNotFound(err) {
			logger.Debug("CephCluster resource not found. Ignoring since object must be deleted.")
			return nil
		}
		return errors.Wrapf(err, "failed to retrieve ceph cluster %q to update ceph Storage", c.clusterInfo.NamespacedName().Name)
	}
	if !reflect.DeepEqual(cephCluster.Status.CephStorage, cephClusterStorage) {
		cephCluster.Status.CephStorage = &cephClusterStorage
		if err := reporting.UpdateStatus(c.context.Client, &cephCluster); err != nil {
			return errors.Wrapf(err, "failed to update cluster %q Storage.", c.clusterInfo.NamespacedName().Name)
		}
	}

	return nil
}

func (c *Cluster) getOSDStoreStatus() (*cephv1.OSDStatus, error) {
	label := fmt.Sprintf("%s=%s", k8sutil.AppAttr, AppName)
	osdDeployments, err := k8sutil.GetDeployments(c.clusterInfo.Context, c.context.Clientset, c.clusterInfo.Namespace, label)
	if err != nil {
		if kerrors.IsNotFound(err) {
			return nil, nil
		}
		return nil, errors.Wrap(err, "failed to get osd deployments")
	}

	storeType := map[string]int{}
	for i := range osdDeployments.Items {
		if osdStore, ok := osdDeployments.Items[i].Labels[osdStore]; ok {
			storeType[osdStore]++
		}
	}

	return &cephv1.OSDStatus{
		StoreType: storeType,
	}, nil
}

func getOSDLocationFromArgs(args []string) (string, bool) {
	for _, a := range args {
		locationPrefix := "--crush-location="
		if strings.HasPrefix(a, locationPrefix) {
			// Extract the same CRUSH location as originally determined by the OSD prepare pod
			// by cutting off the prefix: --crush-location=
			return a[len(locationPrefix):], true
		}
	}

	return "", false
}
