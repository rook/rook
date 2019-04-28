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
package s3

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
	customResourceName       = "s3"
	customResourceNamePlural = "s3s"
)

var logger = capnslog.NewPackageLogger("github.com/rook/rook", "edgefs-op-s3")

// S3Resource represents the s3 custom resource
var S3Resource = opkit.CustomResource{
	Name:    customResourceName,
	Plural:  customResourceNamePlural,
	Group:   edgefsv1beta1.CustomResourceGroup,
	Version: edgefsv1beta1.Version,
	Scope:   apiextensionsv1beta1.NamespaceScoped,
	Kind:    reflect.TypeOf(edgefsv1beta1.S3{}).Name(),
}

// S3Controller represents a controller object for s3 custom resources
type S3Controller struct {
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

// NewS3Controller create controller for watching S3 custom resources created
func NewS3Controller(
	context *clusterd.Context, rookImage string,
	hostNetwork bool,
	dataDirHostPath string,
	dataVolumeSize resource.Quantity,
	placement rookalpha.Placement,
	resources v1.ResourceRequirements,
	resourceProfile string,
	ownerRef metav1.OwnerReference,
) *S3Controller {
	return &S3Controller{
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

// StartWatch watches for instances of S3 custom resources and acts on them
func (c *S3Controller) StartWatch(namespace string, stopCh chan struct{}) error {

	resourceHandlerFuncs := cache.ResourceEventHandlerFuncs{
		AddFunc:    c.onAdd,
		UpdateFunc: c.onUpdate,
		DeleteFunc: c.onDelete,
	}

	logger.Infof("start watching s3 resources in namespace %s", namespace)
	watcher := opkit.NewWatcher(S3Resource, namespace, resourceHandlerFuncs, c.context.RookClientset.EdgefsV1beta1().RESTClient())
	go watcher.Watch(&edgefsv1beta1.S3{}, stopCh)

	return nil
}

func (c *S3Controller) onAdd(obj interface{}) {
	s3, err := getS3Object(obj)
	if err != nil {
		logger.Errorf("failed to get s3 object: %+v", err)
		return
	}

	if err = c.CreateService(*s3, c.serviceOwners(s3)); err != nil {
		logger.Errorf("failed to create s3 %s. %+v", s3.Name, err)
	}
}

func (c *S3Controller) onUpdate(oldObj, newObj interface{}) {
	oldService, err := getS3Object(oldObj)
	if err != nil {
		logger.Errorf("failed to get old s3 object: %+v", err)
		return
	}
	newService, err := getS3Object(newObj)
	if err != nil {
		logger.Errorf("failed to get new s3 object: %+v", err)
		return
	}

	if !serviceChanged(oldService.Spec, newService.Spec) {
		logger.Debugf("s3 %s did not change", newService.Name)
		return
	}

	logger.Infof("applying s3 %s changes", newService.Name)
	if err = c.UpdateService(*newService, c.serviceOwners(newService)); err != nil {
		logger.Errorf("failed to create (modify) s3 %s. %+v", newService.Name, err)
	}
}

func (c *S3Controller) onDelete(obj interface{}) {
	s3, err := getS3Object(obj)
	if err != nil {
		logger.Errorf("failed to get s3 object: %+v", err)
		return
	}

	if err = c.DeleteService(*s3); err != nil {
		logger.Errorf("failed to delete s3 %s. %+v", s3.Name, err)
	}
}

func (c *S3Controller) serviceOwners(service *edgefsv1beta1.S3) []metav1.OwnerReference {
	// Only set the cluster crd as the owner of the S3 resources.
	// If the S3 crd is deleted, the operator will explicitly remove the S3 resources.
	// If the S3 crd still exists when the cluster crd is deleted, this will make sure the S3
	// resources are cleaned up.
	return []metav1.OwnerReference{c.ownerRef}
}

func serviceChanged(oldService, newService edgefsv1beta1.S3Spec) bool {
	return false
}

func getS3Object(obj interface{}) (s3 *edgefsv1beta1.S3, err error) {
	var ok bool
	s3, ok = obj.(*edgefsv1beta1.S3)
	if ok {
		// the s3 object is of the latest type, simply return it
		return s3.DeepCopy(), nil
	}

	return nil, fmt.Errorf("not a known s3 object: %+v", obj)
}
