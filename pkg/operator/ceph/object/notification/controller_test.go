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
	"context"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/coreos/pkg/capnslog"
	bktv1alpha1 "github.com/kube-object-storage/lib-bucket-provisioner/pkg/apis/objectbucket.io/v1alpha1"
	cephv1 "github.com/rook/rook/pkg/apis/ceph.rook.io/v1"
	rookclient "github.com/rook/rook/pkg/client/clientset/versioned/fake"
	"github.com/rook/rook/pkg/operator/test"

	"github.com/rook/rook/pkg/clusterd"
	"github.com/rook/rook/pkg/operator/k8sutil"
	exectest "github.com/rook/rook/pkg/util/exec/test"
	"github.com/stretchr/testify/assert"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/tools/record"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

var (
	testTopicName            = "topic-a"
	testNotificationName     = "notification"
	testNamespace            = "rook-ceph"
	testStoreName            = "test-store"
	testARN                  = "arn:aws:sns:" + testStoreName + "::" + testTopicName
	testBucketName           = "my-bucket"
	testSCName               = "my-storage-class"
	deleteBucketName         = "delete"
	noChangeBucketName       = "nochange"
	multipleCreateBucketName = "multi-create"
	multipleDeleteBucketName = "multi-delete"
	multipleBothBucketName   = "multi-both"
	otherClusterBucketName   = "other-cluster"
	startEvent               = string(cephv1.ReconcileStarted)
	finishedEvent            = string(cephv1.ReconcileSucceeded)
	failedEvent              = string(cephv1.ReconcileFailed)
)

// global variables used inside mockSetup
var (
	testCtx              = context.TODO()
	testContext          *clusterd.Context
	testScheme           = scheme.Scheme
	testRecorder         = record.NewFakeRecorder(256)
	getWasInvoked        = false
	createdNotifications []string
	deletedNotifications []string
)

func resetValues() {
	getWasInvoked = false
	createdNotifications = nil
	deletedNotifications = nil
}

func mockCleanup() {
	resetValues()
	createNotificationFunc = createNotification
	getAllNotificationsFunc = getAllRGWNotifications
	deleteNotificationFunc = deleteNotification
}

func mockSetup(t *testing.T) {
	// set log level
	capnslog.SetGlobalLogLevel(capnslog.DEBUG)
	os.Setenv("ROOK_LOG_LEVEL", "DEBUG")

	// create clients
	testContext = &clusterd.Context{
		Executor:      &exectest.MockExecutor{},
		RookClientset: rookclient.NewSimpleClientset(),
		Clientset:     test.New(t, 3),
	}

	// create scheme
	testScheme.AddKnownTypes(
		cephv1.SchemeGroupVersion,
		&cephv1.CephBucketNotification{},
		&cephv1.CephBucketNotificationList{},
		&cephv1.CephBucketTopic{},
		&cephv1.CephBucketTopicList{},
		&cephv1.CephCluster{},
		&cephv1.CephClusterList{},
		&bktv1alpha1.ObjectBucketClaim{},
		&bktv1alpha1.ObjectBucketClaimList{},
		&bktv1alpha1.ObjectBucket{},
		&bktv1alpha1.ObjectBucketList{},
	)

	// create secrets
	secrets := map[string][]byte{
		"fsid":         []byte("name"),
		"mon-secret":   []byte("monsecret"),
		"admin-secret": []byte("adminsecret"),
	}
	secret := &v1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "rook-ceph-mon",
			Namespace: testNamespace,
		},
		Data: secrets,
		Type: k8sutil.RookType,
	}

	_, err := testContext.Clientset.CoreV1().Secrets(testNamespace).Create(testCtx, secret, metav1.CreateOptions{})
	assert.NoError(t, err)

	// test mocks
	createNotificationFunc = func(p provisioner, bucket *bktv1alpha1.ObjectBucket, topicARN string, notification *cephv1.CephBucketNotification) error {
		createdNotifications = append(createdNotifications, notification.Name)
		return nil
	}
	getAllNotificationsFunc = func(p provisioner, bucket *bktv1alpha1.ObjectBucket) ([]string, error) {
		getWasInvoked = true
		if bucket.Name == deleteBucketName {
			return []string{deleteBucketName + testNotificationName}, nil
		}
		if bucket.Name == noChangeBucketName {
			return []string{testNotificationName}, nil
		}
		if bucket.Name == multipleDeleteBucketName || bucket.Name == multipleBothBucketName {
			return []string{
				multipleDeleteBucketName + testNotificationName + "-1",
				multipleDeleteBucketName + testNotificationName + "-2",
			}, nil
		}
		return nil, nil
	}
	deleteNotificationFunc = func(p provisioner, bucket *bktv1alpha1.ObjectBucket, notificationId string) error {
		deletedNotifications = append(deletedNotifications, notificationId)
		return nil
	}
}

func testReconciler(objects []runtime.Object, notificationName string) (reconcile.Result, error) {
	defer func() { testRecorder.Events <- "END" }()
	cl := fake.NewClientBuilder().WithScheme(testScheme).WithRuntimeObjects(objects...).Build()
	r := &ReconcileNotifications{client: cl, context: testContext, opManagerContext: testCtx, recorder: testRecorder}
	testRequest := reconcile.Request{
		NamespacedName: types.NamespacedName{
			Name:      notificationName,
			Namespace: testNamespace,
		},
	}
	return r.Reconcile(testCtx, testRequest)
}

func testOBCLabelReconciler(objects []runtime.Object, bucketName string) (reconcile.Result, error) {
	defer func() { testRecorder.Events <- "END" }()
	cl := fake.NewClientBuilder().WithScheme(testScheme).WithRuntimeObjects(objects...).Build()
	r := &ReconcileOBCLabels{client: cl, context: testContext, opManagerContext: testCtx, recorder: testRecorder}
	testRequest := reconcile.Request{
		NamespacedName: types.NamespacedName{
			Name:      bucketName,
			Namespace: testNamespace,
		},
	}
	return r.Reconcile(testCtx, testRequest)
}

func verifyEvents(t *testing.T, expectedEvents []string) {
	expectedEvents = append(expectedEvents, "END")
	for _, expectedEvent := range expectedEvents {
		select {
		case event := <-testRecorder.Events:
			if expectedEvent != "END" {
				splitEvent := strings.Split(event, " ")
				// the event message must have at least 3 parts
				assert.GreaterOrEqual(t, len(splitEvent), 3)
				// the type of event (2nd part) must match
				assert.Equal(t, splitEvent[1], expectedEvent)
			} else {
				assert.Equal(t, expectedEvent, event)
			}
		case <-time.After(1 * time.Second):
			assert.Failf(t, "missing event", "missing event: \"%s\"", expectedEvent)
		}
	}
}

func TestCephBucketNotificationController(t *testing.T) {
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
		Status: &cephv1.BucketTopicStatus{ARN: nil},
	}
	bucketNotification := &cephv1.CephBucketNotification{
		ObjectMeta: metav1.ObjectMeta{
			Name:      testNotificationName,
			Namespace: testNamespace,
		},
		TypeMeta: metav1.TypeMeta{
			Kind: "CephBucketNotification",
		},
		Spec: cephv1.BucketNotificationSpec{
			Topic: testTopicName,
		},
	}
	cephCluster := &cephv1.CephCluster{
		ObjectMeta: metav1.ObjectMeta{
			Name:      testNamespace,
			Namespace: testNamespace,
		},
		Status: cephv1.ClusterStatus{
			Phase: k8sutil.EmptyStatus,
			CephStatus: &cephv1.CephStatus{
				Health: "",
			},
		},
	}

	t.Run("create notification configuration without a topic", func(t *testing.T) {
		// Objects to track in the fake client.
		objects := []runtime.Object{
			bucketNotification,
		}

		res, err := testReconciler(objects, testNotificationName)
		// provisioning requeued because the topic is not configured
		assert.NoError(t, err)
		assert.True(t, res.Requeue)
		assert.NoError(t, err, bucketNotification)
		assert.Equal(t, 0, len(createdNotifications))
		verifyEvents(t, []string{startEvent, failedEvent})
	})

	t.Run("create notification and topic configuration when there is no cluster", func(t *testing.T) {
		objects := []runtime.Object{
			bucketNotification,
			bucketTopic,
		}

		res, err := testReconciler(objects, testNotificationName)
		// provisioning requeued because the cluster does not exist
		assert.NoError(t, err)
		assert.True(t, res.Requeue)
		assert.NoError(t, err, bucketNotification)
		assert.Equal(t, 0, len(createdNotifications))
		verifyEvents(t, []string{startEvent, failedEvent})
	})

	t.Run("create notification and topic configuration cluster is not ready", func(t *testing.T) {
		objects := []runtime.Object{
			bucketNotification,
			bucketTopic,
			cephCluster,
		}

		res, err := testReconciler(objects, testNotificationName)
		// provisioning requeued because the cluster is not ready
		assert.NoError(t, err)
		assert.True(t, res.Requeue)
		assert.NoError(t, err, bucketNotification)
		assert.Equal(t, 0, len(createdNotifications))
		verifyEvents(t, []string{startEvent, failedEvent})
	})

	t.Run("create notification and topic configuration when topic is not yet provisioned", func(t *testing.T) {
		cephCluster.Status.Phase = k8sutil.ReadyStatus
		cephCluster.Status.CephStatus.Health = "HEALTH_OK"
		objects := []runtime.Object{
			bucketNotification,
			bucketTopic,
			cephCluster,
		}

		res, err := testReconciler(objects, testNotificationName)
		// provisioning requeued because the topic is not provisioned on the RGW
		assert.NoError(t, err)
		assert.True(t, res.Requeue)
		assert.Equal(t, 0, len(createdNotifications))
		verifyEvents(t, []string{startEvent, failedEvent})
	})

	t.Run("create notification and topic configuration", func(t *testing.T) {
		cephCluster.Status.Phase = k8sutil.ReadyStatus
		cephCluster.Status.CephStatus.Health = "HEALTH_OK"
		bucketTopic.Status.ARN = &testARN
		objects := []runtime.Object{
			bucketNotification,
			bucketTopic,
			cephCluster,
		}

		res, err := testReconciler(objects, testNotificationName)
		assert.NoError(t, err)
		assert.False(t, res.Requeue)
		assert.Equal(t, 0, len(createdNotifications))
		verifyEvents(t, []string{startEvent, finishedEvent})
	})
}

func TestCephBucketNotificationControllerWithOBC(t *testing.T) {
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
	bucketNotification := &cephv1.CephBucketNotification{
		ObjectMeta: metav1.ObjectMeta{
			Name:      testNotificationName,
			Namespace: testNamespace,
		},
		TypeMeta: metav1.TypeMeta{
			Kind: "CephBucketNotification",
		},
		Spec: cephv1.BucketNotificationSpec{
			Topic: testTopicName,
		},
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
	obc := &bktv1alpha1.ObjectBucketClaim{
		ObjectMeta: metav1.ObjectMeta{
			Name:      testBucketName,
			Namespace: testNamespace,
			Labels: map[string]string{
				notificationLabelPrefix + testNotificationName: testNotificationName,
			},
		},
		TypeMeta: metav1.TypeMeta{
			Kind: "ObjectBucketClaim",
		},
		Spec: bktv1alpha1.ObjectBucketClaimSpec{
			StorageClassName:   testSCName,
			GenerateBucketName: testBucketName,
		},
		Status: bktv1alpha1.ObjectBucketClaimStatus{
			Phase: bktv1alpha1.ObjectBucketClaimStatusPhasePending,
		},
	}

	t.Run("provision notification when OBC exists but no OB", func(t *testing.T) {
		objects := []runtime.Object{
			bucketNotification,
			bucketTopic,
			cephCluster,
			obc,
		}

		res, err := testReconciler(objects, testNotificationName)
		assert.NoError(t, err)
		assert.True(t, res.Requeue)
		assert.Equal(t, 0, len(createdNotifications))
		verifyEvents(t, []string{startEvent, failedEvent})
	})

	t.Run("provision notification when OB exists", func(t *testing.T) {
		ob := &bktv1alpha1.ObjectBucket{
			ObjectMeta: metav1.ObjectMeta{
				Name:      testBucketName,
				Namespace: testNamespace,
			},
			TypeMeta: metav1.TypeMeta{
				Kind: "ObjectBucket",
			},
			Spec: bktv1alpha1.ObjectBucketSpec{
				StorageClassName: testSCName,
				Connection: &bktv1alpha1.Connection{
					Endpoint: &bktv1alpha1.Endpoint{
						BucketHost: "rook-ceph-rgw-test-store.rook-ceph.svc",
					},
				},
			},
			Status: bktv1alpha1.ObjectBucketStatus{
				Phase: bktv1alpha1.ObjectBucketStatusPhaseBound,
			},
		}
		obc.Spec.ObjectBucketName = testBucketName
		obc.Status.Phase = bktv1alpha1.ObjectBucketClaimStatusPhaseBound
		objects := []runtime.Object{
			bucketNotification,
			bucketTopic,
			cephCluster,
			obc,
			ob,
		}

		res, err := testReconciler(objects, testNotificationName)
		assert.NoError(t, err)
		assert.False(t, res.Requeue)
		assert.Equal(t, 1, len(createdNotifications))
		verifyEvents(t, []string{startEvent, finishedEvent})
	})
}

func TestGetCephObjectStoreName(t *testing.T) {
	ob := bktv1alpha1.ObjectBucket{
		ObjectMeta: metav1.ObjectMeta{
			Name:      testBucketName,
			Namespace: testNamespace,
		},
		TypeMeta: metav1.TypeMeta{
			Kind: "ObjectBucket",
		},
		Spec: bktv1alpha1.ObjectBucketSpec{
			StorageClassName: testSCName,
			Connection: &bktv1alpha1.Connection{
				Endpoint: &bktv1alpha1.Endpoint{
					BucketHost: "",
				},
			},
		},
		Status: bktv1alpha1.ObjectBucketStatus{
			Phase: bktv1alpha1.ObjectBucketStatusPhaseBound,
		},
	}

	t.Run("empty bucket host", func(t *testing.T) {
		objectStore, err := getCephObjectStoreName(ob)
		assert.Error(t, err)
		assert.Empty(t, objectStore.Name)
	})
	t.Run("malformed suffix", func(t *testing.T) {
		ob.Spec.Connection.Endpoint.BucketHost = "rook-ceph-rgw-" + testStoreName + ".."
		objectStore, err := getCephObjectStoreName(ob)
		assert.Error(t, err)
		assert.Empty(t, objectStore.Name)
	})
	t.Run("malformed prefix", func(t *testing.T) {
		ob.Spec.Connection.Endpoint.BucketHost = "rook-rgw-" + testStoreName + "." + testNamespace + ".svc"
		objectStore, err := getCephObjectStoreName(ob)
		assert.Error(t, err)
		assert.Empty(t, objectStore.Name)
	})
	t.Run("empty store name", func(t *testing.T) {
		ob.Spec.Connection.Endpoint.BucketHost = "rook-ceph-rgw-" + "." + testNamespace + ".svc"
		objectStore, err := getCephObjectStoreName(ob)
		assert.Error(t, err)
		assert.Empty(t, objectStore.Name)
	})
	t.Run("store name contains rgw", func(t *testing.T) {
		testStoreNameWithRGW := "my-rgw-store"
		ob.Spec.Connection.Endpoint.BucketHost = "rook-ceph-rgw-" + testStoreNameWithRGW + "." + testNamespace + ".svc"
		objectStore, err := getCephObjectStoreName(ob)
		assert.NoError(t, err)
		assert.Equal(t, testStoreNameWithRGW, objectStore.Name)
		assert.Equal(t, testNamespace, objectStore.Namespace)
	})
	t.Run("valid store name", func(t *testing.T) {
		ob.Spec.Connection.Endpoint.BucketHost = "rook-ceph-rgw-" + testStoreName + "." + testNamespace + ".svc"
		objectStore, err := getCephObjectStoreName(ob)
		assert.NoError(t, err)
		assert.Equal(t, testStoreName, objectStore.Name)
		assert.Equal(t, testNamespace, objectStore.Namespace)
	})
}
