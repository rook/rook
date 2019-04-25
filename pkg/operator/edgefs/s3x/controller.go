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

// scale-out, multi-cloud Edge-X S3 services controller
package s3x

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
	customResourceName       = "s3x"
	customResourceNamePlural = "s3xs"
)

var logger = capnslog.NewPackageLogger("github.com/rook/rook", "edgefs-op-s3x")

// S3XResource represents the s3x custom resource
var S3XResource = opkit.CustomResource{
	Name:    customResourceName,
	Plural:  customResourceNamePlural,
	Group:   edgefsv1beta1.CustomResourceGroup,
	Version: edgefsv1beta1.Version,
	Scope:   apiextensionsv1beta1.NamespaceScoped,
	Kind:    reflect.TypeOf(edgefsv1beta1.S3X{}).Name(),
}

// S3XController represents a controller object for s3x custom resources
type S3XController struct {
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

// NewS3XController create controller for watching S3X custom resources created
func NewS3XController(
	context *clusterd.Context, rookImage string,
	hostNetwork bool,
	dataDirHostPath string,
	dataVolumeSize resource.Quantity,
	placement rookalpha.Placement,
	resources v1.ResourceRequirements,
	resourceProfile string,
	ownerRef metav1.OwnerReference,
) *S3XController {
	return &S3XController{
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

// StartWatch watches for instances of S3X custom resources and acts on them
func (c *S3XController) StartWatch(namespace string, stopCh chan struct{}) error {

	resourceHandlerFuncs := cache.ResourceEventHandlerFuncs{
		AddFunc:    c.onAdd,
		UpdateFunc: c.onUpdate,
		DeleteFunc: c.onDelete,
	}

	logger.Infof("start watching s3x resources in namespace %s", namespace)
	watcher := opkit.NewWatcher(S3XResource, namespace, resourceHandlerFuncs, c.context.RookClientset.EdgefsV1beta1().RESTClient())
	go watcher.Watch(&edgefsv1beta1.S3X{}, stopCh)

	return nil
}

func (c *S3XController) onAdd(obj interface{}) {
	s3x, err := getS3XObject(obj)
	if err != nil {
		logger.Errorf("failed to get s3x object: %+v", err)
		return
	}

	if err = c.CreateService(*s3x, c.serviceOwners(s3x)); err != nil {
		logger.Errorf("failed to create s3x %s. %+v", s3x.Name, err)
	}
}

func (c *S3XController) onUpdate(oldObj, newObj interface{}) {
	oldService, err := getS3XObject(oldObj)
	if err != nil {
		logger.Errorf("failed to get old s3x object: %+v", err)
		return
	}
	newService, err := getS3XObject(newObj)
	if err != nil {
		logger.Errorf("failed to get new s3x object: %+v", err)
		return
	}

	if !serviceChanged(oldService.Spec, newService.Spec) {
		logger.Debugf("s3x %s did not change", newService.Name)
		return
	}

	logger.Infof("applying s3x %s changes", newService.Name)
	if err = c.UpdateService(*newService, c.serviceOwners(newService)); err != nil {
		logger.Errorf("failed to create (modify) s3x %s. %+v", newService.Name, err)
	}
}

func (c *S3XController) onDelete(obj interface{}) {
	s3x, err := getS3XObject(obj)
	if err != nil {
		logger.Errorf("failed to get s3x object: %+v", err)
		return
	}

	if err = c.DeleteService(*s3x); err != nil {
		logger.Errorf("failed to delete s3x %s. %+v", s3x.Name, err)
	}
}

func (c *S3XController) serviceOwners(service *edgefsv1beta1.S3X) []metav1.OwnerReference {
	// Only set the cluster crd as the owner of the S3X resources.
	// If the S3X crd is deleted, the operator will explicitly remove the S3X resources.
	// If the S3X crd still exists when the cluster crd is deleted, this will make sure the S3X
	// resources are cleaned up.
	return []metav1.OwnerReference{c.ownerRef}
}

func serviceChanged(oldService, newService edgefsv1beta1.S3XSpec) bool {
	return false
}

func getS3XObject(obj interface{}) (s3x *edgefsv1beta1.S3X, err error) {
	var ok bool
	s3x, ok = obj.(*edgefsv1beta1.S3X)
	if ok {
		// the s3x object is of the latest type, simply return it
		return s3x.DeepCopy(), nil
	}

	return nil, fmt.Errorf("not a known s3x object: %+v", obj)
}
