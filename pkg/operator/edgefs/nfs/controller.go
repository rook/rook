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

// scale-out, multi-cloud NFS
package nfs

import (
	"fmt"
	"reflect"

	"github.com/coreos/pkg/capnslog"
	opkit "github.com/rook/operator-kit"
	edgefsv1beta1 "github.com/rook/rook/pkg/apis/edgefs.rook.io/v1beta1"
	rookalpha "github.com/rook/rook/pkg/apis/rook.io/v1alpha2"
	"github.com/rook/rook/pkg/clusterd"
	"k8s.io/api/core/v1"
	apiextensionsv1beta1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1beta1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/tools/cache"
)

const (
	customResourceName       = "nfs"
	customResourceNamePlural = "nfss"
)

var logger = capnslog.NewPackageLogger("github.com/rook/rook", "edgefs-op-nfs")

// NFSResource represents the nfs custom resource
var NFSResource = opkit.CustomResource{
	Name:    customResourceName,
	Plural:  customResourceNamePlural,
	Group:   edgefsv1beta1.CustomResourceGroup,
	Version: edgefsv1beta1.Version,
	Scope:   apiextensionsv1beta1.NamespaceScoped,
	Kind:    reflect.TypeOf(edgefsv1beta1.NFS{}).Name(),
}

// NFSController represents a controller object for nfs custom resources
type NFSController struct {
	context         *clusterd.Context
	rookImage       string
	hostNetwork     bool
	dataDirHostPath string
	dataVolumeSize  resource.Quantity
	annotations     rookalpha.Annotations
	placement       rookalpha.Placement
	resources       v1.ResourceRequirements
	resourceProfile string
	ownerRef        metav1.OwnerReference
}

// NewNFSController create controller for watching nfs custom resources created
func NewNFSController(
	context *clusterd.Context, rookImage string,
	hostNetwork bool,
	dataDirHostPath string,
	dataVolumeSize resource.Quantity,
	placement rookalpha.Placement,
	resources v1.ResourceRequirements,
	resourceProfile string,
	ownerRef metav1.OwnerReference,
) *NFSController {
	return &NFSController{
		context:         context,
		rookImage:       rookImage,
		hostNetwork:     hostNetwork,
		dataDirHostPath: dataDirHostPath,
		dataVolumeSize:  dataVolumeSize,
		placement:       placement,
		resources:       resources,
		resourceProfile: resourceProfile,
		ownerRef:        ownerRef,
	}
}

// StartWatch watches for instances of NFS custom resources and acts on them
func (c *NFSController) StartWatch(namespace string, stopCh chan struct{}) error {

	resourceHandlerFuncs := cache.ResourceEventHandlerFuncs{
		AddFunc:    c.onAdd,
		UpdateFunc: c.onUpdate,
		DeleteFunc: c.onDelete,
	}

	logger.Infof("start watching nfs resources in namespace %s", namespace)
	watcher := opkit.NewWatcher(NFSResource, namespace, resourceHandlerFuncs, c.context.RookClientset.EdgefsV1beta1().RESTClient())
	go watcher.Watch(&edgefsv1beta1.NFS{}, stopCh)

	return nil
}

func (c *NFSController) onAdd(obj interface{}) {
	nfs, err := getNFSObject(obj)
	if err != nil {
		logger.Errorf("failed to get nfs object: %+v", err)
		return
	}

	if err = c.CreateService(*nfs, c.serviceOwners(nfs)); err != nil {
		logger.Errorf("failed to create nfs %s. %+v", nfs.Name, err)
	}
}

func (c *NFSController) onUpdate(oldObj, newObj interface{}) {
	oldService, err := getNFSObject(oldObj)
	if err != nil {
		logger.Errorf("failed to get old nfs object: %+v", err)
		return
	}
	newService, err := getNFSObject(newObj)
	if err != nil {
		logger.Errorf("failed to get new nfs object: %+v", err)
		return
	}

	if !serviceChanged(oldService.Spec, newService.Spec) {
		logger.Debugf("nfs %s did not change", newService.Name)
		return
	}

	logger.Infof("applying nfs %s changes", newService.Name)
	if err = c.UpdateService(*newService, c.serviceOwners(newService)); err != nil {
		logger.Errorf("failed to create (modify) nfs %s. %+v", newService.Name, err)
	}
}

func (c *NFSController) onDelete(obj interface{}) {
	nfs, err := getNFSObject(obj)
	if err != nil {
		logger.Errorf("failed to get nfs object: %+v", err)
		return
	}

	if err = c.DeleteService(*nfs); err != nil {
		logger.Errorf("failed to delete nfs %s. %+v", nfs.Name, err)
	}
}

func (c *NFSController) serviceOwners(service *edgefsv1beta1.NFS) []metav1.OwnerReference {
	// Only set the cluster crd as the owner of the NFS resources.
	// If the NFS crd is deleted, the operator will explicitly remove the NFS resources.
	// If the NFS crd still exists when the cluster crd is deleted, this will make sure the NFS
	// resources are cleaned up.
	return []metav1.OwnerReference{c.ownerRef}
}

func serviceChanged(oldService, newService edgefsv1beta1.NFSSpec) bool {
	return false
}

func getNFSObject(obj interface{}) (nfs *edgefsv1beta1.NFS, err error) {
	var ok bool
	nfs, ok = obj.(*edgefsv1beta1.NFS)
	if ok {
		// the nfs object is of the latest type, simply return it
		return nfs.DeepCopy(), nil
	}

	return nil, fmt.Errorf("not a known nfs object: %+v", obj)
}
