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

// Package cluster to manage a rook cluster.
package cluster

import (
	"fmt"
	"os"
	"reflect"
	"sort"
	"time"

	"github.com/coreos/pkg/capnslog"
	opkit "github.com/rook/operator-kit"
	rookalpha "github.com/rook/rook/pkg/apis/rook.io/v1alpha1"
	"github.com/rook/rook/pkg/clusterd"
	"github.com/rook/rook/pkg/daemon/agent/flexvolume/attachment"
	"github.com/rook/rook/pkg/daemon/ceph/client"
	"github.com/rook/rook/pkg/operator/cluster/api"
	"github.com/rook/rook/pkg/operator/cluster/ceph/mgr"
	"github.com/rook/rook/pkg/operator/cluster/ceph/mon"
	"github.com/rook/rook/pkg/operator/cluster/ceph/osd"
	"github.com/rook/rook/pkg/operator/file"
	"github.com/rook/rook/pkg/operator/k8sutil"
	"github.com/rook/rook/pkg/operator/object"
	"github.com/rook/rook/pkg/operator/pool"
	"k8s.io/api/core/v1"
	apiextensionsv1beta1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1beta1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/tools/cache"
	"k8s.io/kubernetes/pkg/util/version"
)

const (
	CustomResourceName       = "cluster"
	CustomResourceNamePlural = "clusters"
	crushConfigMapName       = "rook-crush-config"
	crushmapCreatedKey       = "initialCrushMapCreated"
	clusterCreateInterval    = 6 * time.Second
	clusterCreateTimeout     = 5 * time.Minute
	updateClusterInterval    = 30 * time.Second
	updateClusterTimeout     = 1 * time.Hour
	defaultMonCount          = 3
	maxMonCount              = 9
)

const (
	// DefaultClusterName states the default name of the rook-cluster if not provided.
	DefaultClusterName         = "rook"
	clusterDeleteRetryInterval = 2 //seconds
	clusterDeleteMaxRetries    = 15
)

var (
	logger        = capnslog.NewPackageLogger("github.com/rook/rook", "op-cluster")
	finalizerName = fmt.Sprintf("%s.%s", ClusterResource.Name, ClusterResource.Group)
)

var ClusterResource = opkit.CustomResource{
	Name:    CustomResourceName,
	Plural:  CustomResourceNamePlural,
	Group:   rookalpha.CustomResourceGroup,
	Version: rookalpha.Version,
	Scope:   apiextensionsv1beta1.NamespaceScoped,
	Kind:    reflect.TypeOf(rookalpha.Cluster{}).Name(),
}

// ClusterController controls an instance of a Rook cluster
type ClusterController struct {
	context          *clusterd.Context
	volumeAttachment attachment.Attachment
	devicesInUse     bool
	rookImage        string
}

type cluster struct {
	context   *clusterd.Context
	Namespace string
	Spec      rookalpha.ClusterSpec
	mons      *mon.Cluster
	mgrs      *mgr.Cluster
	osds      *osd.Cluster
	apis      *api.Cluster
	stopCh    chan struct{}
	ownerRef  metav1.OwnerReference
}

// NewClusterController create controller for watching cluster custom resources created
func NewClusterController(context *clusterd.Context, rookImage string, volumeAttachment attachment.Attachment) *ClusterController {
	return &ClusterController{
		context:          context,
		volumeAttachment: volumeAttachment,
		rookImage:        rookImage,
	}
}

// Watch watches instances of cluster resources
func (c *ClusterController) StartWatch(namespace string, stopCh chan struct{}) error {

	resourceHandlerFuncs := cache.ResourceEventHandlerFuncs{
		AddFunc:    c.onAdd,
		UpdateFunc: c.onUpdate,
		DeleteFunc: c.onDelete,
	}

	logger.Infof("start watching clusters in all namespaces")
	watcher := opkit.NewWatcher(ClusterResource, namespace, resourceHandlerFuncs, c.context.RookClientset.Rook().RESTClient())
	go watcher.Watch(&rookalpha.Cluster{}, stopCh)
	return nil
}

func (c *ClusterController) onAdd(obj interface{}) {
	clusterObj := obj.(*rookalpha.Cluster).DeepCopy()

	cluster := newCluster(clusterObj, c.context)
	logger.Infof("starting cluster in namespace %s", cluster.Namespace)

	if c.devicesInUse && cluster.Spec.Storage.AnyUseAllDevices() {
		message := "using all devices in more than one namespace not supported"
		logger.Error(message)
		if err := c.updateClusterStatus(clusterObj.Namespace, clusterObj.Name, rookalpha.ClusterStateError, message); err != nil {
			logger.Errorf("failed to update cluster status in namespace %s: %+v", cluster.Namespace, err)
		}
		return
	}

	if cluster.Spec.Storage.AnyUseAllDevices() {
		c.devicesInUse = true
	}

	if cluster.Spec.MonCount <= 0 {
		logger.Warningf("mon count is 0 or less (given: %d), should be greater than 0, defaulting to %d", cluster.Spec.MonCount, defaultMonCount)
		cluster.Spec.MonCount = defaultMonCount
	}
	if cluster.Spec.MonCount > maxMonCount {
		logger.Warningf("mon count is bigger than %d (given: %d), not supported, changing to %d", maxMonCount, cluster.Spec.MonCount, maxMonCount)
		cluster.Spec.MonCount = maxMonCount
	}
	if cluster.Spec.MonCount%2 == 0 {
		logger.Warningf("mon count is even (given: %d), should be uneven, continuing", cluster.Spec.MonCount)
	}

	// Start the Rook cluster components. Retry several times in case of failure.
	err := wait.Poll(clusterCreateInterval, clusterCreateTimeout, func() (bool, error) {
		if err := c.updateClusterStatus(clusterObj.Namespace, clusterObj.Name, rookalpha.ClusterStateCreating, ""); err != nil {
			logger.Errorf("failed to update cluster status in namespace %s: %+v", cluster.Namespace, err)
			return false, nil
		}

		err := cluster.createInstance(c.rookImage)
		if err != nil {
			logger.Errorf("failed to create cluster in namespace %s. %+v", cluster.Namespace, err)
			return false, nil
		}

		// cluster is created, update the cluster CRD status now
		if err := c.updateClusterStatus(clusterObj.Namespace, clusterObj.Name, rookalpha.ClusterStateCreated, ""); err != nil {
			logger.Errorf("failed to update cluster status in namespace %s: %+v", cluster.Namespace, err)
			return false, nil
		}

		return true, nil
	})
	if err != nil {
		message := fmt.Sprintf("giving up creating cluster in namespace %s after %s", cluster.Namespace, clusterCreateTimeout)
		logger.Error(message)
		if err := c.updateClusterStatus(clusterObj.Namespace, clusterObj.Name, rookalpha.ClusterStateError, message); err != nil {
			logger.Errorf("failed to update cluster status in namespace %s: %+v", cluster.Namespace, err)
		}
		return
	}

	// Start pool CRD watcher
	poolController := pool.NewPoolController(c.context)
	poolController.StartWatch(cluster.Namespace, cluster.stopCh)

	// Start object store CRD watcher
	objectStoreController := object.NewObjectStoreController(c.context, c.rookImage, cluster.Spec.HostNetwork, cluster.ownerRef)
	objectStoreController.StartWatch(cluster.Namespace, cluster.stopCh)

	// Start file system CRD watcher
	fileController := file.NewFilesystemController(c.context, c.rookImage, cluster.Spec.HostNetwork, cluster.ownerRef)
	fileController.StartWatch(cluster.Namespace, cluster.stopCh)

	// Start mon endpoint watcher
	endpointWatcher := mon.NewEndpointWatcher(cluster.mons)
	go endpointWatcher.StartWatch(cluster.stopCh)

	// Start mon health checker
	healthChecker := mon.NewHealthChecker(cluster.mons)
	go healthChecker.Check(cluster.stopCh)

	// add the finalizer to the crd
	err = c.addFinalizer(clusterObj)
	if err != nil {
		logger.Errorf("failed to add finalizer to cluster crd. %+v", err)
	}
}

func (c *ClusterController) onUpdate(oldObj, newObj interface{}) {
	oldClust := oldObj.(*rookalpha.Cluster).DeepCopy()
	newClust := newObj.(*rookalpha.Cluster).DeepCopy()
	logger.Infof("update event for cluster %s", newClust.Namespace)

	// Check if the cluster is being deleted. This code path is called when a finalizer is specified in the crd.
	// When a cluster is requested for deletion, K8s will only set the deletion timestamp if there are any finalizers in the list.
	// K8s will only delete the crd and child resources when the finalizers have been removed from the crd.
	if newClust.DeletionTimestamp != nil {
		logger.Infof("cluster %s has a deletion timestamp", newClust.Namespace)
		err := c.handleDelete(newClust, time.Duration(clusterDeleteRetryInterval)*time.Second)
		if err != nil {
			logger.Errorf("failed finalizer for cluster. %+v", err)
			return
		}
		// remove the finalizer from the crd, which indicates to k8s that the resource can safely be deleted
		c.removeFinalizer(newClust)
		return
	}

	if !clusterChanged(oldClust.Spec, newClust.Spec) {
		logger.Infof("update event for cluster %s is not supported", newClust.Namespace)
		return
	}

	logger.Infof("update event for cluster %s is supported, orchestrating update now", newClust.Namespace)
	logger.Debugf("old cluster: %+v", oldClust.Spec)
	logger.Debugf("new cluster: %+v", newClust.Spec)

	cluster := newCluster(newClust, c.context)

	// attempt to update the cluster.  note this is done outside of wait.Poll because that function
	// will wait for the retry interval before trying for the first time.
	done, _ := c.handleUpdate(newClust, cluster)
	if done {
		return
	}

	err := wait.Poll(updateClusterInterval, updateClusterTimeout, func() (bool, error) {
		return c.handleUpdate(newClust, cluster)
	})
	if err != nil {
		message := fmt.Sprintf("giving up trying to update cluster in namespace %s after %s", cluster.Namespace, updateClusterTimeout)
		logger.Error(message)
		if err := c.updateClusterStatus(newClust.Namespace, newClust.Name, rookalpha.ClusterStateError, message); err != nil {
			logger.Errorf("failed to update cluster status in namespace %s: %+v", newClust.Namespace, err)
		}
		return
	}
}

func (c *ClusterController) handleUpdate(newClust *rookalpha.Cluster, cluster *cluster) (bool, error) {
	if err := c.updateClusterStatus(newClust.Namespace, newClust.Name, rookalpha.ClusterStateUpdating, ""); err != nil {
		logger.Errorf("failed to update cluster status in namespace %s: %+v", newClust.Namespace, err)
		return false, nil
	}

	if err := cluster.createInstance(c.rookImage); err != nil {
		logger.Errorf("failed to update cluster in namespace %s. %+v", newClust.Namespace, err)
		return false, nil
	}

	if err := c.updateClusterStatus(newClust.Namespace, newClust.Name, rookalpha.ClusterStateCreated, ""); err != nil {
		logger.Errorf("failed to update cluster status in namespace %s: %+v", newClust.Namespace, err)
		return false, nil
	}

	logger.Infof("succeeded updating cluster in namespace %s", newClust.Namespace)
	return true, nil
}

func (c *ClusterController) onDelete(obj interface{}) {
	clust := obj.(*rookalpha.Cluster).DeepCopy()
	err := c.handleDelete(clust, time.Duration(clusterDeleteRetryInterval)*time.Second)
	if err != nil {
		logger.Errorf("failed to delete cluster. %+v", err)
	}
}

func (c *ClusterController) addFinalizer(clust *rookalpha.Cluster) error {
	kubeVersion, err := k8sutil.GetK8SVersion(c.context.Clientset)
	if err != nil {
		return fmt.Errorf("Error getting server version: %v", err)
	}
	if !kubeVersion.AtLeast(version.MustParseSemantic(k8sutil.FirstCRDVersion)) {
		logger.Infof("not adding finalizer for cluster TPR")
		return nil
	}

	// get the latest cluster object since we probably updated it before we got to this point (e.g. by updating its status)
	clust, err = c.context.RookClientset.RookV1alpha1().Clusters(clust.Namespace).Get(clust.Name, metav1.GetOptions{})
	if err != nil {
		return err
	}

	// add the finalizer (cluster.rook.io) if it is not yet defined on the cluster CRD
	for _, finalizer := range clust.Finalizers {
		if finalizer == finalizerName {
			logger.Infof("finalizer already set on cluster %s", clust.Namespace)
			return nil
		}
	}

	// adding finalizer to the cluster crd
	clust.Finalizers = append(clust.Finalizers, finalizerName)

	// update the crd
	_, err = c.context.RookClientset.RookV1alpha1().Clusters(clust.Namespace).Update(clust)
	if err != nil {
		return fmt.Errorf("failed to add finalizer to cluster. %+v", err)
	}

	logger.Infof("added finalizer to cluster %s", clust.Name)
	return nil
}

func (c *ClusterController) removeFinalizer(clust *rookalpha.Cluster) {
	// remove the finalizer (cluster.rook.io) if found in the slice
	found := false
	for i, finalizer := range clust.Finalizers {
		if finalizer == finalizerName {
			clust.Finalizers = append(clust.Finalizers[:i], clust.Finalizers[i+1:]...)
			found = true
			break
		}
	}
	if !found {
		logger.Infof("finalizer cluster.rook.io not found in the crd '%s'", clust.Name)
		return
	}

	// update the crd. retry several times in case of intermittent failures.
	maxRetries := 5
	retrySeconds := 5 * time.Second
	for i := 0; i < maxRetries; i++ {
		_, err := c.context.RookClientset.RookV1alpha1().Clusters(clust.Namespace).Update(clust)
		if err != nil {
			logger.Errorf("failed to remove finalizer from cluster. %+v", err)
			time.Sleep(retrySeconds)
			continue
		}
		logger.Infof("removed finalizer from cluster %s", clust.Name)
		return
	}

	logger.Warningf("giving up from removing the cluster finalizer")
}

func (c *ClusterController) handleDelete(cluster *rookalpha.Cluster, retryInterval time.Duration) error {

	operatorNamespace := os.Getenv(k8sutil.PodNamespaceEnvVar)
	retryCount := 0
	for {
		// TODO: filter this List operation by cluster namespace on the server side
		vols, err := c.volumeAttachment.List(operatorNamespace)
		if err != nil {
			return fmt.Errorf("failed to get volume attachments for operator namespace %s: %+v", operatorNamespace, err)
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
				"exceeded retry count while waiting for volume attachments for cluster %s to be cleaned up. vols: %+v",
				cluster.Namespace,
				vols.Items)
			break
		}

		logger.Infof("waiting for volume attachments in cluster %s to be cleaned up. Retrying in %s.",
			cluster.Namespace, retryInterval)
		<-time.After(retryInterval)
	}

	return nil
}

func (c *ClusterController) updateClusterStatus(namespace, name string, state rookalpha.ClusterState, message string) error {
	// get the most recent cluster CRD object
	cluster, err := c.context.RookClientset.RookV1alpha1().Clusters(namespace).Get(name, metav1.GetOptions{})
	if err != nil {
		return fmt.Errorf("failed to get cluster from namespace %s prior to updating its status: %+v", namespace, err)
	}

	// update the status on the retrieved cluster object
	cluster.Status = rookalpha.ClusterStatus{State: state, Message: message}
	if _, err := c.context.RookClientset.RookV1alpha1().Clusters(cluster.Namespace).Update(cluster); err != nil {
		return fmt.Errorf("failed to update cluster %s status: %+v", cluster.Namespace, err)
	}

	return nil
}

func newCluster(c *rookalpha.Cluster, context *clusterd.Context) *cluster {
	return &cluster{Namespace: c.Namespace, Spec: c.Spec, context: context, ownerRef: ClusterOwnerRef(c.Namespace, string(c.UID))}
}

func ClusterOwnerRef(namespace, clusterID string) metav1.OwnerReference {
	blockOwner := true
	return metav1.OwnerReference{
		APIVersion:         ClusterResource.Version,
		Kind:               ClusterResource.Kind,
		Name:               namespace,
		UID:                types.UID(clusterID),
		BlockOwnerDeletion: &blockOwner,
	}
}

func (c *cluster) createInstance(rookImage string) error {

	// Create a configmap for overriding ceph config settings
	// These settings should only be modified by a user after they are initialized
	placeholderConfig := map[string]string{
		k8sutil.ConfigOverrideVal: "",
	}
	cm := &v1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:            k8sutil.ConfigOverrideName,
			OwnerReferences: []metav1.OwnerReference{c.ownerRef},
		},
		Data: placeholderConfig,
	}

	_, err := c.context.Clientset.CoreV1().ConfigMaps(c.Namespace).Create(cm)
	if err != nil && !errors.IsAlreadyExists(err) {
		return fmt.Errorf("failed to create override configmap %s. %+v", c.Namespace, err)
	}

	// Start the mon pods
	c.mons = mon.New(c.context, c.Namespace, c.Spec.DataDirHostPath, rookImage, c.Spec.MonCount, c.Spec.Placement.GetMon(), c.Spec.HostNetwork, c.Spec.Resources.Mon, c.ownerRef)
	err = c.mons.Start()
	if err != nil {
		return fmt.Errorf("failed to start the mons. %+v", err)
	}

	err = c.createInitialCrushMap()
	if err != nil {
		return fmt.Errorf("failed to create initial crushmap: %+v", err)
	}

	c.mgrs = mgr.New(c.context, c.Namespace, rookImage, c.Spec.Placement.GetMgr(), c.Spec.HostNetwork, c.Spec.Resources.Mgr, c.ownerRef)
	err = c.mgrs.Start()
	if err != nil {
		return fmt.Errorf("failed to start the ceph mgr. %+v", err)
	}

	c.apis = api.New(c.context, c.Namespace, rookImage, c.Spec.Placement.GetAPI(), c.Spec.HostNetwork, c.Spec.Resources.API, c.ownerRef)
	err = c.apis.Start()
	if err != nil {
		return fmt.Errorf("failed to start the REST api. %+v", err)
	}

	// Start the OSDs
	c.osds = osd.New(c.context, c.Namespace, rookImage, c.Spec.Storage, c.Spec.DataDirHostPath,
		c.Spec.Placement.GetOSD(), c.Spec.HostNetwork, c.Spec.Resources.OSD, c.ownerRef)
	err = c.osds.Start()
	if err != nil {
		return fmt.Errorf("failed to start the osds. %+v", err)
	}

	logger.Infof("Done creating rook instance in namespace %s", c.Namespace)
	return nil
}

func (c *cluster) createInitialCrushMap() error {
	configMapExists := false
	createCrushMap := false

	cm, err := c.context.Clientset.CoreV1().ConfigMaps(c.Namespace).Get(crushConfigMapName, metav1.GetOptions{})
	if err != nil {
		if !errors.IsNotFound(err) {
			return err
		}

		// crush config map was not found, meaning we haven't created the initial crush map
		createCrushMap = true
	} else {
		// crush config map was found, look in it to verify we've created the initial crush map
		configMapExists = true
		val, ok := cm.Data[crushmapCreatedKey]
		if !ok {
			createCrushMap = true
		} else if val != "1" {
			createCrushMap = true
		}
	}

	if !createCrushMap {
		// no need to create the crushmap, bail out
		return nil
	}

	logger.Info("creating initial crushmap")
	out, err := client.CreateDefaultCrushMap(c.context, c.Namespace)
	if err != nil {
		return fmt.Errorf("failed to create initial crushmap: %+v. output: %s", err, out)
	}

	logger.Info("created initial crushmap")

	// save the fact that we've created the initial crushmap to a configmap
	configMap := &v1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:            crushConfigMapName,
			Namespace:       c.Namespace,
			OwnerReferences: []metav1.OwnerReference{c.ownerRef},
		},
		Data: map[string]string{crushmapCreatedKey: "1"},
	}

	if !configMapExists {
		if _, err := c.context.Clientset.CoreV1().ConfigMaps(c.Namespace).Create(configMap); err != nil {
			return fmt.Errorf("failed to create configmap %s: %+v", crushConfigMapName, err)
		}
	} else {
		if _, err = c.context.Clientset.CoreV1().ConfigMaps(c.Namespace).Update(configMap); err != nil {
			return fmt.Errorf("failed to update configmap %s: %+v", crushConfigMapName, err)
		}
	}

	return nil
}

func clusterChanged(oldCluster, newCluster rookalpha.ClusterSpec) bool {

	oldStorage := oldCluster.Storage
	newStorage := newCluster.Storage

	// sort the nodes by name then compare to see if there are changes
	sort.Sort(rookalpha.NodesByName(oldStorage.Nodes))
	sort.Sort(rookalpha.NodesByName(newStorage.Nodes))
	if !reflect.DeepEqual(oldStorage.Nodes, newStorage.Nodes) {
		// the nodes list has changed
		return true
	}

	// none of the supported cluster updates were detected
	return false
}
