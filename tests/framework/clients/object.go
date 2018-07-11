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
	"github.com/rook/rook/tests/framework/utils"
)

const rgwPort = 53390

var logger = capnslog.NewPackageLogger("github.com/rook/rook/tests", "clients")

// ObjectOperation is wrapper for k8s rook object operations
type ObjectOperation struct {
	k8sh *utils.K8sHelper
}

// CreateObjectOperation creates new rook object client
func CreateObjectOperation(k8sh *utils.K8sHelper) *ObjectOperation {
	return &ObjectOperation{k8sh: k8sh}
}

// ObjectCreate Function to create a object store in rook
func (ro *ObjectOperation) Create(namespace, storeName string, replicaCount int32) error {

	logger.Infof("creating the object store via CRD")
	storeSpec := fmt.Sprintf(`apiVersion: ceph.rook.io/v1beta1
kind: ObjectStore
metadata:
  name: %s
  namespace: %s
spec:
  metadataPool:
    replicated:
      size: 1
  dataPool:
    replicated:
      size: 1
  gateway:
    type: s3
    sslCertificateRef: 
    port: %d
    securePort:
    instances: %d
    allNodes: false
`, storeName, namespace, rgwPort, replicaCount)

	if _, err := ro.k8sh.ResourceOperation("create", storeSpec); err != nil {
		return err
	}

	err := ro.k8sh.WaitForLabeledPodToRun(fmt.Sprintf("rook_object_store=%s", storeName), namespace)
	if err != nil {
		return fmt.Errorf("rgw did not start via crd. %+v", err)
	}

	// create the external service
	return ro.k8sh.CreateExternalRGWService(namespace, storeName)
}

func (ro *ObjectOperation) Delete(namespace, storeName string) error {

	logger.Infof("Deleting the object store via CRD")
	if _, err := ro.k8sh.DeleteResource("-n", namespace, "ObjectStore", storeName); err != nil {
		return err
	}

	if !ro.k8sh.WaitUntilPodWithLabelDeleted(fmt.Sprintf("rook_object_store=%s", storeName), namespace) {
		return fmt.Errorf("rgw did not stop via crd")
	}
	return nil
}
