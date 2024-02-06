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

	"github.com/rook/rook/pkg/daemon/ceph/client"
	rgw "github.com/rook/rook/pkg/operator/ceph/object"
	"github.com/rook/rook/tests/framework/clients"
	"github.com/rook/rook/tests/framework/utils"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/suite"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	httpServerName      = "sample-http-server"
	httpServerNameSpace = "default"
	httpServerPort      = "8080"
	httpEndpointService = "http://" + httpServerName + "." + httpServerNameSpace + ":" + httpServerPort
	putEvent            = "ObjectCreated:Put"
	deleteEvent         = "ObjectRemoved:Delete"
)

func testBucketNotifications(s *suite.Suite, helper *clients.TestClient, k8sh *utils.K8sHelper, namespace, storeName string) {
	if utils.IsPlatformOpenShift() {
		s.T().Skip("bucket notification tests skipped on openshift")
	}

	bucketNotificationLabelPrefix := "bucket-notification-"
	obcNamespace := "default"

	ctx := context.TODO()
	clusterInfo := client.AdminTestClusterInfo(namespace)
	t := s.T()

	context := k8sh.MakeContext()
	objectStore, err := k8sh.RookClientset.CephV1().CephObjectStores(namespace).Get(ctx, storeName, metav1.GetOptions{})
	assert.Nil(t, err)
	rgwcontext, err := rgw.NewMultisiteContext(context, clusterInfo, objectStore)
	assert.Nil(t, err)

	notificationName := "my-notification"
	topicName := "my-topic"
	appLabel := "app=" + httpServerName

	logger.Infof("Testing Bucket Notifications on %s", storeName)

	t.Run("create HTTP Endpoint for receiving notifications", func(t *testing.T) {
		err := helper.TopicClient.CreateHTTPServer(httpServerName, httpServerNameSpace, httpServerPort)
		assert.Nil(t, err)
	})

	t.Run("create CephBucketTopic", func(t *testing.T) {
		err := helper.TopicClient.CreateTopic(topicName, storeName, httpEndpointService)
		assert.Nil(t, err)
		created := utils.Retry(30, 5*time.Second, "topic is created", func() bool {
			return helper.TopicClient.CheckTopic(topicName)
		})
		assert.True(t, created)
		logger.Info("CephBucketTopic created successfully")
	})

	t.Run("create CephBucketNotification", func(t *testing.T) {
		err := helper.NotificationClient.CreateNotification(notificationName, topicName)
		assert.Nil(t, err)
		created := utils.Retry(12, 2*time.Second, "notification is created", func() bool {
			return helper.NotificationClient.CheckNotificationCR(notificationName)
		})
		assert.True(t, created)
		logger.Info("CephBucketNotification created successfully")
	})

	t.Run("create ObjectBucketClaim", func(t *testing.T) {
		logger.Infof("create OBC %q with storageclass %q and notification %q", obcName, bucketStorageClassName, notificationName)
		cobErr := helper.BucketClient.CreateBucketStorageClass(namespace, storeName, bucketStorageClassName, "Delete")
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

	t.Run("perform s3 operations and check for notifications", func(t *testing.T) {
		var s3client *rgw.S3Agent
		s3endpoint, _ := helper.ObjectClient.GetEndPointUrl(namespace, storeName)
		s3AccessKey, _ := helper.BucketClient.GetAccessKey(obcName)
		s3SecretKey, _ := helper.BucketClient.GetSecretKey(obcName)
		if objectStore.Spec.IsTLSEnabled() {
			s3client, err = rgw.NewInsecureS3Agent(s3AccessKey, s3SecretKey, s3endpoint, true)
		} else {
			s3client, err = rgw.NewS3Agent(s3AccessKey, s3SecretKey, s3endpoint, true, nil)
		}

		assert.Nil(t, err)
		logger.Infof("endpoint (%s) Accesskey (%s) secret (%s)", s3endpoint, s3AccessKey, s3SecretKey)

		t.Run("put object", func(t *testing.T) {
			_, err := s3client.PutObjectInBucket(bucketname, ObjBody, ObjectKey1, contentType)
			assert.Nil(t, err)
		})

		t.Run("check for put bucket notification", func(t *testing.T) {
			notificationReceived, err := helper.NotificationClient.CheckNotificationFromHTTPEndPoint(appLabel, putEvent, ObjectKey1)
			assert.True(t, notificationReceived)
			assert.Nil(t, err)
			// negative test case to confirm didn't receive any delete event notification
			notificationReceived, err = helper.NotificationClient.CheckNotificationFromHTTPEndPoint(appLabel, deleteEvent, ObjectKey1)
			assert.False(t, notificationReceived)
			assert.Nil(t, err)
		})

		t.Run("delete objects", func(t *testing.T) {
			_, err := s3client.DeleteObjectInBucket(bucketname, ObjectKey1)
			assert.Nil(t, err)
		})

		t.Run("check for delete bucket notification", func(t *testing.T) {
			notificationReceived, err := helper.NotificationClient.CheckNotificationFromHTTPEndPoint(appLabel, deleteEvent, ObjectKey1)
			assert.True(t, notificationReceived)
			assert.Nil(t, err)
			// negative test case to confirm didn't receive any put event notification
			notificationReceived, err = helper.NotificationClient.CheckNotificationFromHTTPEndPoint(appLabel, putEvent, ObjectKey1)
			assert.False(t, notificationReceived)
			assert.Nil(t, err)
		})

	})

	t.Run("check CephBucketNotification created for bucket", func(t *testing.T) {
		notificationPresent := utils.Retry(12, 2*time.Second, "notification is created for bucket", func() bool {
			return helper.BucketClient.CheckBucketNotificationSetonRGW(namespace, storeName, obcName, bucketname, notificationName, helper, objectStore.Spec.IsTLSEnabled())
		})
		assert.True(t, notificationPresent)
		logger.Info("CephBucketNotification created successfully on bucket")
	})

	t.Run("add non-notification label to OBC", func(t *testing.T) {
		obc, err := k8sh.BucketClientset.ObjectbucketV1alpha1().ObjectBucketClaims(obcNamespace).Get(ctx, obcName, metav1.GetOptions{})
		assert.Nil(t, err)
		obc.Labels["test-label"] = "test-value"
		_, err = k8sh.BucketClientset.ObjectbucketV1alpha1().ObjectBucketClaims(obcNamespace).Update(ctx, obc, metav1.UpdateOptions{})
		assert.Nil(t, err)
		// check whether existing bucket notification unaffected
		notificationPresent := utils.Retry(12, 2*time.Second, "notification is created for bucket", func() bool {
			// TODO : add api to fetch all the notification from backend to see if it is unaffected
			t.Skipped()
			return helper.BucketClient.CheckBucketNotificationSetonRGW(namespace, storeName, obcName, bucketname, notificationName, helper, objectStore.Spec.IsTLSEnabled())
		})
		assert.True(t, notificationPresent)
	})

	t.Run("remove non-notification label from OBC", func(t *testing.T) {
		obc, err := k8sh.BucketClientset.ObjectbucketV1alpha1().ObjectBucketClaims(obcNamespace).Get(ctx, obcName, metav1.GetOptions{})
		assert.Nil(t, err)
		delete(obc.Labels, "test-label")
		_, err = k8sh.BucketClientset.ObjectbucketV1alpha1().ObjectBucketClaims(obcNamespace).Update(ctx, obc, metav1.UpdateOptions{})
		assert.Nil(t, err)
		// check whether existing bucket notification unaffected
		notificationPresent := utils.Retry(12, 2*time.Second, "notification is created for bucket", func() bool {
			// TODO : add api to fetch all the notification from backend to see if it is unaffected
			t.Skipped()
			return helper.BucketClient.CheckBucketNotificationSetonRGW(namespace, storeName, obcName, bucketname, notificationName, helper, objectStore.Spec.IsTLSEnabled())
		})
		assert.True(t, notificationPresent)
	})

	t.Run("remove notification label from OBC", func(t *testing.T) {
		// TODO: add remove notification label support in OBC
		t.Skipped()
		obc, err := k8sh.BucketClientset.ObjectbucketV1alpha1().ObjectBucketClaims(obcNamespace).Get(ctx, obcName, metav1.GetOptions{})
		assert.Nil(t, err)
		delete(obc.Labels, bucketNotificationLabelPrefix+notificationName)
		_, err = k8sh.BucketClientset.ObjectbucketV1alpha1().ObjectBucketClaims(obcNamespace).Update(ctx, obc, metav1.UpdateOptions{})
		assert.Nil(t, err)
		// check whether existing bucket notification uneffected
		var notificationPresent bool
		for i := 0; i < 4; i++ {
			notificationPresent = helper.BucketClient.CheckBucketNotificationSetonRGW(namespace, storeName, obcName, bucketname, notificationName, helper, objectStore.Spec.IsTLSEnabled())
			if !notificationPresent {
				break
			}
			time.Sleep(5 * time.Second)
		}
		assert.False(t, notificationPresent)
	})

	t.Run("perform s3 operations and confirm notifications are no longer received", func(t *testing.T) {
		var s3client *rgw.S3Agent
		s3endpoint, _ := helper.ObjectClient.GetEndPointUrl(namespace, storeName)
		s3AccessKey, _ := helper.BucketClient.GetAccessKey(obcName)
		s3SecretKey, _ := helper.BucketClient.GetSecretKey(obcName)
		if objectStore.Spec.IsTLSEnabled() {
			s3client, err = rgw.NewInsecureS3Agent(s3AccessKey, s3SecretKey, s3endpoint, true)
		} else {
			s3client, err = rgw.NewS3Agent(s3AccessKey, s3SecretKey, s3endpoint, true, nil)
		}

		assert.Nil(t, err)
		logger.Infof("endpoint (%s) Accesskey (%s) secret (%s)", s3endpoint, s3AccessKey, s3SecretKey)

		t.Run("put object", func(t *testing.T) {
			_, err := s3client.PutObjectInBucket(bucketname, ObjBody, ObjectKey2, contentType)
			assert.Nil(t, err)
		})

		t.Run("check for put bucket notification", func(t *testing.T) {
			notificationReceived, err := helper.NotificationClient.CheckNotificationFromHTTPEndPoint(appLabel, putEvent, ObjectKey2)
			assert.False(t, notificationReceived)
			assert.Nil(t, err)
		})

		t.Run("delete objects", func(t *testing.T) {
			_, err := s3client.DeleteObjectInBucket(bucketname, ObjectKey1)
			assert.Nil(t, err)
		})

		t.Run("check for delete bucket notification", func(t *testing.T) {
			notificationReceived, err := helper.NotificationClient.CheckNotificationFromHTTPEndPoint(appLabel, deleteEvent, ObjectKey2)
			assert.False(t, notificationReceived)
			assert.Nil(t, err)

		})

	})

	t.Run("add topic, notification to existing OBC", func(t *testing.T) {
		newNotificationName := "new-notification"
		newTopicName := "new-topic"
		t.Run("create CephBucketTopic: new-topic", func(t *testing.T) {
			err := helper.TopicClient.CreateTopic(newTopicName, storeName, httpEndpointService)
			assert.Nil(t, err)
			created := utils.Retry(12, 2*time.Second, "topic is created", func() bool {
				return helper.TopicClient.CheckTopic(newTopicName)
			})
			assert.True(t, created)
		})
		t.Run("create CephBucketNotification: new-notification", func(t *testing.T) {
			err = helper.NotificationClient.CreateNotification(newNotificationName, newTopicName)
			assert.Nil(t, err)
			created := utils.Retry(12, 2*time.Second, "notification is created", func() bool {
				return helper.NotificationClient.CheckNotificationCR(newNotificationName)
			})
			assert.True(t, created)
		})
		t.Run("add notification label to OBC", func(t *testing.T) {
			obc, err := k8sh.BucketClientset.ObjectbucketV1alpha1().ObjectBucketClaims(obcNamespace).Get(ctx, obcName, metav1.GetOptions{})
			assert.Nil(t, err)
			obc.Labels[bucketNotificationLabelPrefix+newNotificationName] = newNotificationName
			_, err = k8sh.BucketClientset.ObjectbucketV1alpha1().ObjectBucketClaims(obcNamespace).Update(ctx, obc, metav1.UpdateOptions{})
			assert.Nil(t, err)
		})
		t.Run("new-notification should be configured for bucket", func(t *testing.T) {
			// check whether bucket notification added
			notificationPresent := utils.Retry(12, 2*time.Second, "notification is created for bucket", func() bool {
				return helper.BucketClient.CheckBucketNotificationSetonRGW(namespace, storeName, obcName, bucketname, newNotificationName, helper, objectStore.Spec.IsTLSEnabled())
			})
			assert.True(t, notificationPresent)
		})
		t.Run("delete CephBucketNotification: new-notification", func(t *testing.T) {
			err = helper.NotificationClient.DeleteNotification(newNotificationName, topicName)
			assert.Nil(t, err)
			t.Run("check notification removed from backend Bucket", func(t *testing.T) {
				// TODO: need to add that support
				t.Skipped()
			})
		})
		t.Run("delete CephBucketTopic: new-topic", func(t *testing.T) {
			err = helper.TopicClient.DeleteTopic(newTopicName, storeName, httpEndpointService)
			assert.Nil(t, err)
		})
	})

	t.Run("reverse order of creating notification,topic and adding it to ObjectBucketClaim", func(t *testing.T) {
		reverseNotificationName := "reverse-notification"
		reverseTopicName := "reverse-topic"
		reverseOBCName := "reverse-obc"
		reverseBucketName := "reverse-bucket"
		i := 0

		t.Run("create ObjectBucketClaim: reverse-obc", func(t *testing.T) {
			err := helper.BucketClient.CreateObcNotification(reverseOBCName, bucketStorageClassName, reverseBucketName, reverseNotificationName, true)
			assert.Nil(t, err)

			created := utils.Retry(12, 2*time.Second, "OBC is created", func() bool {
				return helper.BucketClient.CheckOBC(reverseOBCName, "bound")
			})
			assert.True(t, created)
			logger.Info("OBC created successfully")

			var bkt rgw.ObjectBucket
			for i = 0; i < 4; i++ {
				b, code, err := rgw.GetBucket(rgwcontext, reverseBucketName)
				if b != nil && err == nil {
					bkt = *b
					break
				}
				logger.Warningf("cannot get bucket %q, retrying... bucket: %v. code: %d, err: %v", reverseBucketName, b, code, err)
				logger.Infof("(%d) check bucket exists, sleeping for 5 seconds ...", i)
				time.Sleep(5 * time.Second)
			}
			assert.NotEqual(t, 4, i)
			assert.Equal(t, reverseBucketName, bkt.Name)
		})
		t.Run("create CephBucketNotification: reverse-notification", func(t *testing.T) {
			err = helper.NotificationClient.CreateNotification(reverseNotificationName, reverseTopicName)
			assert.Nil(t, err)
			created := utils.Retry(12, 2*time.Second, "notification is created", func() bool {
				return helper.NotificationClient.CheckNotificationCR(reverseNotificationName)
			})
			assert.True(t, created)
		})
		t.Run("the notification should not configured for the backend bucket until topic is created", func(t *testing.T) {
			// check whether bucket notification added, should fail since topic is not created
			// TODO: make below check valid with help of status field in NotificationCR
			t.Skipped()
		})
		t.Run("create CephBucketTopic: reverse-topic", func(t *testing.T) {
			err = helper.TopicClient.CreateTopic(reverseTopicName, storeName, httpEndpointService)
			assert.Nil(t, err)
			created := utils.Retry(12, 2*time.Second, "topic is created", func() bool {
				return helper.TopicClient.CheckTopic(reverseTopicName)
			})
			assert.True(t, created)
		})
		t.Run("notification should be configured after creating the topic", func(t *testing.T) {
			// check whether bucket notification added, should pass since topic got created
			notificationPresent := utils.Retry(12, 2*time.Second, "notification is created for bucket", func() bool {
				return helper.BucketClient.CheckBucketNotificationSetonRGW(namespace, storeName, reverseOBCName, reverseBucketName, reverseNotificationName, helper, objectStore.Spec.IsTLSEnabled())
			})
			assert.True(t, notificationPresent)
		})
		t.Run("delete CephBucketNotification: reverse-notification", func(t *testing.T) {
			err = helper.NotificationClient.DeleteNotification(reverseNotificationName, reverseTopicName)
			assert.Nil(t, err)
			t.Run("check notification removed from backend Bucket", func(t *testing.T) {
				// TODO: need to add that support
				t.Skipped()
			})
		})
		t.Run("delete CephBucketTopic: reverse-topic", func(t *testing.T) {
			err = helper.TopicClient.DeleteTopic(reverseTopicName, storeName, httpEndpointService)
			assert.Nil(t, err)
		})
		t.Run("delete ObjectBucketClaim: reverse-obc", func(t *testing.T) {
			err = helper.BucketClient.DeleteObc(reverseOBCName, bucketStorageClassName, reverseBucketName, maxObject, true)
			assert.Nil(t, err)
			logger.Info("Checking to see if the obc, secret, and cm have all been deleted")
			for i = 0; i < 4 && !helper.BucketClient.CheckOBC(reverseOBCName, "deleted"); i++ {
				logger.Infof("(%d) obc deleted check, sleeping for 5 seconds ...", i)
				time.Sleep(5 * time.Second)
			}
			assert.NotEqual(t, 4, i)
			logger.Info("ensure OBC bucket was deleted")
			var rgwErr int
			for i = 0; i < 4; i++ {
				_, rgwErr, _ = rgw.GetBucket(rgwcontext, reverseBucketName)
				if rgwErr == rgw.RGWErrorNotFound {
					break
				}
				logger.Infof("(%d) check bucket deleted, sleeping for 5 seconds ...", i)
				time.Sleep(5 * time.Second)
			}
			assert.NotEqual(t, 4, i)
			assert.Equal(t, rgwErr, rgw.RGWErrorNotFound)
		})
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

		dobErr := helper.BucketClient.DeleteBucketStorageClass(namespace, storeName, bucketStorageClassName, "Delete")
		assert.Nil(t, dobErr)
	})

	t.Run("delete CephBucketNotification", func(t *testing.T) {
		err := helper.NotificationClient.DeleteNotification(notificationName, topicName)
		assert.Nil(t, err)
		t.Run("check notification removed from backend Bucket", func(t *testing.T) {
			// TODO: need to add that support
			t.Skipped()
		})
	})

	t.Run("delete CephBucketTopic", func(t *testing.T) {
		err := helper.TopicClient.DeleteTopic(topicName, storeName, httpEndpointService)
		assert.Nil(t, err)
	})

	t.Run("delete CephObjectStore", func(t *testing.T) {
		deleteObjectStore(t, k8sh, namespace, storeName)
		assertObjectStoreDeletion(t, k8sh, namespace, storeName)
	})
}
