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
	customResourceName       = "isgw"
	customResourceNamePlural = "isgws"
)

var logger = capnslog.NewPackageLogger("github.com/rook/rook", "edgefs-op-isgw")

// ISGWResource represents the isgw custom resource
var ISGWResource = k8sutil.CustomResource{
	Name:    customResourceName,
	Plural:  customResourceNamePlural,
	Group:   edgefsv1.CustomResourceGroup,
	Version: edgefsv1.Version,
	Kind:    reflect.TypeOf(edgefsv1.ISGW{}).Name(),
}

// ISGWController represents a controller object for isgw custom resources
type ISGWController struct {
	context          *clusterd.Context
	namespace        string
	rookImage        string
	NetworkSpec      rookalpha.NetworkSpec
	dataDirHostPath  string
	dataVolumeSize   resource.Quantity
	annotations      rookalpha.Annotations
	placement        rookalpha.Placement
	resources        v1.ResourceRequirements
	resourceProfile  string
	ownerRef         metav1.OwnerReference
	useHostLocalTime bool
}

// NewISGWController create controller for watching ISGW custom resources created
func NewISGWController(
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
) *ISGWController {
	return &ISGWController{
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

// StartWatch watches for instances of ISGW custom resources and acts on them
func (c *ISGWController) StartWatch(stopCh chan struct{}) error {

	resourceHandlerFuncs := cache.ResourceEventHandlerFuncs{
		AddFunc:    c.onAdd,
		UpdateFunc: c.onUpdate,
		DeleteFunc: c.onDelete,
	}

	logger.Infof("start watching isgw resources in namespace %s", c.namespace)
	go k8sutil.WatchCR(ISGWResource, c.namespace, resourceHandlerFuncs, c.context.RookClientset.EdgefsV1().RESTClient(), &edgefsv1.ISGW{}, stopCh)

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

func (c *ISGWController) serviceOwners(service *edgefsv1.ISGW) []metav1.OwnerReference {
	// Only set the cluster crd as the owner of the ISGW resources.
	// If the ISGW crd is deleted, the operator will explicitly remove the ISGW resources.
	// If the ISGW crd still exists when the cluster crd is deleted, this will make sure the ISGW
	// resources are cleaned up.
	return []metav1.OwnerReference{c.ownerRef}
}

func (c *ISGWController) ParentClusterChanged(cluster edgefsv1.ClusterSpec) {
	if c.rookImage == cluster.EdgefsImageName {
		logger.Infof("No need to update the isgw service, the same images present")
		return
	}

	// update controller options by updated cluster spec
	c.rookImage = cluster.EdgefsImageName

	isgws, err := c.context.RookClientset.EdgefsV1().ISGWs(c.namespace).List(metav1.ListOptions{})
	if err != nil {
		logger.Errorf("failed to retrieve ISGWs to update the Edgefs version. %+v", err)
		return
	}
	for _, isgw := range isgws.Items {
		logger.Infof("updating the Edgefs version for isgw service %s to %s", isgw.Name, cluster.EdgefsImageName)
		err := c.UpdateService(isgw, nil)
		if err != nil {
			logger.Errorf("failed to update isgw service %s. %+v", isgw.Name, err)
		} else {
			logger.Infof("updated isgw service %s to Edgefs version %s", isgw.Name, cluster.EdgefsImageName)
		}
	}
}

func serviceChanged(oldService, newService edgefsv1.ISGWSpec) bool {
	var diff string
	if !reflect.DeepEqual(oldService, newService) {
		func() {
			defer func() {
				if err := recover(); err != nil {
					logger.Warningf("Encountered an issue getting ISGW service change differences: %v", err)
				}
			}()

			// resource.Quantity has non-exportable fields, so we use its comparator method
			resourceQtyComparer := cmp.Comparer(func(x, y resource.Quantity) bool { return x.Cmp(y) == 0 })
			diff = cmp.Diff(oldService, newService, resourceQtyComparer)
			logger.Infof("The ISGW Service has changed. diff=%s", diff)
		}()
	}

	if len(diff) > 0 {
		return true
	}

	return false
}

func getISGWObject(obj interface{}) (isgw *edgefsv1.ISGW, err error) {
	var ok bool
	isgw, ok = obj.(*edgefsv1.ISGW)
	if ok {
		// the isgw object is of the latest type, simply return it
		return isgw.DeepCopy(), nil
	}

	return nil, fmt.Errorf("not a known isgw object: %+v", obj)
}
