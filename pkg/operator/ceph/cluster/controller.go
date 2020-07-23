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

// Package cluster to manage a Ceph cluster.
package cluster

import (
	"fmt"
	"os"
	"path"
	"reflect"
	"sync"
	"time"

	"github.com/coreos/pkg/capnslog"
	cephv1 "github.com/rook/rook/pkg/apis/ceph.rook.io/v1"
	"github.com/rook/rook/pkg/clusterd"
	"github.com/rook/rook/pkg/daemon/ceph/agent/flexvolume/attachment"
	"github.com/rook/rook/pkg/daemon/ceph/client"
	cephconfig "github.com/rook/rook/pkg/daemon/ceph/config"
	discoverDaemon "github.com/rook/rook/pkg/daemon/discover"
	cephclient "github.com/rook/rook/pkg/operator/ceph/client"
	"github.com/rook/rook/pkg/operator/ceph/cluster/crash"
	"github.com/rook/rook/pkg/operator/ceph/cluster/mgr"
	"github.com/rook/rook/pkg/operator/ceph/cluster/mon"
	"github.com/rook/rook/pkg/operator/ceph/cluster/osd"
	"github.com/rook/rook/pkg/operator/ceph/config"
	"github.com/rook/rook/pkg/operator/ceph/controller"
	"github.com/rook/rook/pkg/operator/ceph/csi"
	"github.com/rook/rook/pkg/operator/ceph/object/bucket"
	cephver "github.com/rook/rook/pkg/operator/ceph/version"
	"github.com/rook/rook/pkg/operator/k8sutil"

	"github.com/pkg/errors"
	v1 "k8s.io/api/core/v1"
	kerrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/informers"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/cache"
)

const (
	crushConfigMapName       = "rook-crush-config"
	crushmapCreatedKey       = "initialCrushMapCreated"
	enableFlexDriver         = "ROOK_ENABLE_FLEX_DRIVER"
	clusterCreateInterval    = 6 * time.Second
	clusterCreateTimeout     = 60 * time.Minute
	updateClusterInterval    = 30 * time.Second
	updateClusterTimeout     = 1 * time.Hour
	detectCephVersionTimeout = 15 * time.Minute
)

const (
	// DefaultClusterName states the default name of the rook-cluster if not provided.
	DefaultClusterName         = "rook-ceph"
	clusterDeleteRetryInterval = 2 // seconds
	clusterDeleteMaxRetries    = 15
	disableHotplugEnv          = "ROOK_DISABLE_DEVICE_HOTPLUG"
	minStoreResyncPeriod       = 10 * time.Hour // the minimum duration for forced Store resyncs.
)

var (
	logger        = capnslog.NewPackageLogger("github.com/rook/rook", "op-cluster")
	finalizerName = fmt.Sprintf("%s.%s", ClusterResource.Name, ClusterResource.Group)
	// disallowedHostDirectories directories which are not allowed to be used
	disallowedHostDirectories = []string{"/etc/ceph", "/rook", "/var/log/ceph"}
)

// ClusterResource operator-kit Custom Resource Definition
var ClusterResource = k8sutil.CustomResource{
	Name:    "cephcluster",
	Plural:  "cephclusters",
	Group:   cephv1.CustomResourceGroup,
	Version: cephv1.Version,
	Kind:    reflect.TypeOf(cephv1.CephCluster{}).Name(),
}

// ClusterController controls an instance of a Rook cluster
type ClusterController struct {
	context                 *clusterd.Context
	volumeAttachment        attachment.Attachment
	rookImage               string
	clusterMap              map[string]*cluster
	operatorConfigCallbacks []func() error
	addClusterCallbacks     []func() error
	csiConfigMutex          *sync.Mutex
	nodeStore               cache.Store
	osdChecker              *osd.OSDHealthMonitor
}

// NewClusterController create controller for watching cluster custom resources created
func NewClusterController(context *clusterd.Context, rookImage string, volumeAttachment attachment.Attachment, operatorConfigCallbacks []func() error, addClusterCallbacks []func() error) *ClusterController {
	return &ClusterController{
		context:                 context,
		volumeAttachment:        volumeAttachment,
		rookImage:               rookImage,
		clusterMap:              make(map[string]*cluster),
		operatorConfigCallbacks: operatorConfigCallbacks,
		addClusterCallbacks:     addClusterCallbacks,
		csiConfigMutex:          &sync.Mutex{},
	}
}

// StartWatch watches instances of cluster resources
func (c *ClusterController) StartWatch(namespace string, stopCh chan struct{}) {
	resourceHandlerFuncs := cache.ResourceEventHandlerFuncs{
		AddFunc:    c.onAdd,
		UpdateFunc: c.onUpdate,
		DeleteFunc: c.onDelete,
	}

	if len(namespace) == 0 {
		logger.Infof("start watching clusters in all namespaces")
	} else {
		logger.Infof("start watching clusters in namespace: %v", namespace)
	}
	go k8sutil.WatchCR(ClusterResource, namespace, resourceHandlerFuncs, c.context.RookClientset.CephV1().RESTClient(), &cephv1.CephCluster{}, stopCh)

	// Watch for events on new/updated K8s Nodes objects

	sharedInformerFactory := informers.NewSharedInformerFactory(c.context.Clientset, minStoreResyncPeriod)
	nodeController := sharedInformerFactory.Core().V1().Nodes().Informer()
	nodeController.AddEventHandler(
		cache.ResourceEventHandlerFuncs{
			AddFunc:    c.onK8sNodeAdd,
			UpdateFunc: c.onK8sNodeUpdate,
			DeleteFunc: nil,
		},
	)
	c.nodeStore = nodeController.GetStore()

	go nodeController.Run(stopCh)

	operatorNamespace := os.Getenv(k8sutil.PodNamespaceEnvVar)
	if disableVal := os.Getenv(disableHotplugEnv); disableVal != "true" {
		// watch for updates to the device discovery configmap
		logger.Infof("Enabling hotplug orchestration: %s=%s", disableHotplugEnv, disableVal)
		_, deviceCMController := cache.NewInformer(
			cache.NewFilteredListWatchFromClient(c.context.Clientset.CoreV1().RESTClient(),
				"configmaps", operatorNamespace, func(options *metav1.ListOptions) {
					options.LabelSelector = fmt.Sprintf("%s=%s", k8sutil.AppAttr, discoverDaemon.AppName)
				},
			),
			&v1.ConfigMap{},
			0,
			cache.ResourceEventHandlerFuncs{
				AddFunc:    nil,
				UpdateFunc: c.onDeviceCMUpdate,
				DeleteFunc: nil,
			},
		)

		go deviceCMController.Run(stopCh)
	} else {
		logger.Infof("Disabling hotplug orchestration via %s", disableHotplugEnv)
	}

	// watch for "rook-ceph-operator-config" ConfigMap
	k8sutil.StartOperatorSettingsWatch(c.context, operatorNamespace, controller.OperatorSettingConfigMapName,
		c.operatorConfigChange,
		func(oldObj, newObj interface{}) {
			if reflect.DeepEqual(oldObj, newObj) {
				return
			}
			c.operatorConfigChange(newObj)
			return
		}, nil, stopCh)
}

func (c *ClusterController) StopWatch() {
	for _, cluster := range c.clusterMap {
		close(cluster.stopCh)
	}
	c.clusterMap = make(map[string]*cluster)
}

func (c *ClusterController) GetClusterCount() int {
	return len(c.clusterMap)
}

// ************************************************************************************************
// Add event functions
// ************************************************************************************************
func (c *ClusterController) operatorConfigChange(obj interface{}) {
	cm, ok := obj.(*v1.ConfigMap)
	if !ok {
		logger.Warningf("Expected ConfigMap but handler received %T. %#v", obj, obj)
		return
	}

	logger.Infof("ConfigMap %q changes detected. Updating configurations", cm.Name)
	for _, callback := range c.operatorConfigCallbacks {
		if err := callback(); err != nil {
			logger.Errorf("%v", err)
		}
	}
	return
}

func (c *ClusterController) onK8sNodeAdd(obj interface{}) {
	newNode, ok := obj.(*v1.Node)
	if !ok {
		logger.Warningf("Expected NodeList but handler received %#v", obj)
		return
	}

	if k8sutil.GetNodeSchedulable(*newNode) == false {
		logger.Debugf("Skipping cluster update. Added node %s is unschedulable", newNode.Labels[v1.LabelHostname])
		return
	}

	for _, cluster := range c.clusterMap {
		if k8sutil.NodeIsTolerable(*newNode, cephv1.GetOSDPlacement(cluster.Spec.Placement).Tolerations, false) == false {
			logger.Debugf("Skipping -> Node is not tolerable for cluster %s", cluster.Namespace)
			continue
		}
		if cluster.Spec.Storage.UseAllNodes == false {
			logger.Debugf("Skipping -> Do not use all Nodes in cluster %s", cluster.Namespace)
			continue
		}
		if cluster.Info == nil {
			logger.Infof("Cluster %s is not ready. Skipping orchestration.", cluster.Namespace)
			continue
		}

		if valid, _ := k8sutil.ValidNode(*newNode, cluster.Spec.Placement.All()); valid == true {
			logger.Debugf("Adding %s to cluster %s", newNode.Labels[v1.LabelHostname], cluster.Namespace)
			err := cluster.createInstance(c.rookImage, cluster.Info.CephVersion)
			if err != nil {
				logger.Errorf("failed to update cluster in namespace %q. was not able to add %q. %v", cluster.Namespace, newNode.Labels[v1.LabelHostname], err)
			}
		} else {
			logger.Infof("Could not add host %s . It is not valid", newNode.Labels[v1.LabelHostname])
			continue
		}
		logger.Infof("Added %s to cluster %s", newNode.Labels[v1.LabelHostname], cluster.Namespace)
	}
}

func (c *ClusterController) onAdd(obj interface{}) {
	clusterObj, err := getClusterObject(obj)
	if err != nil {
		logger.Errorf("failed to get cluster object. %v", err)
		return
	}

	if clusterObj.Spec.CleanupPolicy.HasDataDirCleanPolicy() {
		logger.Infof("skipping orchestration for cluster object %q in namespace %q because its cleanup policy is set", clusterObj.Name, clusterObj.Namespace)
		return
	}

	if existing, ok := c.clusterMap[clusterObj.Namespace]; ok {
		logger.Errorf("failed to add cluster cr %q in namespace %q. Cluster cr %q already exists in this namespace. Only one cluster cr per namespace is supported.",
			clusterObj.Name, clusterObj.Namespace, existing.crdName)
		return
	}

	cluster := newCluster(clusterObj, c.context, c.csiConfigMutex)

	// Note that this lock is held through the callback process, as this creates CSI resources, but we must lock in
	// this scope as the clusterMap is authoritative on cluster count and thus involved in the check for CSI resource
	// deletion. If we ever add additional callback functions, we should tighten this lock.
	c.csiConfigMutex.Lock()
	c.clusterMap[cluster.Namespace] = cluster
	logger.Infof("starting cluster in namespace %s", cluster.Namespace)

	for _, callback := range c.addClusterCallbacks {
		if err := callback(); err != nil {
			logger.Errorf("%v", err)
		}
	}
	c.csiConfigMutex.Unlock()

	c.initializeCluster(cluster, clusterObj)
}

func (c *ClusterController) configureExternalCephCluster(namespace, name string, cluster *cluster) error {
	// Make sure the spec contains all the information we need
	err := validateExternalClusterSpec(cluster)
	if err != nil {
		return errors.Wrap(err, "failed to validate external cluster specs")
	}

	config.ConditionExport(c.context, namespace, name,
		cephv1.ConditionConnecting, v1.ConditionTrue, "ClusterConnecting", "Cluster is connecting")

	// loop until we find the secret necessary to connect to the external cluster
	// then populate clusterInfo
	cluster.Info = mon.PopulateExternalClusterInfo(c.context, namespace)

	// If the user to check the ceph health and status is not the admin,
	// we validate that ExternalCred has been populated correctly,
	// then we check if the key (whether admin or not) is encoded in base64
	if !mon.IsExternalHealthCheckUserAdmin(cluster.Info.AdminSecret) {
		if !cluster.Info.IsInitializedExternalCred(true) {
			return errors.New("invalid user health checker credentials")
		}
		if !cephconfig.IsKeyringBase64Encoded(cluster.Info.ExternalCred.Secret) {
			return errors.Errorf("invalid user health checker key %q", cluster.Info.ExternalCred.Username)
		}
	} else {
		// If the client.admin is used
		if !cephconfig.IsKeyringBase64Encoded(cluster.Info.AdminSecret) {
			return errors.Errorf("invalid user health checker key %q", client.AdminUsername)
		}
	}

	// Write connection info (ceph config file and keyring) for ceph commands
	if cluster.Spec.CephVersion.Image == "" {
		err = mon.WriteConnectionConfig(c.context, cluster.Info)
		if err != nil {
			logger.Errorf("failed to write config. attempting to continue. %v", err)
		}
	}

	// Validate versions (local and external)
	// If no image is specified we don't perform any checks
	if cluster.Spec.CephVersion.Image != "" {
		_, _, err = c.detectAndValidateCephVersion(cluster, cluster.Spec.CephVersion.Image)
		if err != nil {
			return errors.Wrap(err, "failed to detect and validate ceph version")
		}

		// Write the rook-config-override configmap (used by various daemons to apply config overrides)
		// If we don't do this, daemons will never start, waiting forever for this configmap to be present
		//
		// Only do this when doing a bit of management...
		logger.Infof("creating %q configmap", k8sutil.ConfigOverrideName)
		err = populateConfigOverrideConfigMap(cluster.context, namespace, cluster.ownerRef)
		if err != nil {
			return errors.Wrapf(err, "failed to populate config override config map")
		}

		logger.Infof("creating %q secret", config.StoreName)
		err = config.GetStore(cluster.context, namespace, &cluster.ownerRef).CreateOrUpdate(cluster.Info)
		if err != nil {
			return errors.Wrap(err, "failed to update the global config")
		}
	}

	// The cluster Identity must be established at this point
	if !cluster.Info.IsInitialized() {
		return errors.New("the cluster identity was not established")
	}
	logger.Info("external cluster identity established")

	// Create CSI Secrets only if the user has provided the admin key
	if cluster.Info.AdminSecret != mon.AdminSecretName {
		err = csi.CreateCSISecrets(c.context, namespace, &cluster.ownerRef)
		if err != nil {
			return errors.Wrap(err, "failed to create csi kubernetes secrets")
		}
	}

	// Create CSI config map
	err = csi.CreateCsiConfigMap(namespace, c.context.Clientset, &cluster.ownerRef)
	if err != nil {
		return errors.Wrap(err, "failed to create csi config map")
	}

	// Save CSI configmap
	err = csi.SaveClusterConfig(c.context.Clientset, namespace, cluster.Info, c.csiConfigMutex)
	if err != nil {
		return errors.Wrap(err, "failed to update csi cluster config")
	}
	logger.Info("successfully updated csi config map")

	// Create Crash Collector Secret
	// In 14.2.5 the crash daemon will read the client.crash key instead of the admin key
	if !cluster.Spec.CrashCollector.Disable {
		err = crash.CreateCrashCollectorSecret(c.context, namespace, &cluster.ownerRef)
		if err != nil {
			return errors.Wrap(err, "failed to create crash collector kubernetes secret")
		}
	}

	// Mark initialization has done
	cluster.initCompleted = true

	return nil
}

// Validate the cluster Specs
func (c *ClusterController) preClusterStartValidation(cluster *cluster, clusterObj *cephv1.CephCluster) error {

	if cluster.Spec.Mon.Count == 0 {
		logger.Warningf("mon count should be at least 1, will use default value of %d", mon.DefaultMonCount)
		cluster.Spec.Mon.Count = mon.DefaultMonCount
	}
	if cluster.Spec.Mon.Count%2 == 0 {
		logger.Warningf("mon count is even (given: %d), should be uneven, continuing", cluster.Spec.Mon.Count)
	}
	if len(cluster.Spec.Storage.Directories) != 0 {
		logger.Warning("running osds on directory is not supported anymore, use devices instead.")
	}
	if cluster.Spec.Network.IsMultus() {
		_, isPublic := cluster.Spec.Network.Selectors[config.PublicNetworkSelectorKeyName]
		_, isCluster := cluster.Spec.Network.Selectors[config.ClusterNetworkSelectorKeyName]
		if !isPublic && !isCluster {
			return errors.New("both network selector values for public and cluster selector cannot be empty for multus provider")
		}

		for _, selector := range config.NetworkSelectors {
			// If one selector is empty, we continue
			// This means a single interface is used both public and cluster network
			if _, ok := cluster.Spec.Network.Selectors[selector]; !ok {
				continue
			}

			// Get network attachment definition
			_, err := c.context.NetworkClient.NetworkAttachmentDefinitions(cluster.Namespace).Get(cluster.Spec.Network.Selectors[selector], metav1.GetOptions{})
			if err != nil {
				if kerrors.IsNotFound(err) {
					return errors.Wrapf(err, "specified network attachment definition for selector %q does not exist", selector)
				}
				return errors.Wrapf(err, "failed to fetch network attachment definition for selector %q", selector)
			}
		}
	}

	logger.Debug("cluster spec successfully validated")
	return nil
}

func (c *ClusterController) configureLocalCephCluster(namespace, name string, cluster *cluster, clusterObj *cephv1.CephCluster) error {
	// Cluster Spec validation
	err := c.preClusterStartValidation(cluster, clusterObj)
	if err != nil {
		return errors.Wrap(err, "failed to perform validation before cluster creation")
	}

	// Start the Rook cluster components. Retry several times in case of failure.
	failedMessage := ""

	err = wait.Poll(clusterCreateInterval, clusterCreateTimeout,
		func() (bool, error) {
			cephVersion, canRetry, err := c.detectAndValidateCephVersion(cluster, cluster.Spec.CephVersion.Image)
			if err != nil {
				failedMessage = fmt.Sprintf("failed the ceph version check. %v", err)
				logger.Errorf(failedMessage)
				if !canRetry {
					// it may seem strange to exit true but we don't want to retry if the version is not supported
					return true, err
				}
				return false, nil
			}
			message := config.CheckConditionReady(c.context, namespace, name)
			config.ConditionExport(c.context, namespace, name,
				cephv1.ConditionProgressing, v1.ConditionTrue, "ClusterProgressing", message)

			err = cluster.createInstance(c.rookImage, *cephVersion)
			if err != nil {
				failedMessage = fmt.Sprintf("failed to create cluster in namespace %q. %v", cluster.Namespace, err)
				logger.Errorf(failedMessage)
				return false, nil
			}
			config.ConditionExport(c.context, namespace, name,
				cephv1.ConditionReady, v1.ConditionTrue, "ClusterCreated", "Cluster created successfully")
			failedMessage = ""
			return true, nil
		})

	if err != nil {
		config.ConditionExport(c.context, namespace, name,
			cephv1.ConditionFailure, v1.ConditionTrue, "ClusterFailure", "Giving up waiting for cluster creating")
		return errors.Wrapf(err, "giving up waiting for cluster creating")
	}

	msg := config.ErrorMapping()
	if msg != nil {
		return errors.Wrapf(msg, "failed to create the cluster")
	}

	return nil
}

func (c *ClusterController) initializeCluster(cluster *cluster, clusterObj *cephv1.CephCluster) {
	cluster.Spec = &clusterObj.Spec

	// Check if the dataDirHostPath is located in the disallowed paths list
	cleanDataDirHostPath := path.Clean(cluster.Spec.DataDirHostPath)
	for _, b := range disallowedHostDirectories {
		if cleanDataDirHostPath == b {
			logger.Errorf("dataDirHostPath (given: %q) must not be used, conflicts with %q internal path", cluster.Spec.DataDirHostPath, b)
			return
		}
	}

	monitoringActivated := false
	config.ConditionInitialize(c.context, clusterObj.Namespace, clusterObj.Name)
	if !cluster.Spec.External.Enable {
		// If the local cluster has already been configured, immediately start monitoring the cluster.
		// Test if the cluster has already been configured if the mgr deployment has been created.
		// If the mgr does not exist, the mons have never been verified to be in quorum.
		opts := metav1.ListOptions{LabelSelector: fmt.Sprintf("%s=%s", k8sutil.AppAttr, mgr.AppName)}
		mgrDeployments, err := c.context.Clientset.AppsV1().Deployments(cluster.Namespace).List(opts)
		if err == nil && len(mgrDeployments.Items) > 0 {
			c.startClusterMonitoring(cluster)
			monitoringActivated = true
		}

		if err := c.configureLocalCephCluster(clusterObj.Namespace, clusterObj.Name, cluster, clusterObj); err != nil {
			logger.Errorf("failed to configure local ceph cluster. %v", err)
			return
		}
	} else {
		if err := c.configureExternalCephCluster(clusterObj.Namespace, clusterObj.Name, cluster); err != nil {
			config.ConditionExport(c.context, clusterObj.Namespace, clusterObj.Name,
				cephv1.ConditionFailure, v1.ConditionTrue, "ClusterFailure", "Failed to configure external ceph cluster")
			logger.Errorf("failed to configure external ceph cluster. %v", err)
			return
		}
	}

	// add the finalizer to the crd
	err := c.addFinalizer(clusterObj.Namespace, clusterObj.Name)
	if err != nil {
		logger.Errorf("failed to add finalizer to cluster crd. %v", err)
	}

	if !monitoringActivated {
		c.startClusterMonitoring(cluster)
	}
}

func (c *ClusterController) startClusterMonitoring(cluster *cluster) {
	if cluster.Info == nil {
		clusterInfo, _, _, err := mon.CreateOrLoadClusterInfo(c.context, cluster.Namespace, nil)
		if err != nil {
			logger.Errorf("failed to start osd monitoring. %v", err)
			return
		}
		cluster.Info = clusterInfo
		logger.Infof("cluster info loaded for monitoring: %+v", clusterInfo)
	}

	// enable the cluster monitoring goroutines once
	logger.Infof("enabling cluster monitoring goroutines")

	// Start client CRD watcher
	clientController := cephclient.NewClientController(c.context, cluster.Namespace)
	clientController.StartWatch(cluster.stopCh)

	clientToUse := client.AdminUsername
	if cluster.Spec.External.Enable {
		clientToUse = cluster.Info.ExternalCred.Username
	}
	// Start the object bucket provisioner
	bucketProvisioner := bucket.NewProvisioner(c.context, cluster.Namespace, clientToUse)
	// If cluster is external, pass down the user to the bucket controller

	// note: the error return below is ignored and is expected to be removed from the
	//   bucket library's `NewProvisioner` function
	bucketController, _ := bucket.NewBucketController(c.context.KubeConfig, bucketProvisioner)
	go bucketController.Run(cluster.stopCh)

	// Populate ClusterInfo
	cluster.mons.ClusterInfo = cluster.Info

	// Start mon health checker
	healthChecker := mon.NewHealthChecker(cluster.mons, cluster.Spec)
	go healthChecker.Check(cluster.stopCh)

	if !cluster.Spec.External.Enable {
		// Start the osd health checker only if running OSDs in the local ceph cluster
		c.osdChecker = osd.NewOSDHealthMonitor(c.context, cluster.Namespace, cluster.Spec.RemoveOSDsIfOutAndSafeToRemove, cluster.Info.CephVersion)
		go c.osdChecker.Start(cluster.stopCh)
	}

	// Start the ceph status checker
	cephChecker := newCephStatusChecker(c.context, cluster.Namespace, cluster.crdName, cluster.Info.ExternalCred, cluster.Spec.External.Enable)
	go cephChecker.checkCephStatus(cluster.stopCh)
}

// ************************************************************************************************
// Update event functions
// ************************************************************************************************
func (c *ClusterController) onK8sNodeUpdate(oldObj, newObj interface{}) {
	// skip forced resyncs
	if reflect.DeepEqual(oldObj, newObj) {
		return
	}

	// Checking for nodes where NoSchedule-Taint got removed
	newNode, ok := newObj.(*v1.Node)
	if !ok {
		logger.Warningf("Expected Node but handler received %#v", newObj)
		return
	}

	oldNode, ok := oldObj.(*v1.Node)
	if !ok {
		logger.Warningf("Expected Node but handler received %#v", oldObj)
		return
	}

	newNodeSchedulable := k8sutil.GetNodeSchedulable(*newNode)
	oldNodeSchedulable := k8sutil.GetNodeSchedulable(*oldNode)

	// Checking for NoSchedule added to storage node
	if oldNodeSchedulable == false && newNodeSchedulable == false {
		// Skipping cluster update. Updated node was and is still unschedulable
		return
	}
	if oldNodeSchedulable == true && newNodeSchedulable == true {
		// Skipping cluster update. Updated node was and is still schedulable
		return
	}

	for _, cluster := range c.clusterMap {
		if cluster.Info == nil {
			logger.Infof("Cluster %s is not ready. Skipping orchestration.", cluster.Namespace)
			continue
		}
		if valid, _ := k8sutil.ValidNode(*newNode, cephv1.GetOSDPlacement(cluster.Spec.Placement)); valid == true {
			logger.Debugf("Adding %s to cluster %s", newNode.Labels[v1.LabelHostname], cluster.Namespace)
			err := cluster.createInstance(c.rookImage, cluster.Info.CephVersion)
			if err != nil {
				logger.Errorf("Failed adding the updated node %q to cluster in namespace %q. %v", newNode.Labels[v1.LabelHostname], cluster.Namespace, err)
				continue
			}
		} else {
			logger.Infof("Updated node %q is not valid and could not get added to cluster in namespace %q.", newNode.Labels[v1.LabelHostname], cluster.Namespace)
			continue
		}
		logger.Infof("Added updated node %q to cluster %q", newNode.Labels[v1.LabelHostname], cluster.Namespace)
	}
}

func (c *ClusterController) onUpdate(oldObj, newObj interface{}) {
	oldClust, err := getClusterObject(oldObj)
	if err != nil {
		logger.Errorf("failed to get old cluster object. %v", err)
		return
	}
	newClust, err := getClusterObject(newObj)
	if err != nil {
		logger.Errorf("failed to get new cluster object. %v", err)
		return
	}

	logger.Debugf("update event for cluster %s", newClust.Namespace)

	if existing, ok := c.clusterMap[newClust.Namespace]; ok && existing.crdName != newClust.Name {
		logger.Errorf("skipping update of cluster cr %q in namespace %q. Cluster cr %q already exists in this namespace. Only one cluster cr per namespace is supported.",
			newClust.Name, newClust.Namespace, existing.crdName)
		return
	}

	// Check if the cluster is being deleted. This code path is called when a finalizer is specified in the crd.
	// When a cluster is requested for deletion, K8s will only set the deletion timestamp if there are any finalizers in the list.
	// K8s will only delete the crd and child resources when the finalizers have been removed from the crd.
	if newClust.DeletionTimestamp != nil {
		logger.Infof("cluster %q has a deletion timestamp", newClust.Namespace)

		// Start cluster clean up only if cleanupPolicy is applied to the ceph cluster
		if newClust.Spec.CleanupPolicy.HasDataDirCleanPolicy() {
			monSecret, clusterFSID, err := c.getCleanUpDetails(newClust.Namespace)
			if err != nil {
				logger.Errorf("failed to clean up cluster. %v", err)
				return
			}
			cephHosts, err := c.getCephHosts(newClust.Namespace)
			if err != nil {
				logger.Errorf("failed to find valid ceph hosts in the cluster %q. %v", newClust.Namespace, err)
				return
			}
			go c.startClusterCleanUp(newClust, cephHosts, monSecret, clusterFSID)
		}

		err = c.handleDelete(newClust, time.Duration(clusterDeleteRetryInterval)*time.Second)
		if err != nil {
			logger.Errorf("failed finalizer for cluster. %v", err)
			return
		}

		// remove the finalizer from the crd, which indicates to k8s that the resource can safely be deleted
		c.removeFinalizer(newClust)
		return
	}

	if newClust.Spec.CleanupPolicy.HasDataDirCleanPolicy() {
		logger.Infof("skipping orchestration for cluster object %q in namespace %q because its cleanup policy is set", newClust.Name, newClust.Namespace)
		return
	}

	cluster, ok := c.clusterMap[newClust.Namespace]
	if !ok {
		logger.Errorf("cannot update cluster %q that does not exist", newClust.Namespace)
		return
	}

	// If the cluster was never initialized during the OnAdd() method due to a failure, we must
	// treat the cluster as if it was just created.
	if !cluster.initialized() {
		logger.Infof("update event for uninitialized cluster %q. Initializing...", newClust.Namespace)
		c.initializeCluster(cluster, newClust)
		return
	}

	changed, _ := clusterChanged(oldClust.Spec, newClust.Spec, cluster)
	if !changed {
		logger.Debugf("update event for cluster %q is not supported", newClust.Namespace)
		return
	}

	logger.Infof("update event for cluster %q is supported, orchestrating update now", newClust.Namespace)

	config.ConditionExport(c.context, newClust.Namespace, newClust.Name,
		cephv1.ConditionUpdating, v1.ConditionTrue, "ClusterUpdating", "Cluster is updating")

	if oldClust.Spec.RemoveOSDsIfOutAndSafeToRemove != newClust.Spec.RemoveOSDsIfOutAndSafeToRemove {
		logger.Infof("removeOSDsIfOutAndSafeToRemove is set to %t", newClust.Spec.RemoveOSDsIfOutAndSafeToRemove)
		c.osdChecker.Update(newClust.Spec.RemoveOSDsIfOutAndSafeToRemove)
	}

	logger.Debugf("old cluster: %+v", oldClust.Spec)
	logger.Debugf("new cluster: %+v", newClust.Spec)

	cluster.Spec = &newClust.Spec

	// if the image changed, we need to detect the new image version
	versionChanged := false
	if oldClust.Spec.CephVersion.Image != newClust.Spec.CephVersion.Image {
		logger.Infof("the ceph version changed from %q to %q", oldClust.Spec.CephVersion.Image, newClust.Spec.CephVersion.Image)
		version, _, err := c.detectAndValidateCephVersion(cluster, newClust.Spec.CephVersion.Image)
		if err != nil {
			logger.Errorf("unknown ceph major version. %q", err)
			return
		}
		versionChanged = true
		cluster.Info.CephVersion = *version
	}

	// Get cluster running versions
	versions, err := client.GetAllCephDaemonVersions(c.context, cluster.Namespace)
	if err != nil {
		logger.Errorf("failed to get ceph daemons versions. %q", err)
		return
	}
	runningVersions := *versions

	// If the image version changed let's make sure we can safely upgrade
	// Also we make sure there is actually an upgrade to perform
	// It's not because the image spec changed that the ceph version did
	// Someone could use the same Ceph version but with a different base OS content
	cluster.isUpgrade = false
	if versionChanged {
		// we compare against cluster.Info.CephVersion since it received the new spec version earlier
		// so don't get confused by the name of the function and its arguments
		updateOrNot, err := diffImageSpecAndClusterRunningVersion(cluster.Info.CephVersion, runningVersions)
		if err != nil {
			logger.Errorf("failed to determine if we should upgrade or not. %v", err)
			return
		}

		if updateOrNot {
			// If the image version changed let's make sure we can safely upgrade
			// check ceph's status, if not healthy we fail
			cephStatus := client.IsCephHealthy(c.context, cluster.Namespace)
			if !cephStatus {
				if cluster.Spec.SkipUpgradeChecks {
					logger.Warning("ceph is not healthy but SkipUpgradeChecks is set, forcing upgrade.")
				} else {
					logger.Errorf("ceph status in namespace %q is not healthy, refusing to upgrade. fix the cluster and re-edit the cluster CR to trigger a new orchestation update", cluster.Namespace)
					return
				}
			}
			// If Ceph is healthy let's start the upgrade!
			config.ConditionExport(c.context, newClust.Namespace, newClust.Name,
				cephv1.ConditionUpgrading, v1.ConditionTrue, "ClusterUpgrading", "Cluster is upgrading")
			cluster.isUpgrade = true
		}
		// If Ceph is healthy let's start the upgrade!
		cluster.isUpgrade = true
	} else {
		logger.Infof("ceph daemons running versions are: %+v", runningVersions)
	}

	// attempt to update the cluster.  note this is done outside of wait.Poll because that function
	// will wait for the retry interval before trying for the first time.
	done, _ := c.handleUpdate(newClust.Name, cluster)
	if done {
		return
	}

	err = wait.Poll(updateClusterInterval, updateClusterTimeout, func() (bool, error) {
		return c.handleUpdate(newClust.Name, cluster)
	})
	if err != nil {
		config.ConditionExport(c.context, newClust.Namespace, newClust.Name, cephv1.ConditionUpgrading, v1.ConditionFalse, "ClusterUpgradeFailure",
			fmt.Sprintf("giving up trying to update cluster in namespace %q after %q. %v", cluster.Namespace, updateClusterTimeout, err))
		return
	}

	// Display success after upgrade
	if versionChanged {
		message := "Cluster upgraded successfully"
		config.ConditionExport(c.context, newClust.Namespace, newClust.Name,
			cephv1.ConditionReady, v1.ConditionTrue, "ClusterReady", message)
		printOverallCephVersion(c.context, cluster.Namespace)
	}
}

func (c *ClusterController) detectAndValidateCephVersion(cluster *cluster, image string) (*cephver.CephVersion, bool, error) {
	version, err := cluster.detectCephVersion(c.rookImage, image, detectCephVersionTimeout)
	if err != nil {
		return nil, true, err
	}
	if err := cluster.validateCephVersion(version); err != nil {
		return nil, false, err
	}
	c.updateClusterCephVersion(cluster.Namespace, cluster.crdName, image, *version)
	return version, false, nil
}

func (c *ClusterController) handleUpdate(crdName string, cluster *cluster) (bool, error) {

	config.ConditionExport(c.context, cluster.Namespace, crdName,
		cephv1.ConditionUpdating, v1.ConditionTrue, "ClusterUpdating", "Cluster is updating")
	if err := cluster.createInstance(c.rookImage, cluster.Info.CephVersion); err != nil {
		logger.Errorf("failed to update cluster in namespace %q. %v", cluster.Namespace, err)
		return false, nil
	}
	config.ConditionExport(c.context, cluster.Namespace, crdName,
		cephv1.ConditionReady, v1.ConditionTrue, "ClusterUpdated", "Cluster updated successfully")
	logger.Infof("succeeded updating cluster in namespace %q", cluster.Namespace)
	return true, nil
}

func (c *ClusterController) onDeviceCMUpdate(oldObj, newObj interface{}) {
	oldCm, ok := oldObj.(*v1.ConfigMap)
	if !ok {
		logger.Warningf("Expected ConfigMap but handler received %#v", oldObj)
		return
	}
	logger.Debugf("onDeviceCMUpdate old device cm: %+v", oldCm)

	newCm, ok := newObj.(*v1.ConfigMap)
	if !ok {
		logger.Warningf("Expected ConfigMap but handler received %#v", newObj)
		return
	}
	logger.Debugf("onDeviceCMUpdate new device cm: %+v", newCm)

	oldDevStr, ok := oldCm.Data[discoverDaemon.LocalDiskCMData]
	if !ok {
		logger.Warningf("unexpected configmap data")
		return
	}

	newDevStr, ok := newCm.Data[discoverDaemon.LocalDiskCMData]
	if !ok {
		logger.Warningf("unexpected configmap data")
		return
	}

	devicesEqual, err := discoverDaemon.DeviceListsEqual(oldDevStr, newDevStr)
	if err != nil {
		logger.Warningf("failed to compare device lists: %v", err)
		return
	}

	if devicesEqual {
		logger.Debugf("device lists are equal. skipping orchestration")
		return
	}

	for _, cluster := range c.clusterMap {
		if cluster.Info == nil {
			logger.Infof("Cluster %s is not ready. Skipping orchestration on device change", cluster.Namespace)
			continue
		}
		if len(cluster.Spec.Storage.StorageClassDeviceSets) > 0 {
			logger.Info("skip orchestration on device config map update for OSDs on PVC")
			continue
		}
		logger.Infof("Running orchestration for namespace %s after device change", cluster.Namespace)
		err := cluster.createInstance(c.rookImage, cluster.Info.CephVersion)
		if err != nil {
			logger.Errorf("Failed orchestration after device change in namespace %q. %v", cluster.Namespace, err)
			continue
		}
	}
}

// ************************************************************************************************
// Delete event functions
// ************************************************************************************************

func (c *ClusterController) onDelete(obj interface{}) {
	clust, err := getClusterObject(obj)
	if err != nil {
		logger.Errorf("failed to get cluster object. %v", err)
		return
	}

	config.ConditionExport(c.context, clust.Namespace, clust.Name,
		cephv1.ConditionDeleting, v1.ConditionTrue, "ClusterDeleting", "Cluster is deleting")

	if existing, ok := c.clusterMap[clust.Namespace]; ok && existing.crdName != clust.Name {
		logger.Errorf("Skipping deletion of cluster cr %q in namespace %q. Cluster cr %q already exists in this namespace. Only one cluster cr per namespace is supported.",
			clust.Name, clust.Namespace, existing.crdName)
		return
	}

	logger.Infof("delete event for cluster %q in namespace %q", clust.Name, clust.Namespace)

	err = c.handleDelete(clust, time.Duration(clusterDeleteRetryInterval)*time.Second)
	if err != nil {
		config.ConditionExport(c.context, clust.Namespace, clust.Name,
			cephv1.ConditionDeleting, v1.ConditionTrue, "ClusterDeleting", "Failed to delete cluster")
		logger.Errorf("failed to delete cluster. %v", err)
	}

	if cluster, ok := c.clusterMap[clust.Namespace]; ok {
		close(cluster.stopCh)
		delete(c.clusterMap, clust.Namespace)
	}

	// Only valid when the cluster is not external
	if clust.Spec.External.Enable {
		err := purgeExternalCluster(c.context.Clientset, clust.Namespace)
		if err != nil {
			config.ConditionExport(c.context, clust.Namespace, clust.Name,
				cephv1.ConditionDeleting, v1.ConditionTrue, "ClusterDeleting", "Failed to purge external cluster resources")
			logger.Errorf("failed to purge external cluster resources. %v", err)
		}
		return
	}
}

func (c *ClusterController) waitForFlexVolumeCleanup(cluster *cephv1.CephCluster, operatorNamespace string, retryInterval time.Duration) error {
	retryCount := 0
	for {
		// TODO: filter this List operation by cluster namespace on the server side
		vols, err := c.volumeAttachment.List(operatorNamespace)
		if err != nil {
			return errors.Wrapf(err, "failed to get volume attachments for operator namespace %q", operatorNamespace)
		}

		// find volume attachments in the deleted cluster
		attachmentsExist := false
	AttachmentLoop:
		for _, vol := range vols.Items {
			for _, a := range vol.Attachments {
				if a.ClusterName == cluster.Namespace {
					// there is still an outstanding volume attachment in the cluster that is being deleted.
					attachmentsExist = true
					break AttachmentLoop
				}
			}
		}

		if !attachmentsExist {
			logger.Infof("no volume attachments for cluster %s to clean up.", cluster.Namespace)
			break
		}

		retryCount++
		if retryCount == clusterDeleteMaxRetries {
			logger.Warningf(
				"exceeded retry count while waiting for volume attachments for cluster %q to be cleaned up. vols: %+v",
				cluster.Namespace,
				vols.Items)
			break
		}

		logger.Infof("waiting for volume attachments in cluster %q to be cleaned up. Retrying in %q.",
			cluster.Namespace, retryInterval.String())
		<-time.After(retryInterval)
	}
	return nil
}

func (c *ClusterController) checkPVPresentInCluster(drivers []string, clusterID string) (bool, error) {
	pv, err := c.context.Clientset.CoreV1().PersistentVolumes().List(metav1.ListOptions{})
	if err != nil {
		return false, errors.Wrapf(err, "failed to list PV")
	}

	for _, p := range pv.Items {
		if p.Spec.CSI == nil {
			logger.Errorf("Spec.CSI is nil for PV %q", p.Name)
			continue
		}
		if p.Spec.CSI.VolumeAttributes["clusterID"] == clusterID {
			//check PV is created by drivers deployed by rook
			for _, d := range drivers {
				if d == p.Spec.CSI.Driver {
					return true, nil
				}
			}

		}
	}
	return false, nil
}

func (c *ClusterController) waitForCSIVolumeCleanup(cluster *cephv1.CephCluster, retryInterval time.Duration) error {
	retryCount := 0
	drivers := []string{csi.CephFSDriverName, csi.RBDDriverName}
	for {
		logger.Infof("checking any PVC created by drivers %q and %q with clusterID %q", csi.CephFSDriverName, csi.RBDDriverName, cluster.Namespace)
		// check any PV is created in this cluster
		attachmentsExist, err := c.checkPVPresentInCluster(drivers, cluster.Namespace)
		if err != nil {
			return errors.Wrapf(err, "failed to list PersistentVolumes")
		}
		// no PVC created in this cluster
		if !attachmentsExist {
			logger.Infof("no volume attachments for cluster %s", cluster.Namespace)
			break
		}

		retryCount++
		if retryCount == clusterDeleteMaxRetries {
			logger.Warningf(
				"exceeded retry count while waiting for volume attachments for cluster %s to be cleaned up", cluster.Namespace)
			break
		}

		logger.Infof("waiting for volume attachments in cluster %s to be cleaned up. Retrying in %s",
			cluster.Namespace, retryInterval.String())
		<-time.After(retryInterval)
	}
	return nil
}

func (c *ClusterController) handleDelete(cluster *cephv1.CephCluster, retryInterval time.Duration) error {
	if csi.CSIEnabled() {
		err := c.waitForCSIVolumeCleanup(cluster, retryInterval)
		if err != nil {
			return errors.Wrapf(err, "failed to wait for the csi volume cleanup")
		}
	}
	operatorNamespace := os.Getenv(k8sutil.PodNamespaceEnvVar)
	flexDriverEnabled := os.Getenv(enableFlexDriver) != "false"
	if !flexDriverEnabled {
		logger.Debugf("Flex driver disabled: no volume attachments for cluster %s (operator namespace: %s)",
			cluster.Namespace, operatorNamespace)
		return nil
	}
	err := c.waitForFlexVolumeCleanup(cluster, operatorNamespace, retryInterval)
	return err
}

func purgeExternalCluster(clientset kubernetes.Interface, namespace string) error {
	// Purge the config maps
	cmsToDelete := []string{
		mon.EndpointConfigMapName,
		k8sutil.ConfigOverrideName,
	}
	for _, cm := range cmsToDelete {
		err := clientset.CoreV1().ConfigMaps(namespace).Delete(cm, &metav1.DeleteOptions{})
		if err != nil && !kerrors.IsNotFound(err) {
			logger.Errorf("failed to delete config map %q. %v", cm, err)
		}
	}

	// Purge the secrets
	secretsToDelete := []string{
		mon.AppName,
		mon.OperatorCreds,
		csi.CsiRBDNodeSecret,
		csi.CsiRBDProvisionerSecret,
		csi.CsiCephFSNodeSecret,
		csi.CsiCephFSProvisionerSecret,
		config.StoreName,
	}
	for _, secret := range secretsToDelete {
		err := clientset.CoreV1().Secrets(namespace).Delete(secret, &metav1.DeleteOptions{})
		if err != nil && !kerrors.IsNotFound(err) {
			logger.Errorf("failed to delete secret %q. %v", secret, err)
		}
	}

	return nil
}

// ************************************************************************************************
// Finalizer functions
// ************************************************************************************************
func (c *ClusterController) addFinalizer(namespace, name string) error {

	// get the latest cluster object since we probably updated it before we got to this point (e.g. by updating its status)
	clust, err := c.context.RookClientset.CephV1().CephClusters(namespace).Get(name, metav1.GetOptions{})
	if err != nil {
		return err
	}

	// add the finalizer (cephcluster.ceph.rook.io) if it is not yet defined on the cluster CRD
	for _, finalizer := range clust.Finalizers {
		if finalizer == finalizerName {
			logger.Infof("finalizer already set on cluster %s", clust.Namespace)
			return nil
		}
	}

	// adding finalizer to the cluster crd
	clust.Finalizers = append(clust.Finalizers, finalizerName)

	// update the crd
	_, err = c.context.RookClientset.CephV1().CephClusters(clust.Namespace).Update(clust)
	if err != nil {
		return errors.Wrapf(err, "failed to add finalizer to cluster")
	}

	logger.Infof("added finalizer to cluster %s", clust.Name)
	return nil
}

func (c *ClusterController) removeFinalizer(obj interface{}) {
	// first determine what type/version of cluster we are dealing with

	cl, ok := obj.(*cephv1.CephCluster)
	if !ok {
		logger.Warningf("cannot remove finalizer from object that is not a cluster: %+v", obj)
		return
	}

	// update the crd to remove the finalizer for good. retry several times in case of intermittent failures.
	maxRetries := 5
	retrySeconds := 5 * time.Second
	for i := 0; i < maxRetries; i++ {
		// Get the latest cluster instead of using the same instance in case it has been changed
		cluster, err := c.context.RookClientset.CephV1().CephClusters(cl.Namespace).Get(cl.Name, metav1.GetOptions{})
		if err != nil {
			if kerrors.IsNotFound(err) {
				logger.Errorf("cluster was removed, no need to remove finalizer")
			} else {
				logger.Errorf("failed to remove finalizer. failed to get cluster. %v", err)
			}
			return
		}
		objectMeta := &cluster.ObjectMeta

		// remove the finalizer from the slice if it exists
		found := false
		for i, finalizer := range objectMeta.Finalizers {
			if finalizer == finalizerName {
				objectMeta.Finalizers = append(objectMeta.Finalizers[:i], objectMeta.Finalizers[i+1:]...)
				found = true
				break
			}
		}
		if !found {
			logger.Infof("finalizer %q not found in the cluster crd %q", finalizerName, objectMeta.Name)
			return
		}

		_, err = c.context.RookClientset.CephV1().CephClusters(cluster.Namespace).Update(cluster)
		if err != nil {
			logger.Errorf("failed to remove finalizer %q from cluster %q. %v", finalizerName, objectMeta.Name, err)
			time.Sleep(retrySeconds)
			continue
		}
		logger.Infof("removed finalizer %s from cluster %s", finalizerName, objectMeta.Name)
		return
	}

	logger.Warningf("giving up from removing the %s cluster finalizer", finalizerName)
}

func (c *ClusterController) updateClusterCephVersion(namespace string, name string, image string, cephVersion cephver.CephVersion) {
	logger.Infof("cluster %q: version %q detected for image %q", namespace, cephVersion.String(), image)

	// get the most recent cluster CRD object
	cluster, err := c.context.RookClientset.CephV1().CephClusters(namespace).Get(name, metav1.GetOptions{})
	if err != nil {
		logger.Errorf("failed to get cluster from namespace %q prior to updating its Ceph version to %q. %v", namespace, cephVersion.String(), err)
	}
	clusterVersion := &cephv1.ClusterVersion{
		Image:   image,
		Version: controller.GetCephVersionLabel(cephVersion),
	}
	// update the Ceph version on the retrieved cluster object
	// do not overwrite the ceph status that is updated in a separate goroutine
	cluster.Status.CephVersion = clusterVersion
	if _, err := c.context.RookClientset.CephV1().CephClusters(namespace).Update(cluster); err != nil {
		logger.Errorf("failed to update version for cluster %q. %v", namespace, err)
	}
}

func ClusterOwnerRef(clusterName, clusterID string) metav1.OwnerReference {
	blockOwner := true
	return metav1.OwnerReference{
		APIVersion:         fmt.Sprintf("%s/%s", ClusterResource.Group, ClusterResource.Version),
		Kind:               ClusterResource.Kind,
		Name:               clusterName,
		UID:                types.UID(clusterID),
		BlockOwnerDeletion: &blockOwner,
	}
}

func printOverallCephVersion(context *clusterd.Context, namespace string) {
	versions, err := client.GetAllCephDaemonVersions(context, namespace)
	if err != nil {
		logger.Errorf("failed to get ceph daemons versions. %v", err)
		return
	}

	if len(versions.Overall) == 1 {
		for v := range versions.Overall {
			version, err := cephver.ExtractCephVersion(v)
			if err != nil {
				logger.Errorf("failed to extract ceph version. %v", err)
				return
			}
			vv := *version
			logger.Infof("successfully upgraded cluster to version: %q", vv.String())
		}
	} else {
		// This shouldn't happen, but let's log just in case
		logger.Warningf("upgrade orchestration completed but somehow we still have more than one Ceph version running. %v:", versions.Overall)
	}
}

func validateExternalClusterSpec(cluster *cluster) error {
	if cluster.Spec.CephVersion.Image != "" {
		if cluster.Spec.DataDirHostPath == "" {
			return errors.New("dataDirHostPath must be specified")
		}
	}

	return nil
}

func populateConfigOverrideConfigMap(context *clusterd.Context, namespace string, ownerRef metav1.OwnerReference) error {
	placeholderConfig := map[string]string{
		k8sutil.ConfigOverrideVal: "",
	}

	cm := &v1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name: k8sutil.ConfigOverrideName,
		},
		Data: placeholderConfig,
	}

	k8sutil.SetOwnerRef(&cm.ObjectMeta, &ownerRef)
	_, err := context.Clientset.CoreV1().ConfigMaps(namespace).Create(cm)
	if err != nil && !kerrors.IsAlreadyExists(err) {
		return errors.Wrapf(err, "failed to create override configmap %s", namespace)
	}

	return nil
}
