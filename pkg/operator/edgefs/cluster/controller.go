/*
Copyright 2019 The Rook Authors. All rights reserved.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.

Portions of this file came from https://github.com/cockroachdb/cockroach, which uses the same license.
*/

// Package cluster to manage a edgefs cluster.
package cluster

import (
	"fmt"
	"reflect"
	"time"

	"github.com/coreos/pkg/capnslog"
	edgefsv1 "github.com/rook/rook/pkg/apis/edgefs.rook.io/v1"
	"github.com/rook/rook/pkg/clusterd"
	"github.com/rook/rook/pkg/operator/edgefs/iscsi"
	"github.com/rook/rook/pkg/operator/edgefs/isgw"
	"github.com/rook/rook/pkg/operator/edgefs/nfs"
	"github.com/rook/rook/pkg/operator/edgefs/s3"
	"github.com/rook/rook/pkg/operator/edgefs/s3x"
	"github.com/rook/rook/pkg/operator/edgefs/swift"
	"github.com/rook/rook/pkg/operator/k8sutil"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/tools/cache"
)

var (
	logger        = capnslog.NewPackageLogger("github.com/rook/rook", "edgefs-op-cluster")
	finalizerName = fmt.Sprintf("%s.%s", ClusterResource.Name, ClusterResource.Group)
)

const (
	CustomResourceName         = "cluster"
	CustomResourceNamePlural   = "clusters"
	appName                    = "rook-edgefs"
	clusterCreateInterval      = 6 * time.Second
	clusterCreateTimeout       = 5 * time.Minute
	updateClusterInterval      = 30 * time.Second
	updateClusterTimeout       = 1 * time.Hour
	clusterDeleteRetryInterval = 2 //seconds
	clusterDeleteMaxRetries    = 15
	defaultEdgefsImageName     = "edgefs/edgefs:latest"
)

var ClusterResource = k8sutil.CustomResource{
	Name:    CustomResourceName,
	Plural:  CustomResourceNamePlural,
	Group:   edgefsv1.CustomResourceGroup,
	Version: edgefsv1.Version,
	Kind:    reflect.TypeOf(edgefsv1.Cluster{}).Name(),
}

type ClusterController struct {
	context        *clusterd.Context
	containerImage string
	devicesInUse   bool
	clusterMap     map[string]*cluster
}

func NewClusterController(context *clusterd.Context, containerImage string) *ClusterController {
	return &ClusterController{
		context:        context,
		containerImage: containerImage,
		clusterMap:     make(map[string]*cluster),
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

func (c *ClusterController) StartWatch(namespace string, stopCh chan struct{}) error {

	resourceHandlerFuncs := cache.ResourceEventHandlerFuncs{
		AddFunc:    c.onAdd,
		UpdateFunc: c.onUpdate,
		DeleteFunc: c.onDelete,
	}

	logger.Infof("start watching edgefs clusters in all namespaces")
	go k8sutil.WatchCR(ClusterResource, namespace, resourceHandlerFuncs, c.context.RookClientset.EdgefsV1().RESTClient(), &edgefsv1.Cluster{}, stopCh)

	return nil
}

func (c *ClusterController) StopWatch() {
	for _, cluster := range c.clusterMap {
		close(cluster.stopCh)
	}
	c.clusterMap = make(map[string]*cluster)
}

func (c *ClusterController) onAdd(obj interface{}) {
	clusterObj := obj.(*edgefsv1.Cluster).DeepCopy()
	logger.Infof("new cluster %s added to namespace %s", clusterObj.Name, clusterObj.Namespace)

	cluster := newCluster(clusterObj, c.context)
	c.clusterMap[cluster.Namespace] = cluster

	//Override rook containerImage value
	c.containerImage = defaultEdgefsImageName
	if cluster.Spec.EdgefsImageName != "" {
		c.containerImage = cluster.Spec.EdgefsImageName
	}

	logger.Infof("starting cluster in namespace %s", cluster.Namespace)
	if c.devicesInUse && cluster.Spec.Storage.AnyUseAllDevices() {
		c.updateClusterStatus(clusterObj.Namespace, clusterObj.Name, edgefsv1.ClusterStateError, "using all devices in more than one namespace is not supported")
		return
	}

	if cluster.Spec.Storage.AnyUseAllDevices() {
		c.devicesInUse = true
	}

	// Start the Rook cluster components. Retry several times in case of failure.
	err := wait.Poll(clusterCreateInterval, clusterCreateTimeout, func() (bool, error) {
		c.updateClusterStatus(clusterObj.Namespace, clusterObj.Name, edgefsv1.ClusterStateCreating, "")

		done, err := cluster.createInstance(c.containerImage, false)
		if err != nil {
			logger.Errorf("%s", err)
			return done, err
		}

		// cluster is created, update the cluster CRD status now
		c.updateClusterStatus(clusterObj.Namespace, clusterObj.Name, edgefsv1.ClusterStateCreated, "")

		return true, nil
	})
	if err != nil {
		c.updateClusterStatus(clusterObj.Namespace, clusterObj.Name, edgefsv1.ClusterStateError,
			fmt.Sprintf("giving up creating cluster in namespace %s after %s. Error: %s", cluster.Namespace, clusterCreateTimeout, err.Error()))
		return
	}

	logger.Infof("succeeded creating and initializing EdgeFS cluster in namespace %s", cluster.Namespace)

	// Start NFS service CRD watcher
	NFSController := nfs.NewNFSController(c.context,
		cluster.Namespace,
		c.containerImage,
		cluster.Spec.Network,
		cluster.Spec.DataDirHostPath, cluster.Spec.DataVolumeSize,
		edgefsv1.GetTargetPlacement(cluster.Spec.Placement),
		cluster.Spec.Resources,
		cluster.Spec.ResourceProfile,
		cluster.ownerRef,
		cluster.Spec.UseHostLocalTime)
	NFSController.StartWatch(cluster.stopCh)

	// Start S3 service CRD watcher
	S3Controller := s3.NewS3Controller(c.context,
		cluster.Namespace,
		c.containerImage,
		cluster.Spec.Network,
		cluster.Spec.DataDirHostPath, cluster.Spec.DataVolumeSize,
		edgefsv1.GetTargetPlacement(cluster.Spec.Placement),
		cluster.Spec.Resources,
		cluster.Spec.ResourceProfile,
		cluster.ownerRef,
		cluster.Spec.UseHostLocalTime)
	S3Controller.StartWatch(cluster.stopCh)

	// Start SWIFT service CRD watcher
	SWIFTController := swift.NewSWIFTController(c.context,
		cluster.Namespace,
		c.containerImage,
		cluster.Spec.Network,
		cluster.Spec.DataDirHostPath, cluster.Spec.DataVolumeSize,
		edgefsv1.GetTargetPlacement(cluster.Spec.Placement),
		cluster.Spec.Resources,
		cluster.Spec.ResourceProfile,
		cluster.ownerRef,
		cluster.Spec.UseHostLocalTime)
	SWIFTController.StartWatch(cluster.stopCh)

	// Start S3X service CRD watcher
	S3XController := s3x.NewS3XController(c.context,
		cluster.Namespace,
		c.containerImage,
		cluster.Spec.Network,
		cluster.Spec.DataDirHostPath, cluster.Spec.DataVolumeSize,
		edgefsv1.GetTargetPlacement(cluster.Spec.Placement),
		cluster.Spec.Resources,
		cluster.Spec.ResourceProfile,
		cluster.ownerRef,
		cluster.Spec.UseHostLocalTime)
	S3XController.StartWatch(cluster.stopCh)

	// Start ISCSI service CRD watcher
	ISCSIController := iscsi.NewISCSIController(c.context,
		cluster.Namespace,
		c.containerImage,
		cluster.Spec.Network,
		cluster.Spec.DataDirHostPath, cluster.Spec.DataVolumeSize,
		edgefsv1.GetTargetPlacement(cluster.Spec.Placement),
		cluster.Spec.Resources,
		cluster.Spec.ResourceProfile,
		cluster.ownerRef,
		cluster.Spec.UseHostLocalTime)
	ISCSIController.StartWatch(cluster.stopCh)

	// Start ISGW service CRD watcher
	ISGWController := isgw.NewISGWController(c.context,
		cluster.Namespace,
		c.containerImage,
		cluster.Spec.Network,
		cluster.Spec.DataDirHostPath, cluster.Spec.DataVolumeSize,
		edgefsv1.GetTargetPlacement(cluster.Spec.Placement),
		cluster.Spec.Resources,
		cluster.Spec.ResourceProfile,
		cluster.ownerRef,
		cluster.Spec.UseHostLocalTime)
	ISGWController.StartWatch(cluster.stopCh)

	cluster.childControllers = []childController{
		NFSController, S3Controller, S3XController, SWIFTController, ISCSIController, ISGWController,
	}

	// add the finalizer to the crd
	err = c.addFinalizer(clusterObj)
	if err != nil {
		logger.Errorf("failed to add finalizer to cluster crd. %+v", err)
	}
}

func (c *ClusterController) onUpdate(oldObj, newObj interface{}) {
	oldCluster := oldObj.(*edgefsv1.Cluster).DeepCopy()
	newCluster := newObj.(*edgefsv1.Cluster).DeepCopy()
	logger.Infof("update event for cluster %s", newCluster.Namespace)

	// Check if the cluster is being deleted. This code path is called when a finalizer is specified in the crd.
	// When a cluster is requested for deletion, K8s will only set the deletion timestamp if there are any finalizers in the list.
	// K8s will only delete the crd and child resources when the finalizers have been removed from the crd.
	if newCluster.DeletionTimestamp != nil {
		logger.Infof("cluster %s has a deletion timestamp", newCluster.Namespace)
		err := c.handleDelete(newCluster, time.Duration(clusterDeleteRetryInterval)*time.Second)
		if err != nil {
			logger.Errorf("failed finalizer for cluster. %+v", err)
			return
		}
		// remove the finalizer from the crd, which indicates to k8s that the resource can safely be deleted
		c.removeFinalizer(newCluster)
		return
	}

	if !clusterChanged(oldCluster.Spec, newCluster.Spec) {
		logger.Infof("update event for cluster %s is not supported", newCluster.Namespace)
		return
	}

	logger.Infof("update event for cluster %s is supported, orchestrating update now", newCluster.Namespace)
	logger.Debugf("old cluster: %+v", oldCluster.Spec)
	logger.Debugf("new cluster: %+v", newCluster.Spec)

	cluster, ok := c.clusterMap[newCluster.Namespace]
	if !ok {
		logger.Errorf("Cannot update cluster %s that does not exist", newCluster.Namespace)
		return
	}
	cluster.Spec = newCluster.Spec

	// attempt to update the cluster.  note this is done outside of wait.Poll because that function
	// will wait for the retry interval before trying for the first time.
	done, err := c.handleUpdate(newCluster, cluster)
	if done {
		if err != nil {
			c.updateClusterStatus(newCluster.Namespace, newCluster.Name, edgefsv1.ClusterStateError, err.Error())
		}
		return
	}

	err = wait.Poll(updateClusterInterval, updateClusterTimeout, func() (bool, error) {
		return c.handleUpdate(newCluster, cluster)
	})
	if err != nil {
		c.updateClusterStatus(newCluster.Namespace, newCluster.Name, edgefsv1.ClusterStateError,
			fmt.Sprintf("giving up trying to update cluster in namespace %s after %s", cluster.Namespace, updateClusterTimeout))
		return
	}
	logger.Infof("cluster %s updated in namespace %s", newCluster.Name, newCluster.Namespace)
}

func (c *ClusterController) handleUpdate(newClust *edgefsv1.Cluster, cluster *cluster) (bool, error) {
	c.updateClusterStatus(newClust.Namespace, newClust.Name, edgefsv1.ClusterStateUpdating, "")

	if newClust.Spec.EdgefsImageName != "" {
		c.containerImage = newClust.Spec.EdgefsImageName
	}

	done, err := cluster.createInstance(c.containerImage, true)
	if err != nil {
		logger.Errorf("failed to update cluster in namespace %s. %+v", newClust.Namespace, err)
		return done, err
	}

	c.updateClusterStatus(newClust.Namespace, newClust.Name, edgefsv1.ClusterStateCreated, "")

	logger.Infof("succeeded updating cluster in namespace %s", newClust.Namespace)
	return true, nil
}

func (c *ClusterController) onDelete(obj interface{}) {
	clust, ok := obj.(*edgefsv1.Cluster)
	if !ok {
		return
	}
	clust = clust.DeepCopy()
	logger.Infof("delete event for cluster %s in namespace %s", clust.Name, clust.Namespace)

	err := c.handleDelete(clust, time.Duration(clusterDeleteRetryInterval)*time.Second)
	if err != nil {
		logger.Errorf("failed to delete cluster. %+v", err)
	}
	if cluster, ok := c.clusterMap[clust.Namespace]; ok {
		close(cluster.stopCh)
		delete(c.clusterMap, clust.Namespace)
	}
	if clust.Spec.Storage.AnyUseAllDevices() {
		c.devicesInUse = false
	}
}

func (c *ClusterController) handleDelete(clust *edgefsv1.Cluster, retryInterval time.Duration) error {

	cluster, ok := c.clusterMap[clust.Namespace]
	if !ok {
		return fmt.Errorf("Cannot delete cluster %s that does not exist", clust.Namespace)
	}

	// grace on misconfigured crd deletions
	if cluster.targets == nil || cluster.targets.Storage.Nodes == nil {
		return nil
	}

	for _, node := range cluster.targets.Storage.Nodes {
		cluster.UnlabelTargetNode(node.Name)
	}

	// delete associated node labels
	return nil
}

func (c *ClusterController) updateClusterStatus(namespace, name string, state edgefsv1.ClusterState, message string) {
	// get the most recent cluster CRD object
	cluster, err := c.context.RookClientset.EdgefsV1().Clusters(namespace).Get(name, metav1.GetOptions{})
	if err != nil {
		logger.Errorf("failed to get cluster from namespace %s prior to updating its status: %+v", namespace, err)
		return
	}

	// update the status on the retrieved cluster object
	cluster.Status = edgefsv1.ClusterStatus{State: state, Message: message}
	if _, err := c.context.RookClientset.EdgefsV1().Clusters(cluster.Namespace).Update(cluster); err != nil {
		logger.Errorf("failed to update cluster %s status: %+v", cluster.Namespace, err)
	}
}

func (c *ClusterController) addFinalizer(clust *edgefsv1.Cluster) error {

	// get the latest cluster object since we probably updated it before we got to this point (e.g. by updating its status)
	clust, err := c.context.RookClientset.EdgefsV1().Clusters(clust.Namespace).Get(clust.Name, metav1.GetOptions{})
	if err != nil {
		return err
	}

	// add the finalizer (cluster.edgefs.rook.io) if it is not yet defined on the cluster CRD
	for _, finalizer := range clust.Finalizers {
		if finalizer == finalizerName {
			logger.Infof("finalizer already set on cluster %s", clust.Namespace)
			return nil
		}
	}

	// adding finalizer to the cluster crd
	clust.Finalizers = append(clust.Finalizers, finalizerName)

	// update the crd
	_, err = c.context.RookClientset.EdgefsV1().Clusters(clust.Namespace).Update(clust)
	if err != nil {
		return fmt.Errorf("failed to add finalizer to cluster. %+v", err)
	}

	logger.Infof("added finalizer to cluster %s", clust.Name)
	return nil
}

func (c *ClusterController) removeFinalizer(obj interface{}) {
	var fname string
	var objectMeta *metav1.ObjectMeta

	// first determine what type/version of cluster we are dealing with
	if cl, ok := obj.(*edgefsv1.Cluster); ok {
		fname = finalizerName
		objectMeta = &cl.ObjectMeta
	} else {
		logger.Warningf("cannot remove finalizer from object that is not a cluster: %+v", obj)
		return
	}

	// remove the finalizer from the slice if it exists
	found := false
	for i, finalizer := range objectMeta.Finalizers {
		if finalizer == fname {
			objectMeta.Finalizers = append(objectMeta.Finalizers[:i], objectMeta.Finalizers[i+1:]...)
			found = true
			break
		}
	}
	if !found {
		logger.Infof("finalizer %s not found in the cluster crd '%s'", fname, objectMeta.Name)
		return
	}

	// update the crd to remove the finalizer for good. retry several times in case of intermittent failures.
	maxRetries := 5
	retrySeconds := 5 * time.Second
	for i := 0; i < maxRetries; i++ {
		var (
			okCheck bool
			err     error
		)
		if cluster, ok := obj.(*edgefsv1.Cluster); ok {
			_, err = c.context.RookClientset.EdgefsV1().Clusters(cluster.Namespace).Update(cluster)
			okCheck = true
		}

		if okCheck != true || err != nil {
			logger.Errorf("failed to remove finalizer %s from cluster %s. %+v", fname, objectMeta.Name, err)
			time.Sleep(retrySeconds)
			continue
		}
		logger.Infof("removed finalizer %s from cluster %s", fname, objectMeta.Name)
		return
	}

	logger.Warningf("giving up from removing the %s cluster finalizer", fname)
}
