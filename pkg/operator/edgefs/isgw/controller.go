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

// scale-out, multi-cloud Edge-X ISGW (Inter-Segment Gateway) services controller
package isgw

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
	customResourceName       = "isgw"
	customResourceNamePlural = "isgws"
)

var logger = capnslog.NewPackageLogger("github.com/rook/rook", "edgefs-op-isgw")

// ISGWResource represents the isgw custom resource
var ISGWResource = opkit.CustomResource{
	Name:    customResourceName,
	Plural:  customResourceNamePlural,
	Group:   edgefsv1beta1.CustomResourceGroup,
	Version: edgefsv1beta1.Version,
	Scope:   apiextensionsv1beta1.NamespaceScoped,
	Kind:    reflect.TypeOf(edgefsv1beta1.ISGW{}).Name(),
}

// ISGWController represents a controller object for isgw custom resources
type ISGWController struct {
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

// NewISGWController create controller for watching ISGW custom resources created
func NewISGWController(
	context *clusterd.Context, rookImage string,
	hostNetwork bool,
	dataDirHostPath string,
	dataVolumeSize resource.Quantity,
	placement rookalpha.Placement,
	resources v1.ResourceRequirements,
	resourceProfile string,
	ownerRef metav1.OwnerReference,
) *ISGWController {
	return &ISGWController{
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

// StartWatch watches for instances of ISGW custom resources and acts on them
func (c *ISGWController) StartWatch(namespace string, stopCh chan struct{}) error {

	resourceHandlerFuncs := cache.ResourceEventHandlerFuncs{
		AddFunc:    c.onAdd,
		UpdateFunc: c.onUpdate,
		DeleteFunc: c.onDelete,
	}

	logger.Infof("start watching isgw resources in namespace %s", namespace)
	watcher := opkit.NewWatcher(ISGWResource, namespace, resourceHandlerFuncs, c.context.RookClientset.EdgefsV1beta1().RESTClient())
	go watcher.Watch(&edgefsv1beta1.ISGW{}, stopCh)

	return nil
}

func (c *ISGWController) onAdd(obj interface{}) {
	isgw, err := getISGWObject(obj)
	if err != nil {
		logger.Errorf("failed to get isgw object: %+v", err)
		return
	}

	if err = c.CreateService(*isgw, c.serviceOwners(isgw)); err != nil {
		logger.Errorf("failed to create isgw %s. %+v", isgw.Name, err)
	}
}

func (c *ISGWController) onUpdate(oldObj, newObj interface{}) {
	oldService, err := getISGWObject(oldObj)
	if err != nil {
		logger.Errorf("failed to get old isgw object: %+v", err)
		return
	}
	newService, err := getISGWObject(newObj)
	if err != nil {
		logger.Errorf("failed to get new isgw object: %+v", err)
		return
	}

	if !serviceChanged(oldService.Spec, newService.Spec) {
		logger.Debugf("isgw %s did not change", newService.Name)
		return
	}

	logger.Infof("applying isgw %s changes", newService.Name)
	if err = c.UpdateService(*newService, c.serviceOwners(newService)); err != nil {
		logger.Errorf("failed to create (modify) isgw %s. %+v", newService.Name, err)
	}
}

func (c *ISGWController) onDelete(obj interface{}) {
	isgw, err := getISGWObject(obj)
	if err != nil {
		logger.Errorf("failed to get isgw object: %+v", err)
		return
	}

	if err = c.DeleteService(*isgw); err != nil {
		logger.Errorf("failed to delete isgw %s. %+v", isgw.Name, err)
	}
}

func (c *ISGWController) serviceOwners(service *edgefsv1beta1.ISGW) []metav1.OwnerReference {
	// Only set the cluster crd as the owner of the ISGW resources.
	// If the ISGW crd is deleted, the operator will explicitly remove the ISGW resources.
	// If the ISGW crd still exists when the cluster crd is deleted, this will make sure the ISGW
	// resources are cleaned up.
	return []metav1.OwnerReference{c.ownerRef}
}

func serviceChanged(oldService, newService edgefsv1beta1.ISGWSpec) bool {
	return false
}

func getISGWObject(obj interface{}) (isgw *edgefsv1beta1.ISGW, err error) {
	var ok bool
	isgw, ok = obj.(*edgefsv1beta1.ISGW)
	if ok {
		// the isgw object is of the latest type, simply return it
		return isgw.DeepCopy(), nil
	}

	return nil, fmt.Errorf("not a known isgw object: %+v", obj)
}
