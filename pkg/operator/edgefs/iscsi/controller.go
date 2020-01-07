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
	customResourceName       = "iscsi"
	customResourceNamePlural = "iscsis"
)

var logger = capnslog.NewPackageLogger("github.com/rook/rook", "edgefs-op-iscsi")

// ISCSIResource represents the iscsi custom resource
var ISCSIResource = k8sutil.CustomResource{
	Name:    customResourceName,
	Plural:  customResourceNamePlural,
	Group:   edgefsv1.CustomResourceGroup,
	Version: edgefsv1.Version,
	Kind:    reflect.TypeOf(edgefsv1.ISCSI{}).Name(),
}

// ISCSIController represents a controller object for iscsi custom resources
type ISCSIController struct {
	context          *clusterd.Context
	namespace        string
	rookImage        string
	NetworkSpec      rookalpha.NetworkSpec
	dataDirHostPath  string
	dataVolumeSize   resource.Quantity
	placement        rookalpha.Placement
	annotations      rookalpha.Annotations
	resources        v1.ResourceRequirements
	resourceProfile  string
	ownerRef         metav1.OwnerReference
	useHostLocalTime bool
}

// NewISCSIController create controller for watching ISCSI custom resources created
func NewISCSIController(
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
) *ISCSIController {
	return &ISCSIController{
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

// StartWatch watches for instances of ISCSI custom resources and acts on them
func (c *ISCSIController) StartWatch(stopCh chan struct{}) error {

	resourceHandlerFuncs := cache.ResourceEventHandlerFuncs{
		AddFunc:    c.onAdd,
		UpdateFunc: c.onUpdate,
		DeleteFunc: c.onDelete,
	}

	logger.Infof("start watching iscsi resources in namespace %s", c.namespace)
	go k8sutil.WatchCR(ISCSIResource, c.namespace, resourceHandlerFuncs, c.context.RookClientset.EdgefsV1().RESTClient(), &edgefsv1.ISCSI{}, stopCh)

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

func (c *ISCSIController) serviceOwners(service *edgefsv1.ISCSI) []metav1.OwnerReference {
	// Only set the cluster crd as the owner of the ISCSI resources.
	// If the ISCSI crd is deleted, the operator will explicitly remove the ISCSI resources.
	// If the ISCSI crd still exists when the cluster crd is deleted, this will make sure the ISCSI
	// resources are cleaned up.
	return []metav1.OwnerReference{c.ownerRef}
}

func (c *ISCSIController) ParentClusterChanged(cluster edgefsv1.ClusterSpec) {
	if c.rookImage == cluster.EdgefsImageName {
		logger.Infof("No need to update the iscsi service, the same images present")
		return
	}

	// update controller options by updated cluster spec
	c.rookImage = cluster.EdgefsImageName

	iscsis, err := c.context.RookClientset.EdgefsV1().ISCSIs(c.namespace).List(metav1.ListOptions{})
	if err != nil {
		logger.Errorf("failed to retrieve NFSes to update the Edgefs version. %+v", err)
		return
	}
	for _, iscsi := range iscsis.Items {
		logger.Infof("updating the Edgefs version for iscsi %s to %s", iscsi.Name, cluster.EdgefsImageName)
		err := c.UpdateService(iscsi, nil)
		if err != nil {
			logger.Errorf("failed to update iscsi service %s. %+v", iscsi.Name, err)
		} else {
			logger.Infof("updated iscsi service %s to Edgefs version %s", iscsi.Name, cluster.EdgefsImageName)
		}
	}
}

func serviceChanged(oldService, newService edgefsv1.ISCSISpec) bool {
	var diff string
	if !reflect.DeepEqual(oldService, newService) {
		func() {
			defer func() {
				if err := recover(); err != nil {
					logger.Warningf("Encountered an issue getting ISCSI service change differences: %v", err)
				}
			}()

			// resource.Quantity has non-exportable fields, so we use its comparator method
			resourceQtyComparer := cmp.Comparer(func(x, y resource.Quantity) bool { return x.Cmp(y) == 0 })
			diff = cmp.Diff(oldService, newService, resourceQtyComparer)
			logger.Infof("The ISCSI Service has changed. diff=%s", diff)
		}()
	}

	if len(diff) > 0 {
		return true
	}

	return false
}

func getISCSIObject(obj interface{}) (iscsi *edgefsv1.ISCSI, err error) {
	var ok bool
	iscsi, ok = obj.(*edgefsv1.ISCSI)
	if ok {
		// the iscsi object is of the latest type, simply return it
		return iscsi.DeepCopy(), nil
	}
	return nil, fmt.Errorf("not a known iscsi object: %+v", obj)
}
