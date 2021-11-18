/*
Copyright 2021 The Rook Authors. All rights reserved.

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

package integration

import (
	"context"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go/service/s3"
	"github.com/rook/rook/pkg/daemon/ceph/client"
	rgw "github.com/rook/rook/pkg/operator/ceph/object"
	"github.com/rook/rook/tests/framework/utils"
	"github.com/stretchr/testify/assert"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func (s *ObjectSuite) TestBucketNotificationsInOrder() {
	if utils.IsPlatformOpenShift() {
		s.T().Skip("bucket notification tests skipped on openshift")
	}

	objectStoreServicePrefix = objectStoreServicePrefixUniq
	storeName := "test-store"
	tlsEnable := false
	namespace := s.settings.Namespace
	helper := s.helper
	k8sh := s.k8sh
	logger.Infof("Running on Rook Cluster %s", namespace)
	createCephObjectStore(s.T(), helper, k8sh, namespace, storeName, 3, tlsEnable)

	ctx := context.TODO()
	clusterInfo := client.AdminTestClusterInfo(namespace)
	t := s.T()

	t.Run("create CephObjectStoreUser", func(t *testing.T) {
		createCephObjectUser(s.Suite, helper, k8sh, namespace, storeName, userid, true, true)
		i := 0
		for i = 0; i < 4; i++ {
			if helper.ObjectUserClient.UserSecretExists(namespace, storeName, userid) {
				break
			}
			logger.Info("waiting 5 more seconds for user secret to exist")
			time.Sleep(5 * time.Second)
		}
		assert.NotEqual(t, 4, i)
	})
	logger.Info("object store user created")

	context := k8sh.MakeContext()
	objectStore, err := k8sh.RookClientset.CephV1().CephObjectStores(namespace).Get(ctx, storeName, metav1.GetOptions{})
	assert.Nil(t, err)
	rgwcontext, err := rgw.NewMultisiteContext(context, clusterInfo, objectStore)
	assert.Nil(t, err)

	notificationName := "my-notification"
	topicName := "my-topic"
	httpEndpointService := "my-notification-sink"

	t.Run("create CephBucketTopic", func(t *testing.T) {
		err := helper.TopicClient.CreateTopic(topicName, storeName, httpEndpointService)
		assert.Nil(t, err)
		created := utils.Retry(12, 2*time.Second, "topic is created", func() bool {
			return helper.TopicClient.CheckTopic(topicName)
		})
		assert.True(t, created)
		logger.Info("CephBucketTopic created successfully")
	})

	t.Run("create CephBucketNotification", func(t *testing.T) {
		err := helper.NotificationClient.CreateNotification(notificationName, topicName)
		assert.Nil(t, err)
		created := utils.Retry(12, 2*time.Second, "notification is created", func() bool {
			return helper.NotificationClient.CheckNotification(notificationName)
		})
		assert.True(t, created)
		logger.Info("CephBucketNotification created successfully")
	})

	t.Run("create ObjectBucketClaim", func(t *testing.T) {
		logger.Infof("create OBC %q with storageclass %q and notification %q", obcName, bucketStorageClassName, notificationName)
		cobErr := helper.BucketClient.CreateBucketStorageClass(namespace, storeName, bucketStorageClassName, "Delete", region)
		assert.Nil(t, cobErr)
		cobcErr := helper.BucketClient.CreateObcNotification(obcName, bucketStorageClassName, bucketname, notificationName, true)
		assert.Nil(t, cobcErr)

		created := utils.Retry(12, 2*time.Second, "OBC is created", func() bool {
			return helper.BucketClient.CheckOBC(obcName, "bound")
		})
		assert.True(t, created)
		logger.Info("OBC created successfully")

		var bkt rgw.ObjectBucket
		i := 0
		for i = 0; i < 4; i++ {
			b, code, err := rgw.GetBucket(rgwcontext, bucketname)
			if b != nil && err == nil {
				bkt = *b
				break
			}
			logger.Warningf("cannot get bucket %q, retrying... bucket: %v. code: %d, err: %v", bucketname, b, code, err)
			logger.Infof("(%d) check bucket exists, sleeping for 5 seconds ...", i)
			time.Sleep(5 * time.Second)
		}
		assert.NotEqual(t, 4, i)
		assert.Equal(t, bucketname, bkt.Name)
		logger.Info("OBC, Secret and ConfigMap created")
	})

	t.Run("check CephBucketNotification created for bucket", func(t *testing.T) {
		var s3client *rgw.S3Agent
		s3endpoint, _ := helper.ObjectClient.GetEndPointUrl(namespace, storeName)
		s3AccessKey, _ := helper.BucketClient.GetAccessKey(obcName)
		s3SecretKey, _ := helper.BucketClient.GetSecretKey(obcName)
		if objectStore.Spec.IsTLSEnabled() {
			s3client, err = rgw.NewInsecureS3Agent(s3AccessKey, s3SecretKey, s3endpoint, rgwcontext.ZoneGroup, true)
		} else {
			s3client, err = rgw.NewS3Agent(s3AccessKey, s3SecretKey, s3endpoint, rgwcontext.ZoneGroup, true, nil)
		}

		assert.Nil(t, err)
		logger.Infof("endpoint (%s) Accesskey (%s) secret (%s) region (%s)", s3endpoint, s3AccessKey, s3SecretKey, rgwcontext.ZoneGroup)

		created := utils.Retry(12, 2*time.Second, "notification is created for bucket", func() bool {
			notifications, err := s3client.Client.GetBucketNotificationConfiguration(&s3.GetBucketNotificationConfigurationRequest{
				Bucket: &bucketname,
			})
			if err != nil {
				logger.Infof("failed to fetch bucket notifications configuration due to %v", err)
				return false
			}
			logger.Infof("%d bucket notifications found in: %v", len(notifications.TopicConfigurations), notifications)
			for _, notification := range notifications.TopicConfigurations {
				if *notification.Id == notificationName {
					return true
				}
				logger.Infof("bucket notifications name mismatch %q != %q", *notification.Id, notificationName)
			}
			return false
		})
		assert.True(t, created)
		logger.Info("CephBucketNotification created successfully on bucket")
	})

	t.Run("delete ObjectBucketClaim", func(t *testing.T) {
		i := 0
		dobcErr := helper.BucketClient.DeleteObc(obcName, bucketStorageClassName, bucketname, maxObject, true)
		assert.Nil(t, dobcErr)
		logger.Info("Checking to see if the obc, secret, and cm have all been deleted")
		for i = 0; i < 4 && !helper.BucketClient.CheckOBC(obcName, "deleted"); i++ {
			logger.Infof("(%d) obc deleted check, sleeping for 5 seconds ...", i)
			time.Sleep(5 * time.Second)
		}
		assert.NotEqual(t, 4, i)

		logger.Info("ensure OBC bucket was deleted")
		var rgwErr int
		for i = 0; i < 4; i++ {
			_, rgwErr, _ = rgw.GetBucket(rgwcontext, bucketname)
			if rgwErr == rgw.RGWErrorNotFound {
				break
			}
			logger.Infof("(%d) check bucket deleted, sleeping for 5 seconds ...", i)
			time.Sleep(5 * time.Second)
		}
		assert.NotEqual(t, 4, i)
		assert.Equal(t, rgwErr, rgw.RGWErrorNotFound)

		dobErr := helper.BucketClient.DeleteBucketStorageClass(namespace, storeName, bucketStorageClassName, "Delete", region)
		assert.Nil(t, dobErr)
	})

	t.Run("delete CephBucketNotification", func(t *testing.T) {
		err := helper.NotificationClient.DeleteNotification(notificationName, topicName)
		assert.Nil(t, err)
	})

	t.Run("delete CephBucketTopic", func(t *testing.T) {
		err := helper.TopicClient.DeleteTopic(topicName, storeName, httpEndpointService)
		assert.Nil(t, err)
	})

	t.Run("delete CephObjectStoreUser", func(t *testing.T) {
		dosuErr := helper.ObjectUserClient.Delete(namespace, userid)
		assert.Nil(t, dosuErr)
		logger.Info("Object store user deleted successfully")
		logger.Info("Checking to see if the user secret has been deleted")
		i := 0
		for i = 0; i < 4 && helper.ObjectUserClient.UserSecretExists(namespace, storeName, userid) == true; i++ {
			logger.Infof("(%d) secret check sleeping for 5 seconds ...", i)
			time.Sleep(5 * time.Second)
		}
		assert.False(t, helper.ObjectUserClient.UserSecretExists(namespace, storeName, userid))
	})

	t.Run("delete CephObjectStore", func(t *testing.T) {
		deleteObjectStore(t, k8sh, namespace, storeName)
		assertObjectStoreDeletion(t, k8sh, namespace, storeName)
	})
}
