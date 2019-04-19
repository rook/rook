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

// Package nfs to manage a NFS Ganesha server
package nfs

import (
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

var logger = capnslog.NewPackageLogger("github.com/rook/rook", "op-nfs")

// CephNFSResource represents the file system custom resource
var CephNFSResource = opkit.CustomResource{
	Name:    "cephnfs",
	Plural:  "cephnfses",
	Group:   cephv1.CustomResourceGroup,
	Version: cephv1.Version,
	Scope:   apiextensionsv1beta1.NamespaceScoped,
	Kind:    reflect.TypeOf(cephv1.CephNFS{}).Name(),
}

// NFSCephNFSController represents a controller for NFS custom resources
type CephNFSController struct {
	clusterInfo        *cephconfig.ClusterInfo
	context            *clusterd.Context
	namespace          string
	rookImage          string
	cephVersion        cephv1.CephVersionSpec
	hostNetwork        bool
	ownerRef           metav1.OwnerReference
	orchestrationMutex sync.Mutex
}

// NewNFSCephNFSController create controller for watching NFS custom resources created
func NewCephNFSController(clusterInfo *cephconfig.ClusterInfo, context *clusterd.Context, namespace, rookImage string, cephVersion cephv1.CephVersionSpec, hostNetwork bool, ownerRef metav1.OwnerReference) *CephNFSController {
	return &CephNFSController{
		clusterInfo: clusterInfo,
		context:     context,
		namespace:   namespace,
		rookImage:   rookImage,
		cephVersion: cephVersion,
		hostNetwork: hostNetwork,
		ownerRef:    ownerRef,
	}
}

// StartWatch watches for instances of CephNFS custom resources and acts on them
func (c *CephNFSController) StartWatch(stopCh chan struct{}) error {

	resourceHandlerFuncs := cache.ResourceEventHandlerFuncs{
		AddFunc:    c.onAdd,
		UpdateFunc: c.onUpdate,
		DeleteFunc: c.onDelete,
	}

	logger.Infof("start watching ceph nfs resource in namespace %s", c.namespace)
	watcher := opkit.NewWatcher(CephNFSResource, c.namespace, resourceHandlerFuncs, c.context.RookClientset.CephV1().RESTClient())
	go watcher.Watch(&cephv1.CephNFS{}, stopCh)

	return nil
}

func (c *CephNFSController) onAdd(obj interface{}) {
	nfs := obj.(*cephv1.CephNFS).DeepCopy()
	if !c.clusterInfo.CephVersion.IsAtLeastNautilus() {
		logger.Errorf("Ceph NFS is only supported with Nautilus or newer. CRD %s will be ignored.", nfs.Name)
		return
	}

	c.acquireOrchestrationLock()
	defer c.releaseOrchestrationLock()

	err := c.upCephNFS(*nfs, 0)
	if err != nil {
		logger.Errorf("failed to create NFS Ganesha %s. %+v", nfs.Name, err)
	}
}

func (c *CephNFSController) onUpdate(oldObj, newObj interface{}) {
	oldNFS := oldObj.(*cephv1.CephNFS).DeepCopy()
	newNFS := newObj.(*cephv1.CephNFS).DeepCopy()
	if !c.clusterInfo.CephVersion.IsAtLeastNautilus() {
		logger.Errorf("Ceph NFS is only supported with Nautilus or newer. CRD %s will be ignored.", newNFS.Name)
		return
	}

	if !nfsChanged(oldNFS.Spec, newNFS.Spec) {
		logger.Debugf("nfs ganesha %s not updated", newNFS.Name)
		return
	}

	c.acquireOrchestrationLock()
	defer c.releaseOrchestrationLock()

	logger.Infof("Updating the ganesha server from %d to %d active count", oldNFS.Spec.Server.Active, newNFS.Spec.Server.Active)
	if oldNFS.Spec.Server.Active < newNFS.Spec.Server.Active {
		err := c.upCephNFS(*newNFS, oldNFS.Spec.Server.Active)
		if err != nil {
			logger.Errorf("Failed to start daemons for CephNFS %s. %+v", newNFS.Name, err)
		}
	} else {
		err := c.downCephNFS(*oldNFS, newNFS.Spec.Server.Active)
		if err != nil {
			logger.Errorf("Failed to stop daemons for CephNFS %s. %+v", newNFS.Name, err)
		}
	}

}

func (c *CephNFSController) onDelete(obj interface{}) {
	nfs := obj.(*cephv1.CephNFS).DeepCopy()
	if !c.clusterInfo.CephVersion.IsAtLeastNautilus() {
		logger.Errorf("Ceph NFS is only supported with Nautilus or newer. CRD %s cleanup will be ignored.", nfs.Name)
		return
	}

	c.acquireOrchestrationLock()
	defer c.releaseOrchestrationLock()

	err := c.downCephNFS(*nfs, 0)
	if err != nil {
		logger.Errorf("failed to delete file system %s. %+v", nfs.Name, err)
	}
}

func (c *CephNFSController) ParentClusterChanged(cluster cephv1.ClusterSpec, clusterInfo *cephconfig.ClusterInfo) {
	c.clusterInfo = clusterInfo
	if cluster.CephVersion.Image == c.cephVersion.Image || !c.clusterInfo.CephVersion.IsAtLeastNautilus() {
		logger.Debugf("No need to update the nfs daemons after the parent cluster changed")
		return
	}

	c.acquireOrchestrationLock()
	defer c.releaseOrchestrationLock()

	c.cephVersion = cluster.CephVersion
	nfses, err := c.context.RookClientset.CephV1().CephNFSes(c.namespace).List(metav1.ListOptions{})
	if err != nil {
		logger.Errorf("failed to retrieve NFSes to update the ceph version. %+v", err)
		return
	}
	for _, nfs := range nfses.Items {
		logger.Infof("updating the ceph version for nfs %s to %s", nfs.Name, c.cephVersion.Image)
		err := c.upCephNFS(nfs, 0)
		if err != nil {
			logger.Errorf("failed to update nfs %s. %+v", nfs.Name, err)
		} else {
			logger.Infof("updated nfs %s to ceph version %s", nfs.Name, c.cephVersion.Image)
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
