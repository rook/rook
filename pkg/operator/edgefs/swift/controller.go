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

// scale-out, multi-cloud OpenStack/SWIFT services controller
package swift

import (
	"fmt"
	"reflect"

	"github.com/coreos/pkg/capnslog"
	"github.com/google/go-cmp/cmp"
	edgefsv1 "github.com/rook/rook/pkg/apis/edgefs.rook.io/v1"
	rookalpha "github.com/rook/rook/pkg/apis/rook.io/v1alpha2"
	"github.com/rook/rook/pkg/clusterd"
	"github.com/rook/rook/pkg/operator/k8sutil"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/tools/cache"
)

const (
	customResourceName       = "swift"
	customResourceNamePlural = "swifts"
)

var logger = capnslog.NewPackageLogger("github.com/rook/rook", "edgefs-op-swift")

// SWIFTResource represents the swift custom resource
var SWIFTResource = k8sutil.CustomResource{
	Name:    customResourceName,
	Plural:  customResourceNamePlural,
	Group:   edgefsv1.CustomResourceGroup,
	Version: edgefsv1.Version,
	Kind:    reflect.TypeOf(edgefsv1.SWIFT{}).Name(),
}

// SWIFTController represents a controller object for swift custom resources
type SWIFTController struct {
	context          *clusterd.Context
	namespace        string
	rookImage        string
	NetworkSpec      rookalpha.NetworkSpec
	dataDirHostPath  string
	dataVolumeSize   resource.Quantity
	placement        rookalpha.Placement
	resources        v1.ResourceRequirements
	resourceProfile  string
	ownerRef         metav1.OwnerReference
	useHostLocalTime bool
}

// NewSWIFTController create controller for watching SWIFT custom resources created
func NewSWIFTController(
	context *clusterd.Context,
	namespace string,
	rookImage string,
	NetworkSpec rookalpha.NetworkSpec,
	dataDirHostPath string,
	dataVolumeSize resource.Quantity,
	placement rookalpha.Placement,
	resources v1.ResourceRequirements,
	resourceProfile string,
	ownerRef metav1.OwnerReference,
	useHostLocalTime bool,
) *SWIFTController {
	return &SWIFTController{
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

// StartWatch watches for instances of SWIFT custom resources and acts on them
func (c *SWIFTController) StartWatch(stopCh chan struct{}) error {

	resourceHandlerFuncs := cache.ResourceEventHandlerFuncs{
		AddFunc:    c.onAdd,
		UpdateFunc: c.onUpdate,
		DeleteFunc: c.onDelete,
	}

	logger.Infof("start watching swift resources in namespace %s", c.namespace)
	go k8sutil.WatchCR(SWIFTResource, c.namespace, resourceHandlerFuncs, c.context.RookClientset.EdgefsV1().RESTClient(), &edgefsv1.SWIFT{}, stopCh)

	return nil
}

func (c *SWIFTController) onAdd(obj interface{}) {
	swift, err := getSWIFTObject(obj)
	if err != nil {
		logger.Errorf("failed to get swift object: %+v", err)
		return
	}

	if err = c.CreateService(*swift, c.serviceOwners(swift)); err != nil {
		logger.Errorf("failed to create swift %s. %+v", swift.Name, err)
	}
}

func (c *SWIFTController) onUpdate(oldObj, newObj interface{}) {
	oldService, err := getSWIFTObject(oldObj)
	if err != nil {
		logger.Errorf("failed to get old swift object: %+v", err)
		return
	}
	newService, err := getSWIFTObject(newObj)
	if err != nil {
		logger.Errorf("failed to get new swift object: %+v", err)
		return
	}

	if !serviceChanged(oldService.Spec, newService.Spec) {
		logger.Debugf("swift %s did not change", newService.Name)
		return
	}

	logger.Infof("applying swift %s changes", newService.Name)
	if err = c.UpdateService(*newService, c.serviceOwners(newService)); err != nil {
		logger.Errorf("failed to create (modify) swift %s. %+v", newService.Name, err)
	}
}

func (c *SWIFTController) onDelete(obj interface{}) {
	swift, err := getSWIFTObject(obj)
	if err != nil {
		logger.Errorf("failed to get swift object: %+v", err)
		return
	}

	if err = c.DeleteService(*swift); err != nil {
		logger.Errorf("failed to delete swift %s. %+v", swift.Name, err)
	}
}

func (c *SWIFTController) serviceOwners(service *edgefsv1.SWIFT) []metav1.OwnerReference {
	// Only set the cluster crd as the owner of the SWIFT resources.
	// If the SWIFT crd is deleted, the operator will explicitly remove the SWIFT resources.
	// If the SWIFT crd still exists when the cluster crd is deleted, this will make sure the SWIFT
	// resources are cleaned up.
	return []metav1.OwnerReference{c.ownerRef}
}

func (c *SWIFTController) ParentClusterChanged(cluster edgefsv1.ClusterSpec) {
	if c.rookImage == cluster.EdgefsImageName {
		logger.Infof("No need to update the swift service, the same images present")
		return
	}

	// update controller options by updated cluster spec
	c.rookImage = cluster.EdgefsImageName

	svcs, err := c.context.RookClientset.EdgefsV1().SWIFTs(c.namespace).List(metav1.ListOptions{})
	if err != nil {
		logger.Errorf("failed to retrieve SWIFTs to update the Edgefs version. %+v", err)
		return
	}
	for _, svc := range svcs.Items {
		logger.Infof("updating the Edgefs version for swift service %s to %s", svc.Name, cluster.EdgefsImageName)
		err := c.UpdateService(svc, nil)
		if err != nil {
			logger.Errorf("failed to update swift service %s. %+v", svc.Name, err)
		} else {
			logger.Infof("updated swift service %s to Edgefs version %s", svc.Name, cluster.EdgefsImageName)
		}
	}
}

func serviceChanged(oldService, newService edgefsv1.SWIFTSpec) bool {
	var diff string
	if !reflect.DeepEqual(oldService, newService) {
		func() {
			defer func() {
				if err := recover(); err != nil {
					logger.Warningf("Encountered an issue getting SWIFT service change differences: %v", err)
				}
			}()

			// resource.Quantity has non-exportable fields, so we use its comparator method
			resourceQtyComparer := cmp.Comparer(func(x, y resource.Quantity) bool { return x.Cmp(y) == 0 })
			diff = cmp.Diff(oldService, newService, resourceQtyComparer)
			logger.Infof("The SWIFT Service has changed. diff=%s", diff)
		}()
	}

	if len(diff) > 0 {
		return true
	}

	return false
}

func getSWIFTObject(obj interface{}) (swift *edgefsv1.SWIFT, err error) {
	var ok bool
	swift, ok = obj.(*edgefsv1.SWIFT)
	if ok {
		// the swift object is of the latest type, simply return it
		return swift.DeepCopy(), nil
	}

	return nil, fmt.Errorf("not a known swift object: %+v", obj)
}
