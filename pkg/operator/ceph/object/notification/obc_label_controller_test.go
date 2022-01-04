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

// Package notification to manage a rook bucket notifications.
package notification

import (
	"testing"

	bktv1alpha1 "github.com/kube-object-storage/lib-bucket-provisioner/pkg/apis/objectbucket.io/v1alpha1"
	cephv1 "github.com/rook/rook/pkg/apis/ceph.rook.io/v1"

	"github.com/rook/rook/pkg/operator/ceph/object"
	"github.com/rook/rook/pkg/operator/k8sutil"
	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
)

func createOBResources(name string) (*bktv1alpha1.ObjectBucketClaim, *bktv1alpha1.ObjectBucket) {
	return &bktv1alpha1.ObjectBucketClaim{
			ObjectMeta: metav1.ObjectMeta{
				Name:      name,
				Namespace: testNamespace,
			},
			TypeMeta: metav1.TypeMeta{
				Kind: "ObjectBucketClaim",
			},
			Spec: bktv1alpha1.ObjectBucketClaimSpec{
				StorageClassName:   testSCName,
				GenerateBucketName: name,
			},
			Status: bktv1alpha1.ObjectBucketClaimStatus{
				Phase: bktv1alpha1.ObjectBucketClaimStatusPhasePending,
			},
		}, &bktv1alpha1.ObjectBucket{
			ObjectMeta: metav1.ObjectMeta{
				Name:      name,
				Namespace: testNamespace,
			},
			TypeMeta: metav1.TypeMeta{
				Kind: "ObjectBucket",
			},
			Spec: bktv1alpha1.ObjectBucketSpec{
				StorageClassName: testSCName,
				Connection: &bktv1alpha1.Connection{
					Endpoint: &bktv1alpha1.Endpoint{
						BucketHost: object.BuildDomainName(testStoreName, testNamespace),
					},
				},
				ClaimRef: &corev1.ObjectReference{
					Name: name},
			},
			Status: bktv1alpha1.ObjectBucketStatus{
				Phase: bktv1alpha1.ObjectBucketStatusPhaseBound,
			},
		}
}

func createBucketNotification(name string) *cephv1.CephBucketNotification {
	return &cephv1.CephBucketNotification{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: testNamespace,
		},
		TypeMeta: metav1.TypeMeta{
			Kind: "CephBucketNotification",
		},
		Spec: cephv1.BucketNotificationSpec{
			Topic: testTopicName,
		},
	}
}

func setNotificationLabels(labelList []string) map[string]string {
	var label = make(map[string]string)
	for _, value := range labelList {
		label[notificationLabelPrefix+value] = value
	}
	return label
}

func TestCephBucketNotificationOBCLabelController(t *testing.T) {
	mockSetup(t)
	defer mockCleanup()

	bucketTopic := &cephv1.CephBucketTopic{
		ObjectMeta: metav1.ObjectMeta{
			Name:      testTopicName,
			Namespace: testNamespace,
		},
		TypeMeta: metav1.TypeMeta{
			Kind: "CephBucketTopic",
		},
		Spec: cephv1.BucketTopicSpec{
			ObjectStoreName:      testStoreName,
			ObjectStoreNamespace: testNamespace,
			Endpoint: cephv1.TopicEndpointSpec{
				HTTP: &cephv1.HTTPEndpointSpec{
					URI: "http://localhost",
				},
			},
		},
		Status: &cephv1.BucketTopicStatus{ARN: &testARN},
	}

	cephCluster := &cephv1.CephCluster{
		ObjectMeta: metav1.ObjectMeta{
			Name:      testNamespace,
			Namespace: testNamespace,
		},
		Status: cephv1.ClusterStatus{
			Phase: k8sutil.ReadyStatus,
			CephStatus: &cephv1.CephStatus{
				Health: "HEALTH_OK",
			},
		},
	}

	obc, ob := createOBResources(testBucketName)
	obc.Labels = setNotificationLabels([]string{testNotificationName})

	t.Run("provision OBC with notification label with no ob", func(t *testing.T) {
		resetValues()
		objects := []runtime.Object{
			cephCluster,
			obc,
		}

		res, err := testOBCLabelReconciler(objects, testBucketName)
		assert.NoError(t, err)
		assert.True(t, res.Requeue)
		assert.False(t, getWasInvoked)
		assert.Equal(t, 0, len(createdNotifications))
		assert.Equal(t, 0, len(deletedNotifications))
		verifyEvents(t, []string{})
	})

	t.Run("provision OBC with notification label with not ready ob", func(t *testing.T) {
		resetValues()
		objects := []runtime.Object{
			cephCluster,
			obc,
			ob,
		}

		res, err := testOBCLabelReconciler(objects, testBucketName)
		assert.NoError(t, err)
		assert.True(t, res.Requeue)
		assert.False(t, getWasInvoked)
		assert.Equal(t, 0, len(createdNotifications))
		assert.Equal(t, 0, len(deletedNotifications))
		verifyEvents(t, []string{})
	})

	obc.Spec.ObjectBucketName = testBucketName
	obc.Status.Phase = bktv1alpha1.ObjectBucketClaimStatusPhaseBound

	t.Run("provision OBC with notification label with no notification", func(t *testing.T) {
		resetValues()
		objects := []runtime.Object{
			cephCluster,
			obc,
			ob,
		}

		res, err := testOBCLabelReconciler(objects, testBucketName)
		assert.Error(t, err)
		assert.True(t, res.Requeue)
		assert.True(t, getWasInvoked)
		assert.Equal(t, 0, len(createdNotifications))
		assert.Equal(t, 0, len(deletedNotifications))
		verifyEvents(t, []string{startEvent, failedEvent})
	})

	bucketNotification := createBucketNotification(testNotificationName)
	t.Run("provision OBC with notification label and notification with no topic", func(t *testing.T) {
		resetValues()
		objects := []runtime.Object{
			cephCluster,
			obc,
			ob,
			bucketNotification,
		}

		res, err := testOBCLabelReconciler(objects, testBucketName)
		assert.Error(t, err)
		assert.True(t, res.Requeue)
		assert.True(t, getWasInvoked)
		assert.Equal(t, 0, len(createdNotifications))
		assert.Equal(t, 0, len(deletedNotifications))
		verifyEvents(t, []string{startEvent, failedEvent})
	})

	t.Run("provision OBC with notification label", func(t *testing.T) {
		resetValues()
		objects := []runtime.Object{
			cephCluster,
			obc,
			ob,
			bucketNotification,
			bucketTopic,
		}

		res, err := testOBCLabelReconciler(objects, testBucketName)
		assert.NoError(t, err)
		assert.False(t, res.Requeue)
		assert.True(t, getWasInvoked)
		assert.Equal(t, 1, len(createdNotifications))
		assert.ElementsMatch(t, createdNotifications, []string{testNotificationName})
		assert.Equal(t, 0, len(deletedNotifications))
	})

	t.Run("reconcile with already existing label for the obc", func(t *testing.T) {
		resetValues()
		noChangeOBC, noChangeOB := createOBResources(noChangeBucketName)
		noChangeOBC.Labels = setNotificationLabels([]string{testNotificationName})
		noChangeOBC.Spec.ObjectBucketName = noChangeBucketName
		noChangeOBC.Status.Phase = bktv1alpha1.ObjectBucketClaimStatusPhaseBound
		objects := []runtime.Object{
			cephCluster,
			noChangeOBC,
			noChangeOB,
			bucketNotification,
			bucketTopic,
		}

		res, err := testOBCLabelReconciler(objects, noChangeBucketName)
		assert.NoError(t, err)
		assert.False(t, res.Requeue)
		assert.True(t, getWasInvoked)
		assert.Equal(t, 1, len(createdNotifications))
		assert.ElementsMatch(t, createdNotifications, []string{testNotificationName})
		assert.Equal(t, 0, len(deletedNotifications))
	})

	t.Run("delete notification from the obc", func(t *testing.T) {
		resetValues()
		deleteOBC, deleteOB := createOBResources(deleteBucketName)
		deleteOBC.Spec.GenerateBucketName = deleteBucketName
		deleteOBC.Spec.ObjectBucketName = deleteBucketName
		deleteOBC.Status.Phase = bktv1alpha1.ObjectBucketClaimStatusPhaseBound
		objects := []runtime.Object{
			cephCluster,
			deleteOBC,
			deleteOB,
		}

		res, err := testOBCLabelReconciler(objects, deleteBucketName)
		assert.NoError(t, err)
		assert.False(t, res.Requeue)
		assert.True(t, getWasInvoked)
		assert.Equal(t, 0, len(createdNotifications))
		assert.Equal(t, 1, len(deletedNotifications))
		assert.ElementsMatch(t, deletedNotifications,
			[]string{deleteBucketName + testNotificationName})
	})

	t.Run("provision OBC with multiple notification labels", func(t *testing.T) {
		resetValues()
		multipleCreateOBC, multipleCreateOB := createOBResources(multipleCreateBucketName)
		multipleCreateOBC.Labels = setNotificationLabels([]string{testNotificationName + "-1", testNotificationName + "-2"})
		multipleCreateOBC.Spec.ObjectBucketName = multipleCreateBucketName
		multipleCreateOBC.Status.Phase = bktv1alpha1.ObjectBucketClaimStatusPhaseBound
		bucketNotification1 := createBucketNotification(testNotificationName + "-1")
		bucketNotification2 := createBucketNotification(testNotificationName + "-2")

		objects := []runtime.Object{
			cephCluster,
			multipleCreateOBC,
			multipleCreateOB,
			bucketNotification1,
			bucketNotification2,
			bucketTopic,
		}

		res, err := testOBCLabelReconciler(objects, multipleCreateBucketName)
		assert.NoError(t, err)
		assert.False(t, res.Requeue)
		assert.True(t, getWasInvoked)
		assert.Equal(t, 2, len(createdNotifications))
		assert.ElementsMatch(t, createdNotifications, []string{testNotificationName + "-1", testNotificationName + "-2"})
		assert.Equal(t, 0, len(deletedNotifications))
	})

	t.Run("delete multiple notifications from the obc", func(t *testing.T) {
		resetValues()
		multipleDeleteOBC, multipleDeleteOB := createOBResources(multipleDeleteBucketName)
		multipleDeleteOBC.Spec.GenerateBucketName = multipleDeleteBucketName
		multipleDeleteOBC.Spec.ObjectBucketName = multipleDeleteBucketName
		multipleDeleteOBC.Status.Phase = bktv1alpha1.ObjectBucketClaimStatusPhaseBound
		objects := []runtime.Object{
			cephCluster,
			multipleDeleteOBC,
			multipleDeleteOB,
		}

		res, err := testOBCLabelReconciler(objects, multipleDeleteBucketName)
		assert.NoError(t, err)
		assert.False(t, res.Requeue)
		assert.True(t, getWasInvoked)
		assert.Equal(t, 0, len(createdNotifications))
		assert.Equal(t, 2, len(deletedNotifications))
		assert.ElementsMatch(t, deletedNotifications,
			[]string{multipleDeleteBucketName + testNotificationName + "-1", multipleDeleteBucketName + testNotificationName + "-2"})
	})
	t.Run("provision OBC with multiple delete and create of notifications", func(t *testing.T) {
		resetValues()
		multipleBothOBC, multipleBothOB := createOBResources(multipleBothBucketName)
		multipleBothOBC.Labels = setNotificationLabels([]string{testNotificationName + "-1", testNotificationName + "-2"})
		multipleBothOBC.Spec.ObjectBucketName = multipleBothBucketName
		multipleBothOBC.Status.Phase = bktv1alpha1.ObjectBucketClaimStatusPhaseBound
		bucketNotification1 := createBucketNotification(testNotificationName + "-1")
		bucketNotification2 := createBucketNotification(testNotificationName + "-2")

		objects := []runtime.Object{
			cephCluster,
			multipleBothOBC,
			multipleBothOB,
			bucketNotification1,
			bucketNotification2,
			bucketTopic,
		}

		res, err := testOBCLabelReconciler(objects, multipleBothBucketName)
		assert.NoError(t, err)
		assert.False(t, res.Requeue)
		assert.True(t, getWasInvoked)
		assert.Equal(t, 2, len(createdNotifications))
		assert.ElementsMatch(t, createdNotifications, []string{testNotificationName + "-1", testNotificationName + "-2"})
		assert.Equal(t, 2, len(deletedNotifications))
		assert.ElementsMatch(t, deletedNotifications,
			[]string{multipleDeleteBucketName + testNotificationName + "-1", multipleDeleteBucketName + testNotificationName + "-2"})
		verifyEvents(t, []string{startEvent, finishedEvent})
	})
}
