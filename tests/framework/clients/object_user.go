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
	"context"
	"fmt"
	"strings"

	"github.com/rook/rook/pkg/daemon/ceph/client"
	rgw "github.com/rook/rook/pkg/operator/ceph/object"
	"github.com/rook/rook/tests/framework/installer"
	"github.com/rook/rook/tests/framework/utils"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
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
	ctx := o.k8sh.MakeContext()
	clusterInfo := client.AdminTestClusterInfo(namespace)
	objectStore, err := o.k8sh.RookClientset.CephV1().CephObjectStores(namespace).Get(context.TODO(), store, metav1.GetOptions{})
	if err != nil {
		return nil, fmt.Errorf("failed to get objectstore info: %+v", err)
	}
	rgwcontext, err := rgw.NewMultisiteContext(ctx, clusterInfo, objectStore)
	if err != nil {
		return nil, fmt.Errorf("failed to get RGW context: %+v", err)
	}
	userinfo, _, err := rgw.GetUser(rgwcontext, userid)
	if err != nil {
		return nil, fmt.Errorf("failed to get user info: %+v", err)
	}
	return userinfo, nil
}

// UserSecretExists Function to check that user secret was created
func (o *ObjectUserOperation) UserSecretExists(namespace string, store string, userid string) bool {
	message, err := o.k8sh.GetResource("-n", namespace, "secrets", "-l", "rook_object_store="+store, "-l", "user="+userid)
	//GetResource(blah) returns success if blah is or is not found.
	//err = success and found_sec not "No resources found." means it was found
	//err = success and found_sec contains "No resources found." means it was not found
	//err != success is another error
	if err == nil && !strings.Contains(message, "No resources found") {
		logger.Infof("Object User Secret Exists")
		return true
	}
	logger.Infof("Unable to find user secret")
	return false
}

// ObjectUserCreate Function to create a object store user in rook
func (o *ObjectUserOperation) Create(userid, displayName, store, usercaps, maxsize string, maxbuckets, maxobjects int) error {

	logger.Infof("creating the object store user via CRD")
	if err := o.k8sh.ResourceOperation("apply", o.manifests.GetObjectStoreUser(userid, displayName, store, usercaps, maxsize, maxbuckets, maxobjects)); err != nil {
		return err
	}
	return nil
}

func (o *ObjectUserOperation) Delete(namespace string, userid string) error {

	logger.Infof("Deleting the object store user via CRD")
	if err := o.k8sh.DeleteResource("-n", namespace, "CephObjectStoreUser", userid); err != nil {
		return err
	}
	return nil
}
