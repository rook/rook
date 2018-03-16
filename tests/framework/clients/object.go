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

	kind := "ObjectStore"
	if !ro.k8sh.VersionAtLeast("1.7.0") {
		kind = "Objectstore"
	}
	logger.Infof("creating the object store via CRD")
	storeSpec := fmt.Sprintf(`apiVersion: rook.io/v1alpha1
kind: %s
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
`, kind, storeName, namespace, rgwPort, replicaCount)

	if _, err := ro.k8sh.ResourceOperation("create", storeSpec); err != nil {
		return err
	}

	if !ro.k8sh.IsPodWithLabelRunning(fmt.Sprintf("rook_object_store=%s", storeName), namespace) {
		return fmt.Errorf("rgw did not start via crd")
	}

	// create the external service
	return ro.k8sh.CreateExternalRGWService(namespace, storeName)
}

func (ro *ObjectOperation) Delete(namespace, storeName string, replicaCount int32) error {

	if !ro.k8sh.VersionAtLeast("1.7.0") {
		// The operator fails to process the Objectstore TPR upon deletion. Support for 1.6 is going away, so just disable this test in that case.
		return nil
	}
	logger.Infof("Deleting the object store via CRD")
	storeSpec := fmt.Sprintf(`apiVersion: rook.io/v1alpha1
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

	if _, err := ro.k8sh.ResourceOperation("delete", storeSpec); err != nil {
		return err
	}

	if !ro.k8sh.WaitUntilPodWithLabelDeleted(fmt.Sprintf("rook_object_store=%s", storeName), namespace) {
		return fmt.Errorf("rgw did not stop via crd")
	}
	return nil
}
