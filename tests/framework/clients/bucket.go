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

	"github.com/aws/aws-sdk-go/service/s3"
	bktv1alpha1 "github.com/kube-object-storage/lib-bucket-provisioner/pkg/apis/objectbucket.io/v1alpha1"
	rgw "github.com/rook/rook/pkg/operator/ceph/object"
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

func (b *BucketOperation) CreateObcNotification(obcName string, storageClassName string, bucketName string, notification string, createBucket bool) error {
	return b.k8sh.ResourceOperation("create", b.manifests.GetOBCNotification(obcName, storageClassName, bucketName, notification, createBucket))
}

func (b *BucketOperation) DeleteObc(obcName string, storageClassName string, bucketName string, maxObject string, createBucket bool) error {
	return b.k8sh.ResourceOperation("delete", b.manifests.GetOBC(obcName, storageClassName, bucketName, maxObject, createBucket))
}

func (b *BucketOperation) UpdateObc(obcName string, storageClassName string, bucketName string, maxObject string, createBucket bool) error {
	return b.k8sh.ResourceOperation("apply", b.manifests.GetOBC(obcName, storageClassName, bucketName, maxObject, createBucket))
}

func (b *BucketOperation) UpdateObcNotificationAdd(obcName string, storageClassName string, bucketName string, notification string, createBucket bool) error {
	return b.k8sh.ResourceOperation("apply", b.manifests.GetOBCNotification(obcName, storageClassName, bucketName, notification, createBucket))
}

func (b *BucketOperation) UpdateObcNotificationRemove(obcName string, storageClassName string, bucketName string, maxObject string, createBucket bool) error {
	return b.k8sh.ResourceOperation("apply", b.manifests.GetOBC(obcName, storageClassName, bucketName, maxObject, createBucket))
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

// Fetch SecretKey, AccessKey for s3 client.
func (b *BucketOperation) GetAccessKey(obcName string) (string, error) {
	args := []string{"get", "secret", obcName, "-o", "jsonpath={@.data.AWS_ACCESS_KEY_ID}"}
	AccessKey, err := b.k8sh.Kubectl(args...)
	if err != nil {
		return "", fmt.Errorf("unable to find access key -- %s", err)
	}
	decode, _ := b64.StdEncoding.DecodeString(AccessKey)
	return string(decode), nil
}

func (b *BucketOperation) GetSecretKey(obcName string) (string, error) {
	args := []string{"get", "secret", obcName, "-o", "jsonpath={@.data.AWS_SECRET_ACCESS_KEY}"}
	SecretKey, err := b.k8sh.Kubectl(args...)
	if err != nil {
		return "", fmt.Errorf("unable to find secret key-- %s", err)
	}
	decode, _ := b64.StdEncoding.DecodeString(SecretKey)
	return string(decode), nil
}

// Checks whether MaxObject is updated for ob
func (b *BucketOperation) CheckOBMaxObject(obcName, maxobject string) bool {
	obName, _ := b.k8sh.GetResource("obc", obcName, "--output", "jsonpath={.spec.objectBucketName}")
	fetchMaxObject, _ := b.k8sh.GetResource("ob", obName, "--output", "jsonpath={.spec.endpoint.additionalConfig.maxObjects}")
	return maxobject == fetchMaxObject
}

// Checks the bucket notifications set on RGW backend bucket
func (b *BucketOperation) CheckBucketNotificationSetonRGW(namespace, storeName, obcName, bucketname, notificationName string, helper *TestClient, tlsEnabled bool) bool {
	var s3client *rgw.S3Agent
	var err error
	s3endpoint, _ := helper.ObjectClient.GetEndPointUrl(namespace, storeName)
	s3AccessKey, _ := helper.BucketClient.GetAccessKey(obcName)
	s3SecretKey, _ := helper.BucketClient.GetSecretKey(obcName)
	s3client, err = rgw.NewS3Agent(s3AccessKey, s3SecretKey, s3endpoint, true, nil, tlsEnabled, nil)
	if err != nil {
		logger.Infof("failed to s3client due to %v", err)
		return false
	}
	logger.Infof("endpoint (%s) Accesskey (%s) secret (%s)", s3endpoint, s3AccessKey, s3SecretKey)
	notifications, err := s3client.Client.GetBucketNotificationConfiguration(&s3.GetBucketNotificationConfigurationRequest{
		Bucket: &bucketname,
	})
	if err != nil {
		logger.Infof("failed to fetch bucket notifications configuration due to %v", err)
		return false
	}
	logger.Infof("%d bucket notifications found in: %+v", len(notifications.TopicConfigurations), notifications)
	for _, notification := range notifications.TopicConfigurations {
		if *notification.Id == notificationName {
			return true
		}
		logger.Infof("bucket notifications name mismatch %q != %q", *notification.Id, notificationName)
	}
	return false
}
