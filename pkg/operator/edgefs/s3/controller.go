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
	"github.com/google/go-cmp/cmp"
	edgefsv1 "github.com/rook/rook/pkg/apis/edgefs.rook.io/v1"
	rookv1 "github.com/rook/rook/pkg/apis/rook.io/v1"
	"github.com/rook/rook/pkg/clusterd"
	"github.com/rook/rook/pkg/operator/k8sutil"
	v1 "k8s.io/api/core/v1"
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
var S3Resource = k8sutil.CustomResource{
	Name:    customResourceName,
	Plural:  customResourceNamePlural,
	Group:   edgefsv1.CustomResourceGroup,
	Version: edgefsv1.Version,
	Kind:    reflect.TypeOf(edgefsv1.S3{}).Name(),
}

// S3Controller represents a controller object for s3 custom resources
type S3Controller struct {
	context          *clusterd.Context
	namespace        string
	rookImage        string
	NetworkSpec      rookv1.NetworkSpec
	dataDirHostPath  string
	dataVolumeSize   resource.Quantity
	annotations      rookv1.Annotations
	placement        rookv1.Placement
	resources        v1.ResourceRequirements
	resourceProfile  string
	ownerRef         metav1.OwnerReference
	useHostLocalTime bool
}

// NewS3Controller create controller for watching S3 custom resources created
func NewS3Controller(
	context *clusterd.Context,
	namespace string,
	rookImage string,
	NetworkSpec rookv1.NetworkSpec,
	dataDirHostPath string,
	dataVolumeSize resource.Quantity,
	placement rookv1.Placement,
	resources v1.ResourceRequirements,
	resourceProfile string,
	ownerRef metav1.OwnerReference,
	useHostLocalTime bool,
) *S3Controller {
	return &S3Controller{
		context:          context,
		namespace:        namespace,
		rookImage:        rookImage,
		NetworkSpec:      NetworkSpec,
		dataDirHostPath:  dataDirHostPath,
		dataVolumeSize:   dataVolumeSize,
		placement:        placement,
		resources:        resources,
		resourceProfile:  resourceProfile,
		ownerRef:         ownerRef,
		useHostLocalTime: useHostLocalTime,
	}
}

// StartWatch watches for instances of S3 custom resources and acts on them
func (c *S3Controller) StartWatch(stopCh chan struct{}) {

	resourceHandlerFuncs := cache.ResourceEventHandlerFuncs{
		AddFunc:    c.onAdd,
		UpdateFunc: c.onUpdate,
		DeleteFunc: c.onDelete,
	}

	logger.Infof("start watching s3 resources in namespace %s", c.namespace)
	go k8sutil.WatchCR(S3Resource, c.namespace, resourceHandlerFuncs, c.context.RookClientset.EdgefsV1().RESTClient(), &edgefsv1.S3{}, stopCh)
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

func (c *S3Controller) serviceOwners(service *edgefsv1.S3) []metav1.OwnerReference {
	// Only set the cluster crd as the owner of the S3 resources.
	// If the S3 crd is deleted, the operator will explicitly remove the S3 resources.
	// If the S3 crd still exists when the cluster crd is deleted, this will make sure the S3
	// resources are cleaned up.
	return []metav1.OwnerReference{c.ownerRef}
}

func (c *S3Controller) ParentClusterChanged(cluster edgefsv1.ClusterSpec) {
	if c.rookImage == cluster.EdgefsImageName {
		logger.Infof("No need to update the s3 service, the same images present")
		return
	}

	// update controller options by updated cluster spec
	c.rookImage = cluster.EdgefsImageName

	s3s, err := c.context.RookClientset.EdgefsV1().S3s(c.namespace).List(metav1.ListOptions{})
	if err != nil {
		logger.Errorf("failed to retrieve S3s to update the Edgefs version. %+v", err)
		return
	}
	for _, s3 := range s3s.Items {
		logger.Infof("updating the Edgefs version for s3 %s to %s", s3.Name, cluster.EdgefsImageName)
		err := c.UpdateService(s3, nil)
		if err != nil {
			logger.Errorf("failed to update s3 service %s. %+v", s3.Name, err)
		} else {
			logger.Infof("updated s3 service %s to Edgefs version %s", s3.Name, cluster.EdgefsImageName)
		}
	}
}

func serviceChanged(oldService, newService edgefsv1.S3Spec) bool {
	var diff string
	if !reflect.DeepEqual(oldService, newService) {
		func() {
			defer func() {
				if err := recover(); err != nil {
					logger.Warningf("Encountered an issue getting S3 service change differences: %v", err)
				}
			}()

			// resource.Quantity has non-exportable fields, so we use its comparator method
			resourceQtyComparer := cmp.Comparer(func(x, y resource.Quantity) bool { return x.Cmp(y) == 0 })
			diff = cmp.Diff(oldService, newService, resourceQtyComparer)
			logger.Infof("The S3 Service has changed. diff=%s", diff)
		}()
	}

	if len(diff) > 0 {
		return true
	}

	return false
}

func getS3Object(obj interface{}) (s3 *edgefsv1.S3, err error) {
	var ok bool
	s3, ok = obj.(*edgefsv1.S3)
	if ok {
		// the s3 object is of the latest type, simply return it
		return s3.DeepCopy(), nil
	}

	return nil, fmt.Errorf("not a known s3 object: %+v", obj)
}
