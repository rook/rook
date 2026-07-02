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
	bktv1alpha1 "github.com/kube-object-storage/lib-bucket-provisioner/pkg/apis/objectbucket.io/v1alpha1"
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

func (b *BucketOperation) CreateBucketStorageClass(namespace string, storeName string, storageClassName string, reclaimPolicy string) error {
	return b.k8sh.ResourceOperation("create", b.manifests.GetBucketStorageClass(storeName, storageClassName, reclaimPolicy))
}

func (b *BucketOperation) DeleteBucketStorageClass(namespace string, storeName string, storageClassName string, reclaimPolicy string) error {
	err := b.k8sh.ResourceOperation("delete", b.manifests.GetBucketStorageClass(storeName, storageClassName, reclaimPolicy))
	return err
}

func (b *BucketOperation) CreateObc(obcName string, storageClassName string, bucketName string, maxObject string, createBucket bool) error {
	return b.k8sh.ResourceOperation("create", b.manifests.GetOBC(obcName, storageClassName, bucketName, maxObject, createBucket))
}

func (b *BucketOperation) DeleteObc(obcName string, storageClassName string, bucketName string, maxObject string, createBucket bool) error {
	return b.k8sh.ResourceOperation("delete", b.manifests.GetOBC(obcName, storageClassName, bucketName, maxObject, createBucket))
}

// CheckOBC, returns true if the obc, secret and configmap are all in the "check" state,
// and returns false if any of these resources are not in the "check" state.
// Check state values:
//
//	"created", all must exist,
//	"bound", all must exist and OBC in Bound phase
//	"deleted", all must be missing.
func (b *BucketOperation) CheckOBC(obcName, check string) bool {
	resources := []string{"obc", "secret", "configmap"}
	shouldBeBound := (check == "bound")
	shouldExist := (shouldBeBound || check == "created") // bound implies created

	for _, res := range resources {
		_, err := b.k8sh.GetResource(res, obcName)
		// note: we assume a `GetResource` error is a missing resource
		if shouldExist == (err != nil) {
			return false
		}
		logger.Infof("%s %s %s", res, obcName, check)
	}
	logger.Infof("%s resources %v all %s", obcName, resources, check)

	if shouldBeBound {
		// OBC should be in bound phase as well as existing
		state, _ := b.k8sh.GetResource("obc", obcName, "--output", "jsonpath={.status.phase}")
		boundPhase := bktv1alpha1.ObjectBucketClaimStatusPhaseBound // i.e., "Bound"
		if state != boundPhase {
			logger.Infof(`resources exist, but OBC is not in %q phase: %q`, boundPhase, state)
			return false
		}

		// Regression test: OBC should have spec.objectBucketName set
		obName, _ := b.k8sh.GetResource("obc", obcName, "--output", "jsonpath={.spec.objectBucketName}")
		if obName == "" {
			logger.Error("failed regression: OBC spec.objectBucketName is not set")
			return false
		}
		// Regression test: OB should have claim ref to OBC
		refName, _ := b.k8sh.GetResource("ob", obName, "--output", "jsonpath={.spec.claimRef.name}")
		if refName != obcName {
			logger.Errorf("failed regression: OB spec.claimRef.name (%q) does not match expected OBC name (%q)", refName, obcName)
			return false
		}

		logger.Infof("OBC is %q", boundPhase)
	}

	return true
}
