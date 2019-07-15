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
	"github.com/rook/rook/pkg/clusterd"
	cephconfig "github.com/rook/rook/pkg/daemon/ceph/config"
	apiextensionsv1beta1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1beta1"
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
	return nil
}

func (c *FilesystemController) onAdd(obj interface{}) {
	filesystem, err := getFilesystemObject(obj)
	if err != nil {
		logger.Errorf("failed to get filesystem object: %+v", err)
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
	oldFS, err := getFilesystemObject(oldObj)
	if err != nil {
		logger.Errorf("failed to get old filesystem object: %+v", err)
		return
	}
	newFS, err := getFilesystemObject(newObj)
	if err != nil {
		logger.Errorf("failed to get new filesystem object: %+v", err)
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
	filesystem, err := getFilesystemObject(obj)
	if err != nil {
		logger.Errorf("failed to get filesystem object: %+v", err)
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

func getFilesystemObject(obj interface{}) (filesystem *cephv1.CephFilesystem, err error) {
	var ok bool
	filesystem, ok = obj.(*cephv1.CephFilesystem)
	if ok {
		// the filesystem object is of the latest type, simply return it
		return filesystem.DeepCopy(), nil
	}

	return nil, fmt.Errorf("not a known filesystem object: %+v", obj)
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
