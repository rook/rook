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

// scale-out, multi-cloud Edge-X ISCSI services controller
package iscsi

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
	customResourceName       = "iscsi"
	customResourceNamePlural = "iscsis"
)

var logger = capnslog.NewPackageLogger("github.com/rook/rook", "edgefs-op-iscsi")

// ISCSIResource represents the iscsi custom resource
var ISCSIResource = opkit.CustomResource{
	Name:    customResourceName,
	Plural:  customResourceNamePlural,
	Group:   edgefsv1beta1.CustomResourceGroup,
	Version: edgefsv1beta1.Version,
	Scope:   apiextensionsv1beta1.NamespaceScoped,
	Kind:    reflect.TypeOf(edgefsv1beta1.ISCSI{}).Name(),
}

// ISCSIController represents a controller object for iscsi custom resources
type ISCSIController struct {
	context         *clusterd.Context
	rookImage       string
	hostNetwork     bool
	dataDirHostPath string
	dataVolumeSize  resource.Quantity
	placement       rookalpha.Placement
	annotations     rookalpha.Annotations
	resources       v1.ResourceRequirements
	resourceProfile string
	ownerRef        metav1.OwnerReference
}

// NewISCSIController create controller for watching ISCSI custom resources created
func NewISCSIController(
	context *clusterd.Context, rookImage string,
	hostNetwork bool,
	dataDirHostPath string,
	dataVolumeSize resource.Quantity,
	placement rookalpha.Placement,
	resources v1.ResourceRequirements,
	resourceProfile string,
	ownerRef metav1.OwnerReference,
) *ISCSIController {
	return &ISCSIController{
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

// StartWatch watches for instances of ISCSI custom resources and acts on them
func (c *ISCSIController) StartWatch(namespace string, stopCh chan struct{}) error {

	resourceHandlerFuncs := cache.ResourceEventHandlerFuncs{
		AddFunc:    c.onAdd,
		UpdateFunc: c.onUpdate,
		DeleteFunc: c.onDelete,
	}

	logger.Infof("start watching iscsi resources in namespace %s", namespace)
	watcher := opkit.NewWatcher(ISCSIResource, namespace, resourceHandlerFuncs, c.context.RookClientset.EdgefsV1beta1().RESTClient())
	go watcher.Watch(&edgefsv1beta1.ISCSI{}, stopCh)

	return nil
}

func (c *ISCSIController) onAdd(obj interface{}) {
	iscsi, err := getISCSIObject(obj)
	if err != nil {
		logger.Errorf("failed to get iscsi object: %+v", err)
		return
	}

	if err = c.CreateService(*iscsi, c.serviceOwners(iscsi)); err != nil {
		logger.Errorf("failed to create iscsi %s. %+v", iscsi.Name, err)
	}
}

func (c *ISCSIController) onUpdate(oldObj, newObj interface{}) {
	oldService, err := getISCSIObject(oldObj)
	if err != nil {
		logger.Errorf("failed to get old iscsi object: %+v", err)
		return
	}
	newService, err := getISCSIObject(newObj)
	if err != nil {
		logger.Errorf("failed to get new iscsi object: %+v", err)
		return
	}

	if !serviceChanged(oldService.Spec, newService.Spec) {
		logger.Debugf("iscsi %s did not change", newService.Name)
		return
	}

	logger.Infof("applying iscsi %s changes", newService.Name)
	if err = c.UpdateService(*newService, c.serviceOwners(newService)); err != nil {
		logger.Errorf("failed to create (modify) iscsi %s. %+v", newService.Name, err)
	}
}

func (c *ISCSIController) onDelete(obj interface{}) {
	iscsi, err := getISCSIObject(obj)
	if err != nil {
		logger.Errorf("failed to get iscsi object: %+v", err)
		return
	}

	if err = c.DeleteService(*iscsi); err != nil {
		logger.Errorf("failed to delete iscsi %s. %+v", iscsi.Name, err)
	}
}

func (c *ISCSIController) serviceOwners(service *edgefsv1beta1.ISCSI) []metav1.OwnerReference {
	// Only set the cluster crd as the owner of the ISCSI resources.
	// If the ISCSI crd is deleted, the operator will explicitly remove the ISCSI resources.
	// If the ISCSI crd still exists when the cluster crd is deleted, this will make sure the ISCSI
	// resources are cleaned up.
	return []metav1.OwnerReference{c.ownerRef}
}

func serviceChanged(oldService, newService edgefsv1beta1.ISCSISpec) bool {
	return false
}

func getISCSIObject(obj interface{}) (iscsi *edgefsv1beta1.ISCSI, err error) {
	var ok bool
	iscsi, ok = obj.(*edgefsv1beta1.ISCSI)
	if ok {
		// the iscsi object is of the latest type, simply return it
		return iscsi.DeepCopy(), nil
	}

	return nil, fmt.Errorf("not a known iscsi object: %+v", obj)
}
