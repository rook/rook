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

// Package file manages a CephFS filesystem and the required daemons.
package file

import (
	"fmt"
	"reflect"
	"sync"

	"github.com/coreos/pkg/capnslog"
	opkit "github.com/rook/operator-kit"
	cephv1 "github.com/rook/rook/pkg/apis/ceph.rook.io/v1"
	cephbeta "github.com/rook/rook/pkg/apis/ceph.rook.io/v1beta1"
	"github.com/rook/rook/pkg/clusterd"
	cephconfig "github.com/rook/rook/pkg/daemon/ceph/config"
	"github.com/rook/rook/pkg/operator/ceph/pool"
	apiextensionsv1beta1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1beta1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/tools/cache"
)

var logger = capnslog.NewPackageLogger("github.com/rook/rook", "op-file")

// FilesystemResource represents the filesystem custom resource
var FilesystemResource = opkit.CustomResource{
	Name:    "cephfilesystem",
	Plural:  "cephfilesystems",
	Group:   cephv1.CustomResourceGroup,
	Version: cephv1.Version,
	Scope:   apiextensionsv1beta1.NamespaceScoped,
	Kind:    reflect.TypeOf(cephv1.CephFilesystem{}).Name(),
}

var filesystemResourceRookLegacy = opkit.CustomResource{
	Name:    "filesystem",
	Plural:  "filesystems",
	Group:   cephbeta.CustomResourceGroup,
	Version: cephbeta.Version,
	Scope:   apiextensionsv1beta1.NamespaceScoped,
	Kind:    reflect.TypeOf(cephbeta.Filesystem{}).Name(),
}

// FilesystemController represents a controller for filesystem custom resources
type FilesystemController struct {
	clusterInfo        *cephconfig.ClusterInfo
	context            *clusterd.Context
	namespace          string
	rookVersion        string
	cephVersion        cephv1.CephVersionSpec
	hostNetwork        bool
	ownerRef           metav1.OwnerReference
	dataDirHostPath    string
	orchestrationMutex sync.Mutex
}

// NewFilesystemController create controller for watching filesystem custom resources created
func NewFilesystemController(
	clusterInfo *cephconfig.ClusterInfo,
	context *clusterd.Context,
	namespace string,
	rookVersion string,
	cephVersion cephv1.CephVersionSpec,
	hostNetwork bool,
	ownerRef metav1.OwnerReference,
	dataDirHostPath string,
) *FilesystemController {
	return &FilesystemController{
		clusterInfo:     clusterInfo,
		context:         context,
		namespace:       namespace,
		rookVersion:     rookVersion,
		cephVersion:     cephVersion,
		hostNetwork:     hostNetwork,
		ownerRef:        ownerRef,
		dataDirHostPath: dataDirHostPath,
	}
}

// StartWatch watches for instances of Filesystem custom resources and acts on them
func (c *FilesystemController) StartWatch(stopCh chan struct{}) error {

	resourceHandlerFuncs := cache.ResourceEventHandlerFuncs{
		AddFunc:    c.onAdd,
		UpdateFunc: c.onUpdate,
		DeleteFunc: c.onDelete,
	}

	logger.Infof("start watching filesystem resource in namespace %s", c.namespace)
	watcher := opkit.NewWatcher(FilesystemResource, c.namespace, resourceHandlerFuncs, c.context.RookClientset.CephV1().RESTClient())
	go watcher.Watch(&cephv1.CephFilesystem{}, stopCh)

	// watch for events on all legacy types too
	c.watchLegacyFilesystems(c.namespace, stopCh, resourceHandlerFuncs)

	return nil
}

func (c *FilesystemController) onAdd(obj interface{}) {
	filesystem, migrationNeeded, err := getFilesystemObject(obj)
	if err != nil {
		logger.Errorf("failed to get filesystem object: %+v", err)
		return
	}

	if migrationNeeded {
		if err = c.migrateFilesystemObject(filesystem, obj); err != nil {
			logger.Errorf("failed to migrate filesystem %s in namespace %s: %+v", filesystem.Name, filesystem.Namespace, err)
		}
		return
	}

	c.acquireOrchestrationLock()
	defer c.releaseOrchestrationLock()

	err = createFilesystem(c.clusterInfo, c.context, *filesystem, c.rookVersion, c.cephVersion, c.hostNetwork, c.filesystemOwners(filesystem), c.dataDirHostPath)
	if err != nil {
		logger.Errorf("failed to create filesystem %s: %+v", filesystem.Name, err)
	}
}

func (c *FilesystemController) onUpdate(oldObj, newObj interface{}) {
	oldFS, _, err := getFilesystemObject(oldObj)
	if err != nil {
		logger.Errorf("failed to get old filesystem object: %+v", err)
		return
	}
	newFS, migrationNeeded, err := getFilesystemObject(newObj)
	if err != nil {
		logger.Errorf("failed to get new filesystem object: %+v", err)
		return
	}

	if migrationNeeded {
		if err = c.migrateFilesystemObject(newFS, newObj); err != nil {
			logger.Errorf("failed to migrate filesystem %s in namespace %s: %+v", newFS.Name, newFS.Namespace, err)
		}
		return
	}

	if !filesystemChanged(oldFS.Spec, newFS.Spec) {
		logger.Debugf("filesystem %s not updated", newFS.Name)
		return
	}

	c.acquireOrchestrationLock()
	defer c.releaseOrchestrationLock()

	// if the filesystem is modified, allow the filesystem to be created if it wasn't already
	logger.Infof("updating filesystem %s", newFS.Name)
	err = createFilesystem(c.clusterInfo, c.context, *newFS, c.rookVersion, c.cephVersion, c.hostNetwork, c.filesystemOwners(newFS), c.dataDirHostPath)
	if err != nil {
		logger.Errorf("failed to create (modify) filesystem %s: %+v", newFS.Name, err)
	}
}

func (c *FilesystemController) ParentClusterChanged(cluster cephv1.ClusterSpec, clusterInfo *cephconfig.ClusterInfo) {
	c.clusterInfo = clusterInfo
	if cluster.CephVersion.Image == c.cephVersion.Image {
		logger.Debugf("No need to update the file system after the parent cluster changed")
		return
	}

	c.acquireOrchestrationLock()
	defer c.releaseOrchestrationLock()

	c.cephVersion = cluster.CephVersion
	filesystems, err := c.context.RookClientset.CephV1().CephFilesystems(c.namespace).List(metav1.ListOptions{})
	if err != nil {
		logger.Errorf("failed to retrieve filesystems to update the ceph version. %+v", err)
		return
	}
	for _, fs := range filesystems.Items {
		logger.Infof("updating the ceph version for filesystem %s to %s", fs.Name, c.cephVersion.Image)
		err = createFilesystem(c.clusterInfo, c.context, fs, c.rookVersion, c.cephVersion, c.hostNetwork, c.filesystemOwners(&fs), c.dataDirHostPath)
		if err != nil {
			logger.Errorf("failed to update filesystem %s. %+v", fs.Name, err)
		} else {
			logger.Infof("updated filesystem %s to ceph version %s", fs.Name, c.cephVersion.Image)
		}
	}
}

func (c *FilesystemController) onDelete(obj interface{}) {
	filesystem, migrationNeeded, err := getFilesystemObject(obj)
	if err != nil {
		logger.Errorf("failed to get filesystem object: %+v", err)
		return
	}

	if migrationNeeded {
		logger.Infof("ignoring deletion of legacy filesystem %s in namespace %s", filesystem.Name, filesystem.Namespace)
		return
	}

	c.acquireOrchestrationLock()
	defer c.releaseOrchestrationLock()

	err = deleteFilesystem(c.context, c.clusterInfo.CephVersion, *filesystem)
	if err != nil {
		logger.Errorf("failed to delete filesystem %s: %+v", filesystem.Name, err)
	}
}

func (c *FilesystemController) filesystemOwners(fs *cephv1.CephFilesystem) []metav1.OwnerReference {
	// Only set the cluster crd as the owner of the filesystem resources.
	// If the filesystem crd is deleted, the operator will explicitly remove the filesystem resources.
	// If the filesystem crd still exists when the cluster crd is deleted, this will make sure the filesystem
	// resources are cleaned up.
	return []metav1.OwnerReference{c.ownerRef}
}

func filesystemChanged(oldFS, newFS cephv1.FilesystemSpec) bool {
	if len(oldFS.DataPools) != len(newFS.DataPools) {
		logger.Infof("number of data pools changed from %d to %d", len(oldFS.DataPools), len(newFS.DataPools))
		return true
	}
	if oldFS.MetadataServer.ActiveCount != newFS.MetadataServer.ActiveCount {
		logger.Infof("number of mds active changed from %d to %d", oldFS.MetadataServer.ActiveCount, newFS.MetadataServer.ActiveCount)
		return true
	}
	if oldFS.MetadataServer.ActiveStandby != newFS.MetadataServer.ActiveStandby {
		logger.Infof("mds active standby changed from %t to %t", oldFS.MetadataServer.ActiveStandby, newFS.MetadataServer.ActiveStandby)
		return true
	}
	return false
}

func (c *FilesystemController) watchLegacyFilesystems(namespace string, stopCh chan struct{}, resourceHandlerFuncs cache.ResourceEventHandlerFuncs) {
	// watch for filesystem.rook.io/v1alpha1 events if the CRD exists
	if _, err := c.context.RookClientset.CephV1beta1().Filesystems(namespace).List(metav1.ListOptions{}); err != nil {
		logger.Infof("skipping watching for legacy rook filesystem events (legacy filesystem CRD probably doesn't exist): %+v", err)
	} else {
		logger.Infof("start watching legacy rook filesystems in all namespaces")
		watcherLegacy := opkit.NewWatcher(filesystemResourceRookLegacy, namespace, resourceHandlerFuncs, c.context.RookClientset.CephV1beta1().RESTClient())
		go watcherLegacy.Watch(&cephbeta.Filesystem{}, stopCh)
	}
}

func getFilesystemObject(obj interface{}) (filesystem *cephv1.CephFilesystem, migrationNeeded bool, err error) {
	var ok bool
	filesystem, ok = obj.(*cephv1.CephFilesystem)
	if ok {
		// the filesystem object is of the latest type, simply return it
		return filesystem.DeepCopy(), false, nil
	}

	// type assertion to current filesystem type failed, try instead asserting to the legacy filesystem types
	// then convert it to the current filesystem type
	filesystemRookLegacy, ok := obj.(*cephbeta.Filesystem)
	if ok {
		return convertRookLegacyFilesystem(filesystemRookLegacy.DeepCopy()), true, nil
	}

	return nil, false, fmt.Errorf("not a known filesystem object: %+v", obj)
}

func (c *FilesystemController) migrateFilesystemObject(filesystemToMigrate *cephv1.CephFilesystem, legacyObj interface{}) error {
	logger.Infof("migrating legacy filesystem %s in namespace %s", filesystemToMigrate.Name, filesystemToMigrate.Namespace)

	_, err := c.context.RookClientset.CephV1().CephFilesystems(filesystemToMigrate.Namespace).Get(filesystemToMigrate.Name, metav1.GetOptions{})
	if err == nil {
		// filesystem of current type with same name/namespace already exists, don't overwrite it
		logger.Warningf("filesystem object %s in namespace %s already exists, will not overwrite with migrated legacy filesystem.",
			filesystemToMigrate.Name, filesystemToMigrate.Namespace)
	} else {
		if !errors.IsNotFound(err) {
			return err
		}

		// filesystem of current type does not already exist, create it now to complete the migration
		_, err = c.context.RookClientset.CephV1().CephFilesystems(filesystemToMigrate.Namespace).Create(filesystemToMigrate)
		if err != nil {
			return err
		}

		logger.Infof("completed migration of legacy filesystem %s in namespace %s", filesystemToMigrate.Name, filesystemToMigrate.Namespace)
	}

	// delete the legacy filesystem instance, it should not be used anymore now that a migrated instance of the current type exists
	deletePropagation := metav1.DeletePropagationOrphan
	if _, ok := legacyObj.(*cephbeta.Filesystem); ok {
		logger.Infof("deleting legacy rook filesystem %s in namespace %s", filesystemToMigrate.Name, filesystemToMigrate.Namespace)
		return c.context.RookClientset.CephV1beta1().Filesystems(filesystemToMigrate.Namespace).Delete(
			filesystemToMigrate.Name, &metav1.DeleteOptions{PropagationPolicy: &deletePropagation})
	}

	return fmt.Errorf("not a known filesystem object: %+v", legacyObj)
}

func convertRookLegacyFilesystem(legacyFilesystem *cephbeta.Filesystem) *cephv1.CephFilesystem {
	if legacyFilesystem == nil {
		return nil
	}

	legacySpec := legacyFilesystem.Spec

	dataPools := make([]cephv1.PoolSpec, len(legacySpec.DataPools))
	for i, dp := range legacySpec.DataPools {
		dataPools[i] = pool.ConvertRookLegacyPoolSpec(dp)
	}

	filesystem := &cephv1.CephFilesystem{
		ObjectMeta: metav1.ObjectMeta{
			Name:      legacyFilesystem.Name,
			Namespace: legacyFilesystem.Namespace,
		},
		Spec: cephv1.FilesystemSpec{
			MetadataPool: pool.ConvertRookLegacyPoolSpec(legacySpec.MetadataPool),
			DataPools:    dataPools,
			MetadataServer: cephv1.MetadataServerSpec{
				ActiveCount:   legacySpec.MetadataServer.ActiveCount,
				ActiveStandby: legacySpec.MetadataServer.ActiveStandby,
				Placement:     legacySpec.MetadataServer.Placement,
				Resources:     legacySpec.MetadataServer.Resources,
			},
		},
	}

	return filesystem
}

func (c *FilesystemController) acquireOrchestrationLock() {
	logger.Debugf("Acquiring lock for filesystem orchestration")
	c.orchestrationMutex.Lock()
	logger.Debugf("Acquired lock for filesystem orchestration")
}

func (c *FilesystemController) releaseOrchestrationLock() {
	c.orchestrationMutex.Unlock()
	logger.Debugf("Released lock for filesystem orchestration")
}
