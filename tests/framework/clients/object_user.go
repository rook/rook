/*
Copyright 2018 The Rook Authors. All rights reserved.

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
	"github.com/rook/rook/pkg/daemon/ceph/rgw"
	"github.com/rook/rook/tests/framework/installer"
	"github.com/rook/rook/tests/framework/utils"
)

// ObjectUserOperation is wrapper for k8s rook object user operations
type ObjectUserOperation struct {
	k8sh      *utils.K8sHelper
	manifests installer.CephManifests
}

// CreateObjectUserOperation creates new rook object user client
func CreateObjectUserOperation(k8sh *utils.K8sHelper, manifests installer.CephManifests) *ObjectUserOperation {
	return &ObjectUserOperation{k8sh, manifests}
}

// ObjectUserGet Function to get the details of an object user from radosgw
func (o *ObjectUserOperation) GetUser(namespace string, store string, userid string) (*rgw.ObjectUser, error) {
	context := o.k8sh.MakeContext()
	rgwcontext := rgw.NewContext(context, store, namespace)
	userinfo, _, err := rgw.GetUser(rgwcontext, userid)
	if err != nil {
		return nil, fmt.Errorf("failed to get user info: %+v", err)
	}
	return userinfo, nil
}

// UserSecretExists Function to check that user secret was created
func (o *ObjectUserOperation) UserSecretExists(namespace string, store string, userid string) bool {
	_, err := o.k8sh.GetResource("-n", namespace, "secrets", "-l", "rook_object_store="+store, "-l", "user="+userid)
	if err == nil {
		logger.Infof("Object User Secret Exists")
		return true
	}
	logger.Infof("Unable to find user secret")
	return false
}

// ObjectUserCreate Function to create a object store user in rook
func (o *ObjectUserOperation) Create(namespace string, userid string, displayName string, store string) error {

	logger.Infof("creating the object store user via CRD")
	if _, err := o.k8sh.ResourceOperation("create", o.manifests.GetObjectStoreUser(namespace, userid, displayName, store)); err != nil {
		return err
	}
	return nil
}

func (o *ObjectUserOperation) Delete(namespace string, userid string) error {

	logger.Infof("Deleting the object store user via CRD")
	if _, err := o.k8sh.DeleteResource("-n", namespace, "ObjectStoreUser", userid); err != nil {
		return err
	}
	return nil
}
