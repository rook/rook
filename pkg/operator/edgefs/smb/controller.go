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

// scale-out, multi-cloud SMB
package smb

import (
	"context"
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
	customResourceName       = "smb"
	customResourceNamePlural = "smbs"
)

var logger = capnslog.NewPackageLogger("github.com/rook/rook", "edgefs-op-smb")

// SMBResource represents the smb custom resource
var SMBResource = k8sutil.CustomResource{
	Name:    customResourceName,
	Plural:  customResourceNamePlural,
	Group:   edgefsv1.CustomResourceGroup,
	Version: edgefsv1.Version,
	Kind:    reflect.TypeOf(edgefsv1.SMB{}).Name(),
}

// SMBController represents a controller object for smb custom resources
type SMBController struct {
	context          *clusterd.Context
	namespace        string
	rookImage        string
	NetworkSpec      rookv1.NetworkSpec
	dataDirHostPath  string
	dataVolumeSize   resource.Quantity
	placement        rookv1.Placement
	resources        v1.ResourceRequirements
	resourceProfile  string
	ownerRef         metav1.OwnerReference
	useHostLocalTime bool
}

// NewSMBController create controller for watching smb custom resources created
func NewSMBController(
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
) *SMBController {
	return &SMBController{
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

// StartWatch watches for instances of SMB custom resources and acts on them
func (c *SMBController) StartWatch(stopCh chan struct{}) {

	resourceHandlerFuncs := cache.ResourceEventHandlerFuncs{
		AddFunc:    c.onAdd,
		UpdateFunc: c.onUpdate,
		DeleteFunc: c.onDelete,
	}

	logger.Infof("start watching smb resources in namespace %s", c.namespace)
	go k8sutil.WatchCR(SMBResource, c.namespace, resourceHandlerFuncs, c.context.RookClientset.EdgefsV1().RESTClient(), &edgefsv1.SMB{}, stopCh)

}

func (c *SMBController) onAdd(obj interface{}) {
	smb, err := getSMBObject(obj)
	if err != nil {
		logger.Errorf("failed to get smb object: %+v", err)
		return
	}

	if err = c.CreateService(*smb, c.serviceOwners(smb)); err != nil {
		logger.Errorf("failed to create smb %s. %+v", smb.Name, err)
	}
}

func (c *SMBController) onUpdate(oldObj, newObj interface{}) {
	oldService, err := getSMBObject(oldObj)
	if err != nil {
		logger.Errorf("failed to get old smb object: %+v", err)
		return
	}
	newService, err := getSMBObject(newObj)
	if err != nil {
		logger.Errorf("failed to get new smb object: %+v", err)
		return
	}

	if !serviceChanged(oldService.Spec, newService.Spec) {
		logger.Debugf("smb %s did not change", newService.Name)
		return
	}

	logger.Infof("applying smb %s changes", newService.Name)
	if err = c.UpdateService(*newService, c.serviceOwners(newService)); err != nil {
		logger.Errorf("failed to create (modify) smb %s. %+v", newService.Name, err)
	}
}

func (c *SMBController) onDelete(obj interface{}) {
	smb, err := getSMBObject(obj)
	if err != nil {
		logger.Errorf("failed to get smb object: %+v", err)
		return
	}

	if err = c.DeleteService(*smb); err != nil {
		logger.Errorf("failed to delete smb %s. %+v", smb.Name, err)
	}
}
func (c *SMBController) ParentClusterChanged(cluster edgefsv1.ClusterSpec) {
	ctx := context.TODO()
	if c.rookImage == cluster.EdgefsImageName {
		logger.Infof("No need to update the smb service, the same images present")
		return
	}

	// update controller options by updated cluster spec
	c.rookImage = cluster.EdgefsImageName

	smbes, err := c.context.RookClientset.EdgefsV1().SMBs(c.namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		logger.Errorf("failed to retrieve SMBes to update the Edgefs version. %+v", err)
		return
	}
	for _, smb := range smbes.Items {
		logger.Infof("updating the Edgefs version for smb %s to %s", smb.Name, cluster.EdgefsImageName)
		err := c.UpdateService(smb, nil)
		if err != nil {
			logger.Errorf("failed to update smb %s. %+v", smb.Name, err)
		} else {
			logger.Infof("updated smb %s to Edgefs version %s", smb.Name, cluster.EdgefsImageName)
		}
	}
}

func (c *SMBController) serviceOwners(service *edgefsv1.SMB) []metav1.OwnerReference {
	// Only set the cluster crd as the owner of the SMB resources.
	// If the SMB crd is deleted, the operator will explicitly remove the SMB resources.
	// If the SMB crd still exists when the cluster crd is deleted, this will make sure the SMB
	// resources are cleaned up.
	return []metav1.OwnerReference{c.ownerRef}
}

func serviceChanged(oldService, newService edgefsv1.SMBSpec) bool {

	var diff string
	// any change in the crd will trigger an orchestration
	if !reflect.DeepEqual(oldService, newService) {
		func() {
			defer func() {
				if err := recover(); err != nil {
					logger.Warningf("Encountered an issue getting smb service change differences: %v", err)
				}
			}()

			// resource.Quantity has non-exportable fields, so we use its comparator method
			resourceQtyComparer := cmp.Comparer(func(x, y resource.Quantity) bool { return x.Cmp(y) == 0 })
			diff = cmp.Diff(oldService, newService, resourceQtyComparer)
			logger.Infof("The SMB Service has changed. diff=%s", diff)
		}()
	}

	if len(diff) > 0 {
		return true
	}

	return false
}

func getSMBObject(obj interface{}) (smb *edgefsv1.SMB, err error) {
	var ok bool
	smb, ok = obj.(*edgefsv1.SMB)
	if ok {
		// the smb object is of the latest type, simply return it
		return smb.DeepCopy(), nil
	}

	return nil, fmt.Errorf("not a known smb object: %+v", obj)
}
