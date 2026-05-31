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

const rgwPort = 80

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
func (o *ObjectOperation) Create(namespace, storeName string, replicaCount int32, tlsEnable bool, swiftAndKeystone bool) error {
	logger.Info("creating the object store via CRD")

	if err := o.k8sh.ResourceOperation("apply", o.manifests.GetObjectStore(storeName, int(replicaCount), rgwPort, tlsEnable, swiftAndKeystone)); err != nil {
		return err
	}

	// Starting an object store takes longer than the average operation, so add more retries
	err := o.k8sh.WaitForLabeledPodsToRunWithRetries(fmt.Sprintf("rook_object_store=%s", storeName), namespace, 80)
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

// Need to improve the below function for better error handling
func (o *ObjectOperation) GetEndPointUrl(namespace string, storeName string) (string, error) {
	args := []string{"get", "svc", "-n", namespace, "-l", fmt.Sprintf("rgw=%s", storeName), "-o", "jsonpath={.items[*].spec.clusterIP}"}
	EndPointUrl, err := o.k8sh.Kubectl(args...)
	if err != nil {
		return "", fmt.Errorf("unable to find rgw end point-- %s", err)
	}
	return fmt.Sprintf("%s:%d", EndPointUrl, rgwPort), nil
}
