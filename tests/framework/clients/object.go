/*
Copyright 2016 The Rook Authors. All rights reserved.

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

package clients

import (
	"fmt"

	"github.com/coreos/pkg/capnslog"
	"github.com/rook/rook/tests/framework/installer"
	"github.com/rook/rook/tests/framework/utils"
)

const rgwPort = 53390

var logger = capnslog.NewPackageLogger("github.com/rook/rook/tests", "clients")

// ObjectOperation is wrapper for k8s rook object operations
type ObjectOperation struct {
	k8sh      *utils.K8sHelper
	manifests installer.CephManifests
}

// CreateObjectOperation creates new rook object client
func CreateObjectOperation(k8sh *utils.K8sHelper, manifests installer.CephManifests) *ObjectOperation {
	return &ObjectOperation{k8sh, manifests}
}

// ObjectCreate Function to create a object store in rook
func (o *ObjectOperation) Create(namespace, storeName string, replicaCount int32) error {

	logger.Infof("creating the object store via CRD")
	if err := o.k8sh.ResourceOperation("create", o.manifests.GetObjectStore(namespace, storeName, int(replicaCount), rgwPort)); err != nil {
		return err
	}

	// Starting an object store takes longer than the average operation, so add more retries
	err := o.k8sh.WaitForLabeledPodsToRunWithRetries(fmt.Sprintf("rook_object_store=%s", storeName), namespace, 40)
	if err != nil {
		return fmt.Errorf("rgw did not start via crd. %+v", err)
	}

	// create the external service
	return o.k8sh.CreateExternalRGWService(namespace, storeName)
}

func (o *ObjectOperation) Delete(namespace, storeName string) error {

	logger.Infof("Deleting the object store via CRD")
	if err := o.k8sh.DeleteResource("-n", namespace, "CephObjectStore", storeName); err != nil {
		return err
	}

	if !o.k8sh.WaitUntilPodWithLabelDeleted(fmt.Sprintf("rook_object_store=%s", storeName), namespace) {
		return fmt.Errorf("rgw did not stop via crd")
	}
	return nil
}
