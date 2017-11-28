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

Some of the code below came from https://github.com/coreos/etcd-operator
which also has the apache 2.0 license.
*/

// Package cluster to manage a rook cluster.
package cluster

import (
	"fmt"
	"os"
	"time"

	"github.com/coreos/pkg/capnslog"
	opkit "github.com/rook/operator-kit"
	flexcrd "github.com/rook/rook/pkg/agent/flexvolume/crd"
	"github.com/rook/rook/pkg/ceph/client"
	"github.com/rook/rook/pkg/clusterd"
	"github.com/rook/rook/pkg/operator/api"
	"github.com/rook/rook/pkg/operator/k8sutil"
	"github.com/rook/rook/pkg/operator/mds"
	"github.com/rook/rook/pkg/operator/mgr"
	"github.com/rook/rook/pkg/operator/mon"
	"github.com/rook/rook/pkg/operator/osd"
	"github.com/rook/rook/pkg/operator/pool"
	"github.com/rook/rook/pkg/operator/rgw"
	"k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/tools/cache"
)

const (
	CustomResourceName       = "cluster"
	CustomResourceNamePlural = "clusters"
	crushConfigMapName       = "crush-config"
	crushmapCreatedKey       = "initialCrushMapCreated"
	clusterCreateInterval    = 6 * time.Second
	clusterCreateTimeout     = 5 * time.Minute
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
	logger = capnslog.NewPackageLogger("github.com/rook/rook", "op-cluster")
)

// ClusterController controls an instance of a Rook cluster
type ClusterController struct {
	context                    *clusterd.Context
	scheme                     *runtime.Scheme
	volumeAttachmentController flexcrd.VolumeAttachmentController
	devicesInUse               bool
}

// NewClusterController create controller for watching cluster custom resources created
func NewClusterController(context *clusterd.Context, volumeAttachmentController flexcrd.VolumeAttachmentController) *ClusterController {
	return &ClusterController{
		context:                    context,
		volumeAttachmentController: volumeAttachmentController,
	}
}

// Watch watches instances of cluster resources
func (c *ClusterController) StartWatch(namespace string, stopCh chan struct{}) error {
	customResourceClient, scheme, err := opkit.NewHTTPClient(k8sutil.CustomResourceGroup, k8sutil.V1Alpha1, SchemeBuilder)
	if err != nil {
		return fmt.Errorf("failed to get a k8s client for watching cluster resources: %v", err)
	}
	c.scheme = scheme

	resourceHandlerFuncs := cache.ResourceEventHandlerFuncs{
		AddFunc:    c.onAdd,
		UpdateFunc: c.onUpdate,
		DeleteFunc: c.onDelete,
	}

	logger.Infof("start watching clusters in all namespaces")
	watcher := opkit.NewWatcher(ClusterResource, namespace, resourceHandlerFuncs, customResourceClient)
	go watcher.Watch(&Cluster{}, stopCh)
	return nil
}

func (c *ClusterController) onAdd(obj interface{}) {
	clusterOrig := obj.(*Cluster)

	// NEVER modify objects from the store. It's a read-only, local cache.
	// Use scheme.Copy() to make a deep copy of original object.
	copyObj, err := c.scheme.Copy(clusterOrig)
	if err != nil {
		logger.Errorf("failed to create a deep copy of cluster object: %v\n", err)
		return
	}
	cluster := copyObj.(*Cluster)

	cluster.init(c.context)
	if c.devicesInUse && cluster.Spec.Storage.AnyUseAllDevices() {
		logger.Errorf("using all devices in more than one namespace not supported")
		return
	}

	if cluster.Spec.Storage.AnyUseAllDevices() {
		c.devicesInUse = true
	}

	if cluster.Spec.MonCount <= 0 {
		logger.Warningf("mon count is 0 or less (given: %d), needs to be greater 0", cluster.Spec.MonCount)
		cluster.Spec.MonCount = defaultMonCount
	}
	if cluster.Spec.MonCount > maxMonCount {
		logger.Warningf("mon count is bigger than %d (given: %d), this is NOT recommended", maxMonCount, cluster.Spec.MonCount)
		cluster.Spec.MonCount = maxMonCount
	}
	if cluster.Spec.MonCount%2 == 0 {
		logger.Warningf("mon count is even (given: %d), should be uneven", cluster.Spec.MonCount)
	}

	logger.Infof("starting cluster %s in namespace %s", cluster.Name, cluster.Namespace)
	// Start the Rook cluster components. Retry several times in case of failure.
	err = wait.Poll(clusterCreateInterval, clusterCreateTimeout, func() (bool, error) {
		err = cluster.createInstance()
		if err != nil {
			logger.Errorf("failed to create cluster %s in namespace %s. %+v", cluster.Name, cluster.Namespace, err)
			return false, nil
		}
		return true, nil
	})
	if err != nil {
		logger.Errorf("giving up to create cluster %s in namespace %s after %s", cluster.Name, cluster.Namespace, clusterCreateTimeout)
		return
	}

	// Start pool CRD watcher
	poolController := pool.NewPoolController(c.context)
	poolController.StartWatch(cluster.Namespace, cluster.stopCh)

	// Start object store CRD watcher
	objectStoreController := rgw.NewObjectStoreController(c.context, cluster.Spec.VersionTag, cluster.Spec.HostNetwork)
	objectStoreController.StartWatch(cluster.Namespace, cluster.stopCh)

	// Start file system CRD watcher
	fileController := mds.NewFilesystemController(c.context, cluster.Spec.VersionTag, cluster.Spec.HostNetwork)
	fileController.StartWatch(cluster.Namespace, cluster.stopCh)

	// Start mon health checker
	healthChecker := mon.NewHealthChecker(cluster.mons)
	go healthChecker.Check(cluster.stopCh)
}

func (c *ClusterController) onUpdate(oldObj, newObj interface{}) {
	oldCluster := oldObj.(*Cluster)
	newCluster := newObj.(*Cluster)
	if !clusterChanged(oldCluster.Spec, newCluster.Spec) {
		logger.Debugf("no updates made in the cluster")
		return
	}

	logger.Infof("updating cluster %s", newCluster.Namespace)
	if err := newCluster.createInstance(); err != nil {
		logger.Errorf("failed to update cluster %s in namespace %s. %+v", newCluster.Name, newCluster.Namespace, err)
	}
}

func (c *ClusterController) onDelete(obj interface{}) {
	clusterOrig := obj.(*Cluster)

	// NEVER modify objects from the store. It's a read-only, local cache.
	// Use scheme.Copy() to make a deep copy of original object.
	copyObj, err := c.scheme.Copy(clusterOrig)
	if err != nil {
		logger.Errorf("failed to create a deep copy of cluster object: %v\n", err)
		return
	}
	cluster := copyObj.(*Cluster)

	c.handleDelete(cluster, time.Duration(clusterDeleteRetryInterval)*time.Second)
}

func (c *ClusterController) handleDelete(cluster *Cluster, retryInterval time.Duration) {
	logger.Infof("cluster in namespace %s is being deleted.  Waiting for agents to perform cleanup operations.", cluster.Namespace)

	operatorNamespace := os.Getenv(k8sutil.PodNamespaceEnvVar)
	retryCount := 0
	for {
		// TODO: filter this List operation by cluster namespace on the server side
		vols, err := c.volumeAttachmentController.List(operatorNamespace)
		if err != nil {
			logger.Errorf("failed to get volume attachments for operator namespace %s: %+v", operatorNamespace, err)
			break
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
			logger.Infof("all volume attachments for cluster %s have been cleaned up.", cluster.Namespace)
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
}

func (c *Cluster) init(context *clusterd.Context) {
	c.context = context
}

func (c *Cluster) createInstance() error {

	// Create a configmap for overriding ceph config settings
	// These settings should only be modified by a user after they are initialized
	placeholderConfig := map[string]string{
		k8sutil.ConfigOverrideVal: "",
	}
	cm := &v1.ConfigMap{Data: placeholderConfig}
	cm.Name = k8sutil.ConfigOverrideName
	_, err := c.context.Clientset.CoreV1().ConfigMaps(c.Namespace).Create(cm)
	if err != nil && !errors.IsAlreadyExists(err) {
		return fmt.Errorf("failed to create override configmap %s. %+v", c.Namespace, err)
	}

	// Start the mon pods
	c.mons = mon.New(c.context, c.Namespace, c.Spec.DataDirHostPath, c.Spec.VersionTag, c.Spec.MonCount, c.Spec.Placement.GetMON(), c.Spec.HostNetwork, c.Spec.Resources.MON)
	err = c.mons.Start()
	if err != nil {
		return fmt.Errorf("failed to start the mons. %+v", err)
	}

	err = c.createInitialCrushMap()
	if err != nil {
		return fmt.Errorf("failed to create initial crushmap: %+v", err)
	}

	c.mgrs = mgr.New(c.context, c.Namespace, c.Spec.VersionTag, c.Spec.Placement.GetMGR(), c.Spec.HostNetwork, c.Spec.Resources.MGR)
	err = c.mgrs.Start()
	if err != nil {
		return fmt.Errorf("failed to start the ceph mgr. %+v", err)
	}

	c.apis = api.New(c.context, c.Namespace, c.Spec.VersionTag, c.Spec.Placement.GetAPI(), c.Spec.HostNetwork, c.Spec.Resources.API)
	err = c.apis.Start()
	if err != nil {
		return fmt.Errorf("failed to start the REST api. %+v", err)
	}

	// Start the OSDs
	c.osds = osd.New(c.context, c.Namespace, c.Spec.VersionTag, c.Spec.Storage, c.Spec.DataDirHostPath, c.Spec.Placement.GetOSD(), c.Spec.HostNetwork, c.Spec.Resources.OSD)
	err = c.osds.Start()
	if err != nil {
		return fmt.Errorf("failed to start the osds. %+v", err)
	}

	logger.Infof("Done creating rook instance in namespace %s", c.Namespace)
	return nil
}

func (c *Cluster) createInitialCrushMap() error {
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
			Name:      crushConfigMapName,
			Namespace: c.Namespace,
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

func clusterChanged(oldCluster, newCluster ClusterSpec) bool {
	// no updates to the cluster supported yet
	return false
}
