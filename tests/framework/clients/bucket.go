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
	b64 "encoding/base64"
	"fmt"

	"github.com/rook/rook/tests/framework/installer"
	"github.com/rook/rook/tests/framework/utils"
)

// BucketOperation is a wrapper for rook bucket operations
type BucketOperation struct {
	k8sh      *utils.K8sHelper
	manifests installer.CephManifests
}

// CreateBucketOperation creates a new bucket client
func CreateBucketOperation(k8sh *utils.K8sHelper, manifests installer.CephManifests) *BucketOperation {
	return &BucketOperation{k8sh, manifests}
}

func (b *BucketOperation) CreateBucketStorageClass(namespace string, storeName string, storageClassName string, reclaimPolicy string, region string) error {
	return b.k8sh.ResourceOperation("create", b.manifests.GetBucketStorageClass(namespace, storeName, storageClassName, reclaimPolicy, region))
}

func (b *BucketOperation) DeleteBucketStorageClass(namespace string, storeName string, storageClassName string, reclaimPolicy string, region string) error {
	err := b.k8sh.ResourceOperation("delete", b.manifests.GetBucketStorageClass(namespace, storeName, storageClassName, reclaimPolicy, region))
	return err
}

func (b *BucketOperation) CreateObc(obcName string, storageClassName string, bucketName string, maxObject string, createBucket bool) error {
	return b.k8sh.ResourceOperation("create", b.manifests.GetObc(obcName, storageClassName, bucketName, maxObject, createBucket))
}

func (b *BucketOperation) DeleteObc(obcName string, storageClassName string, bucketName string, maxObject string, createBucket bool) error {
	return b.k8sh.ResourceOperation("delete", b.manifests.GetObc(obcName, storageClassName, bucketName, maxObject, createBucket))
}

// CheckOBC, returns true if the obc, secret and configmap are all in the "check" state,
// and returns false if any of these resources are not in the "check" state.
// Check state values:
//   "created", all must exist,
//   "deleted", all must be missing.
func (b *BucketOperation) CheckOBC(obcName, check string) bool {
	resources := []string{"obc", "secret", "configmap"}
	shouldExist := (check == "created")

	for _, res := range resources {
		_, err := b.k8sh.GetResource(res, obcName)
		// note: we assume a `GetResource` error is a missing resource
		if shouldExist == (err != nil) {
			return false
		}
		logger.Infof("%s %s %s", res, obcName, check)
	}
	logger.Infof("%s resources %v all %s", obcName, resources, check)

	return true
}

// Fetch SecretKey, AccessKey for s3 client.
func (b *BucketOperation) GetAccessKey(obcName string) (string, error) {
	args := []string{"get", "secret", obcName, "-o", "jsonpath={@.data.AWS_ACCESS_KEY_ID}"}
	AccessKey, err := b.k8sh.Kubectl(args...)
	if err != nil {
		return "", fmt.Errorf("Unable to find access key -- %s", err)
	}
	decode, _ := b64.StdEncoding.DecodeString(AccessKey)
	return string(decode), nil
}

func (b *BucketOperation) GetSecretKey(obcName string) (string, error) {
	args := []string{"get", "secret", obcName, "-o", "jsonpath={@.data.AWS_SECRET_ACCESS_KEY}"}
	SecretKey, err := b.k8sh.Kubectl(args...)
	if err != nil {
		return "", fmt.Errorf("Unable to find secret key-- %s", err)
	}
	decode, _ := b64.StdEncoding.DecodeString(SecretKey)
	return string(decode), nil

}
