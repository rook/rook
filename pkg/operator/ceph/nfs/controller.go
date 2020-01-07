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
*/

package nfs

import (
	"fmt"
	"reflect"
	"sync"

	"github.com/coreos/pkg/capnslog"
	cephv1 "github.com/rook/rook/pkg/apis/ceph.rook.io/v1"
	"github.com/rook/rook/pkg/clusterd"
	cephconfig "github.com/rook/rook/pkg/daemon/ceph/config"
	"github.com/rook/rook/pkg/operator/k8sutil"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/tools/cache"
)

var logger = capnslog.NewPackageLogger("github.com/rook/rook", "op-nfs")

// CephNFSResource represents the file system custom resource
var CephNFSResource = k8sutil.CustomResource{
	Name:    "cephnfs",
	Plural:  "cephnfses",
	Group:   cephv1.CustomResourceGroup,
	Version: cephv1.Version,
	Kind:    reflect.TypeOf(cephv1.CephNFS{}).Name(),
}

// CephNFSController represents a controller for NFS custom resources
type CephNFSController struct {
	clusterInfo        *cephconfig.ClusterInfo
	context            *clusterd.Context
	dataDirHostPath    string
	namespace          string
	rookImage          string
	clusterSpec        *cephv1.ClusterSpec
	orchestrationMutex sync.Mutex
	isUpgrade          bool
}

// NewCephNFSController create controller for watching NFS custom resources created
func NewCephNFSController(clusterInfo *cephconfig.ClusterInfo, context *clusterd.Context, dataDirHostPath, namespace, rookImage string, clusterSpec *cephv1.ClusterSpec, ownerRef metav1.OwnerReference) *CephNFSController {
	return &CephNFSController{
		clusterInfo:     clusterInfo,
		context:         context,
		dataDirHostPath: dataDirHostPath,
		namespace:       namespace,
		rookImage:       rookImage,
		clusterSpec:     clusterSpec,
	}
}

// StartWatch watches for instances of CephNFS custom resources and acts on them
func (c *CephNFSController) StartWatch(namespace string, stopCh chan struct{}) error {

	resourceHandlerFuncs := cache.ResourceEventHandlerFuncs{
		AddFunc:    c.onAdd,
		UpdateFunc: c.onUpdate,
		DeleteFunc: c.onDelete,
	}

	logger.Infof("start watching ceph nfs resource in namespace %s", namespace)
	go k8sutil.WatchCR(CephNFSResource, namespace, resourceHandlerFuncs, c.context.RookClientset.CephV1().RESTClient(), &cephv1.CephNFS{}, stopCh)

	return nil
}

func (c *CephNFSController) onAdd(obj interface{}) {
	if c.clusterSpec.External.Enable && c.clusterSpec.CephVersion.Image == "" {
		logger.Warningf("Creating nfs for an external ceph cluster is disabled because no Ceph image is specified")
		return
	}

	nfs := obj.(*cephv1.CephNFS).DeepCopy()
	if !c.clusterInfo.CephVersion.IsAtLeastNautilus() {
		logger.Errorf("Ceph NFS is only supported with Nautilus or newer. CRD %q will be ignored.", nfs.Name)
		return
	}

	c.acquireOrchestrationLock()
	defer c.releaseOrchestrationLock()

	updateCephNFSStatus(nfs.GetName(), nfs.GetNamespace(), k8sutil.ProcessingStatus, c.context)

	err := c.upCephNFS(*nfs, 0)
	if err != nil {
		logger.Errorf("failed to create NFS Ganesha %q. %v", nfs.Name, err)
		updateCephNFSStatus(nfs.GetName(), nfs.GetNamespace(), k8sutil.FailedStatus, c.context)
	}
	updateCephNFSStatus(nfs.GetName(), nfs.GetNamespace(), k8sutil.ReadyStatus, c.context)
}

func (c *CephNFSController) onUpdate(oldObj, newObj interface{}) {
	if c.clusterSpec.External.Enable && c.clusterSpec.CephVersion.Image == "" {
		logger.Warningf("Updating nfs for an external ceph cluster is disabled because no Ceph image is specified")
		return
	}

	oldNFS := oldObj.(*cephv1.CephNFS).DeepCopy()
	newNFS := newObj.(*cephv1.CephNFS).DeepCopy()
	if !c.clusterInfo.CephVersion.IsAtLeastNautilus() {
		logger.Errorf("Ceph NFS is only supported with Nautilus or newer. CRD %q will be ignored.", newNFS.Name)
		return
	}

	if !nfsChanged(oldNFS.Spec, newNFS.Spec) {
		logger.Debugf("nfs ganesha %q not updated", newNFS.Name)
		return
	}

	c.acquireOrchestrationLock()
	defer c.releaseOrchestrationLock()

	logger.Infof("Updating the ganesha server from %d to %d active count", oldNFS.Spec.Server.Active, newNFS.Spec.Server.Active)

	updateCephNFSStatus(newNFS.GetName(), newNFS.GetNamespace(), k8sutil.ProcessingStatus, c.context)

	if oldNFS.Spec.Server.Active < newNFS.Spec.Server.Active {
		err := c.upCephNFS(*newNFS, oldNFS.Spec.Server.Active)
		if err != nil {
			logger.Errorf("Failed to start daemons for CephNFS %q. %v", newNFS.Name, err)
			updateCephNFSStatus(newNFS.GetName(), newNFS.GetNamespace(), k8sutil.FailedStatus, c.context)
		}
	} else {
		err := c.downCephNFS(*oldNFS, newNFS.Spec.Server.Active)
		if err != nil {
			logger.Errorf("Failed to stop daemons for CephNFS %q. %v", newNFS.Name, err)
			updateCephNFSStatus(newNFS.GetName(), newNFS.GetNamespace(), k8sutil.FailedStatus, c.context)
		}
	}
	updateCephNFSStatus(newNFS.GetName(), newNFS.GetNamespace(), k8sutil.ReadyStatus, c.context)
}

func (c *CephNFSController) onDelete(obj interface{}) {
	if c.clusterSpec.External.Enable && c.clusterSpec.CephVersion.Image == "" {
		logger.Warningf("Deleting nfs for an external ceph cluster is disabled because no Ceph image is specified")
		return
	}

	nfs := obj.(*cephv1.CephNFS).DeepCopy()
	if !c.clusterInfo.CephVersion.IsAtLeastNautilus() {
		logger.Errorf("Ceph NFS is only supported with Nautilus or newer. CRD %q cleanup will be ignored.", nfs.Name)
		return
	}

	c.acquireOrchestrationLock()
	defer c.releaseOrchestrationLock()

	err := c.downCephNFS(*nfs, 0)
	if err != nil {
		logger.Errorf("failed to delete file system %s. %v", nfs.Name, err)
	}
}

// ParentClusterChanged performs the steps needed to update the NFS cluster when the parent Ceph
// cluster has changed.
func (c *CephNFSController) ParentClusterChanged(cluster cephv1.ClusterSpec, clusterInfo *cephconfig.ClusterInfo, isUpgrade bool) {
	c.clusterInfo = clusterInfo
	if !isUpgrade || !c.clusterInfo.CephVersion.IsAtLeastNautilus() {
		logger.Debugf("No need to update the nfs daemons after the parent cluster changed")
		return
	}

	// This is mostly a placeholder since we don't perform any upgrade checks for nfs since it's not in Ceph's servicemap yet
	// This is an upgrade so let's activate the flag
	c.isUpgrade = isUpgrade

	c.acquireOrchestrationLock()
	defer c.releaseOrchestrationLock()

	c.clusterSpec.CephVersion = cluster.CephVersion
	nfses, err := c.context.RookClientset.CephV1().CephNFSes(c.namespace).List(metav1.ListOptions{})
	if err != nil {
		logger.Errorf("failed to retrieve NFSes to update the ceph version. %v", err)
		return
	}
	for _, nfs := range nfses.Items {
		logger.Infof("updating the ceph version for nfs %q to %q", nfs.Name, c.clusterSpec.CephVersion.Image)
		err := c.upCephNFS(nfs, 0)
		if err != nil {
			logger.Errorf("failed to update nfs %q. %v", nfs.Name, err)
		} else {
			logger.Infof("updated nfs %q to ceph version %q", nfs.Name, c.clusterSpec.CephVersion.Image)
		}
	}
}

func nfsChanged(oldNFS, newNFS cephv1.NFSGaneshaSpec) bool {
	if oldNFS.Server.Active != newNFS.Server.Active {
		return true
	}
	return false
}

func (c *CephNFSController) acquireOrchestrationLock() {
	logger.Debugf("Acquiring lock for nfs orchestration")
	c.orchestrationMutex.Lock()
	logger.Debugf("Acquired lock for nfs orchestration")
}

func (c *CephNFSController) releaseOrchestrationLock() {
	c.orchestrationMutex.Unlock()
	logger.Debugf("Released lock for nfs orchestration")
}

func ownerRefs(nfs cephv1.CephNFS) []metav1.OwnerReference {
	// Set the filesystem CR as the owner
	return []metav1.OwnerReference{{
		APIVersion: fmt.Sprintf("%s/%s", CephNFSResource.Group, CephNFSResource.Version),
		Kind:       CephNFSResource.Kind,
		Name:       nfs.Name,
		UID:        nfs.UID,
	}}
}

func updateCephNFSStatus(name, namespace, status string, context *clusterd.Context) {
	updatedCephNFS, err := context.RookClientset.CephV1().CephNFSes(namespace).Get(name, metav1.GetOptions{})
	if err != nil {
		logger.Errorf("Unable to update the cephNFS %s status %v", updatedCephNFS.GetName(), err)
		return
	}
	if updatedCephNFS.Status == nil {
		updatedCephNFS.Status = &cephv1.Status{}
	} else if updatedCephNFS.Status.Phase == status {
		return
	}
	updatedCephNFS.Status.Phase = status
	_, err = context.RookClientset.CephV1().CephNFSes(updatedCephNFS.Namespace).Update(updatedCephNFS)
	if err != nil {
		logger.Errorf("Unable to update the cephNFS %s status %v", updatedCephNFS.GetName(), err)
		return
	}
}
