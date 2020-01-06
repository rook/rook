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
	customResourceName       = "nfs"
	customResourceNamePlural = "nfss"
)

var logger = capnslog.NewPackageLogger("github.com/rook/rook", "edgefs-op-nfs")

// NFSResource represents the nfs custom resource
var NFSResource = k8sutil.CustomResource{
	Name:    customResourceName,
	Plural:  customResourceNamePlural,
	Group:   edgefsv1.CustomResourceGroup,
	Version: edgefsv1.Version,
	Kind:    reflect.TypeOf(edgefsv1.NFS{}).Name(),
}

// NFSController represents a controller object for nfs custom resources
type NFSController struct {
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

// NewNFSController create controller for watching nfs custom resources created
func NewNFSController(
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
) *NFSController {
	return &NFSController{
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

// StartWatch watches for instances of NFS custom resources and acts on them
func (c *NFSController) StartWatch(stopCh chan struct{}) error {

	resourceHandlerFuncs := cache.ResourceEventHandlerFuncs{
		AddFunc:    c.onAdd,
		UpdateFunc: c.onUpdate,
		DeleteFunc: c.onDelete,
	}

	logger.Infof("start watching nfs resources in namespace %s", c.namespace)
	go k8sutil.WatchCR(NFSResource, c.namespace, resourceHandlerFuncs, c.context.RookClientset.EdgefsV1().RESTClient(), &edgefsv1.NFS{}, stopCh)

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
func (c *NFSController) ParentClusterChanged(cluster edgefsv1.ClusterSpec) {
	if c.rookImage == cluster.EdgefsImageName {
		logger.Infof("No need to update the nfs service, the same images present")
		return
	}

	// update controller options by updated cluster spec
	c.rookImage = cluster.EdgefsImageName

	nfses, err := c.context.RookClientset.EdgefsV1().NFSs(c.namespace).List(metav1.ListOptions{})
	if err != nil {
		logger.Errorf("failed to retrieve NFSes to update the Edgefs version. %+v", err)
		return
	}
	for _, nfs := range nfses.Items {
		logger.Infof("updating the Edgefs version for nfs %s to %s", nfs.Name, cluster.EdgefsImageName)
		err := c.UpdateService(nfs, nil)
		if err != nil {
			logger.Errorf("failed to update nfs %s. %+v", nfs.Name, err)
		} else {
			logger.Infof("updated nfs %s to Edgefs version %s", nfs.Name, cluster.EdgefsImageName)
		}
	}
}

func (c *NFSController) serviceOwners(service *edgefsv1.NFS) []metav1.OwnerReference {
	// Only set the cluster crd as the owner of the NFS resources.
	// If the NFS crd is deleted, the operator will explicitly remove the NFS resources.
	// If the NFS crd still exists when the cluster crd is deleted, this will make sure the NFS
	// resources are cleaned up.
	return []metav1.OwnerReference{c.ownerRef}
}

func serviceChanged(oldService, newService edgefsv1.NFSSpec) bool {

	var diff string
	// any change in the crd will trigger an orchestration
	if !reflect.DeepEqual(oldService, newService) {
		func() {
			defer func() {
				if err := recover(); err != nil {
					logger.Warningf("Encountered an issue getting nfs service change differences: %v", err)
				}
			}()

			// resource.Quantity has non-exportable fields, so we use its comparator method
			resourceQtyComparer := cmp.Comparer(func(x, y resource.Quantity) bool { return x.Cmp(y) == 0 })
			diff = cmp.Diff(oldService, newService, resourceQtyComparer)
			logger.Infof("The NFS Service has changed. diff=%s", diff)
		}()
	}

	if len(diff) > 0 {
		return true
	}

	return false
}

func getNFSObject(obj interface{}) (nfs *edgefsv1.NFS, err error) {
	var ok bool
	nfs, ok = obj.(*edgefsv1.NFS)
	if ok {
		// the nfs object is of the latest type, simply return it
		return nfs.DeepCopy(), nil
	}

	return nil, fmt.Errorf("not a known nfs object: %+v", obj)
}
