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
	"github.com/pkg/errors"
	cephv1 "github.com/rook/rook/pkg/apis/ceph.rook.io/v1"
	"github.com/rook/rook/pkg/clusterd"
	cephconfig "github.com/rook/rook/pkg/daemon/ceph/config"
	cephspec "github.com/rook/rook/pkg/operator/ceph/spec"
	"github.com/rook/rook/pkg/operator/k8sutil"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/tools/cache"
)

var logger = capnslog.NewPackageLogger("github.com/rook/rook", "op-file")

// FilesystemResource represents the filesystem custom resource
var FilesystemResource = k8sutil.CustomResource{
	Name:    "cephfilesystem",
	Plural:  "cephfilesystems",
	Group:   cephv1.CustomResourceGroup,
	Version: cephv1.Version,
	Kind:    reflect.TypeOf(cephv1.CephFilesystem{}).Name(),
}

// FilesystemController represents a controller for filesystem custom resources
type FilesystemController struct {
	clusterInfo        *cephconfig.ClusterInfo
	context            *clusterd.Context
	namespace          string
	rookVersion        string
	clusterSpec        *cephv1.ClusterSpec
	ownerRef           metav1.OwnerReference
	dataDirHostPath    string
	orchestrationMutex sync.Mutex
	isUpgrade          bool
}

// NewFilesystemController create controller for watching filesystem custom resources created
func NewFilesystemController(
	clusterInfo *cephconfig.ClusterInfo,
	context *clusterd.Context,
	namespace string,
	rookVersion string,
	clusterSpec *cephv1.ClusterSpec,
	ownerRef metav1.OwnerReference,
	dataDirHostPath string,
	isUpgrade bool,
) *FilesystemController {
	return &FilesystemController{
		clusterInfo:     clusterInfo,
		context:         context,
		namespace:       namespace,
		rookVersion:     rookVersion,
		clusterSpec:     clusterSpec,
		ownerRef:        ownerRef,
		dataDirHostPath: dataDirHostPath,
		isUpgrade:       isUpgrade,
	}
}

// StartWatch watches for instances of Filesystem custom resources and acts on them
func (c *FilesystemController) StartWatch(namespace string, stopCh chan struct{}) error {

	resourceHandlerFuncs := cache.ResourceEventHandlerFuncs{
		AddFunc:    c.onAdd,
		UpdateFunc: c.onUpdate,
		DeleteFunc: c.onDelete,
	}

	logger.Infof("start watching filesystem resource in namespace %s", c.namespace)
	go k8sutil.WatchCR(FilesystemResource, namespace, resourceHandlerFuncs, c.context.RookClientset.CephV1().RESTClient(), &cephv1.CephFilesystem{}, stopCh)
	return nil
}

func (c *FilesystemController) onAdd(obj interface{}) {

	if c.clusterSpec.External.Enable && c.clusterSpec.CephVersion.Image == "" {
		logger.Warningf("Creating filesystems for an external ceph cluster is disabled because no Ceph image is specified")
		return
	}

	filesystem, err := getFilesystemObject(obj)
	if err != nil {
		logger.Errorf("failed to get filesystem object. %v", err)
		return
	}

	c.acquireOrchestrationLock()
	defer c.releaseOrchestrationLock()

	if c.clusterSpec.External.Enable {
		_, err := cephspec.ValidateCephVersionsBetweenLocalAndExternalClusters(c.context, c.namespace, c.clusterInfo.CephVersion)
		if err != nil {
			// This handles the case where the operator is running, the external cluster has been upgraded and a CR creation is called
			// If that's a major version upgrade we fail, if it's a minor version, we continue, it's not ideal but not critical
			logger.Errorf("refusing to run new crd. %v", err)
			return
		}
	}
	updateCephFilesystemStatus(filesystem.GetName(), filesystem.GetNamespace(), k8sutil.ProcessingStatus, c.context)

	err = createFilesystem(c.clusterInfo, c.context, *filesystem, c.rookVersion, c.clusterSpec, c.filesystemOwner(filesystem), c.clusterSpec.DataDirHostPath, c.isUpgrade)
	if err != nil {
		logger.Errorf("failed to create filesystem %q. %v", filesystem.Name, err)
		updateCephFilesystemStatus(filesystem.GetName(), filesystem.GetNamespace(), k8sutil.FailedStatus, c.context)
	}
	updateCephFilesystemStatus(filesystem.GetName(), filesystem.GetNamespace(), k8sutil.ReadyStatus, c.context)
}

func (c *FilesystemController) onUpdate(oldObj, newObj interface{}) {
	if c.clusterSpec.External.Enable && c.clusterSpec.CephVersion.Image == "" {
		logger.Warningf("Updating filesystems for an external ceph cluster is disabled because no Ceph image is specified")
		return
	}

	oldFS, err := getFilesystemObject(oldObj)
	if err != nil {
		logger.Errorf("failed to get old filesystem object. %v", err)
		return
	}
	newFS, err := getFilesystemObject(newObj)
	if err != nil {
		logger.Errorf("failed to get new filesystem object. %v", err)
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
	updateCephFilesystemStatus(newFS.GetName(), newFS.GetNamespace(), k8sutil.ProcessingStatus, c.context)
	err = createFilesystem(c.clusterInfo, c.context, *newFS, c.rookVersion, c.clusterSpec, c.filesystemOwner(newFS), c.clusterSpec.DataDirHostPath, c.isUpgrade)
	if err != nil {
		logger.Errorf("failed to create (modify) filesystem %q. %v", newFS.Name, err)
		updateCephFilesystemStatus(newFS.GetName(), newFS.GetNamespace(), k8sutil.FailedStatus, c.context)
	}
	updateCephFilesystemStatus(newFS.GetName(), newFS.GetNamespace(), k8sutil.ReadyStatus, c.context)
}

// ParentClusterChanged determines wether or not a CR update has been sent
func (c *FilesystemController) ParentClusterChanged(cluster cephv1.ClusterSpec, clusterInfo *cephconfig.ClusterInfo, isUpgrade bool) {
	c.clusterInfo = clusterInfo
	if !isUpgrade {
		logger.Debugf("No need to update the file system after the parent cluster changed")
		return
	}

	// This is an upgrade so let's activate the flag
	c.isUpgrade = isUpgrade

	c.acquireOrchestrationLock()
	defer c.releaseOrchestrationLock()

	c.clusterSpec.CephVersion = cluster.CephVersion
	filesystems, err := c.context.RookClientset.CephV1().CephFilesystems(c.namespace).List(metav1.ListOptions{})
	if err != nil {
		logger.Errorf("failed to retrieve filesystems to update the ceph version. %v", err)
		return
	}
	for _, fs := range filesystems.Items {
		logger.Infof("updating the ceph version for filesystem %s to %s", fs.Name, c.clusterSpec.CephVersion.Image)
		err = createFilesystem(c.clusterInfo, c.context, fs, c.rookVersion, c.clusterSpec, c.filesystemOwner(&fs), c.clusterSpec.DataDirHostPath, c.isUpgrade)
		if err != nil {
			logger.Errorf("failed to update filesystem %q. %v", fs.Name, err)
		} else {
			logger.Infof("updated filesystem %q to ceph version %q", fs.Name, c.clusterSpec.CephVersion.Image)
		}
	}
}

func (c *FilesystemController) onDelete(obj interface{}) {
	if c.clusterSpec.External.Enable && c.clusterSpec.CephVersion.Image == "" {
		logger.Warningf("Deleting filesystems for an external ceph cluster is disabled because no Ceph image is specified")
		return
	}

	filesystem, err := getFilesystemObject(obj)
	if err != nil {
		logger.Errorf("failed to get filesystem object. %v", err)
		return
	}

	c.acquireOrchestrationLock()
	defer c.releaseOrchestrationLock()

	err = deleteFilesystem(c.context, c.clusterInfo.CephVersion, *filesystem)
	if err != nil {
		logger.Errorf("failed to delete filesystem %q. %v", filesystem.Name, err)
	}
}

func (c *FilesystemController) filesystemOwner(fs *cephv1.CephFilesystem) metav1.OwnerReference {
	// Set the filesystem CR as the owner
	return metav1.OwnerReference{
		APIVersion: fmt.Sprintf("%s/%s", FilesystemResource.Group, FilesystemResource.Version),
		Kind:       FilesystemResource.Kind,
		Name:       fs.Name,
		UID:        fs.UID,
	}
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
	if oldFS.PreservePoolsOnDelete != newFS.PreservePoolsOnDelete {
		logger.Infof("value of Preserve pools setting changed from %t to %t", oldFS.PreservePoolsOnDelete, newFS.PreservePoolsOnDelete)
		// This setting only will be used when the filesystem will be deleted
		return false
	}
	if oldFS.MetadataServer.PriorityClassName != newFS.MetadataServer.PriorityClassName {
		logger.Infof("mds priority class name changed from %s to %s", oldFS.MetadataServer.PriorityClassName, newFS.MetadataServer.PriorityClassName)
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

	return nil, errors.Errorf("not a known filesystem object: %+v", obj)
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

func updateCephFilesystemStatus(name, namespace, status string, context *clusterd.Context) {
	updatedCephFilesystem, err := context.RookClientset.CephV1().CephFilesystems(namespace).Get(name, metav1.GetOptions{})
	if err != nil {
		logger.Errorf("Unable to update the cephObjectStore %s status %v", updatedCephFilesystem.GetName(), err)
		return
	}
	if updatedCephFilesystem.Status == nil {
		updatedCephFilesystem.Status = &cephv1.Status{}
	} else if updatedCephFilesystem.Status.Phase == status {
		return
	}
	updatedCephFilesystem.Status.Phase = status
	_, err = context.RookClientset.CephV1().CephFilesystems(updatedCephFilesystem.Namespace).Update(updatedCephFilesystem)
	if err != nil {
		logger.Errorf("Unable to update the cephObjectStore %s status %v", updatedCephFilesystem.GetName(), err)
		return
	}
}
