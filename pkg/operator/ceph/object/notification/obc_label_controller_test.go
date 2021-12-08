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
	"testing"

	"github.com/coreos/pkg/capnslog"
	bktv1alpha1 "github.com/kube-object-storage/lib-bucket-provisioner/pkg/apis/objectbucket.io/v1alpha1"
	cephv1 "github.com/rook/rook/pkg/apis/ceph.rook.io/v1"
	rookclient "github.com/rook/rook/pkg/client/clientset/versioned/fake"
	"github.com/rook/rook/pkg/operator/test"

	"github.com/rook/rook/pkg/clusterd"
	"github.com/rook/rook/pkg/operator/ceph/object"
	"github.com/rook/rook/pkg/operator/k8sutil"
	exectest "github.com/rook/rook/pkg/util/exec/test"
	"github.com/stretchr/testify/assert"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

func TestCephBucketNotificationOBCLabelController(t *testing.T) {
	mockSetup()
	defer mockCleanup()
	ctx := context.TODO()
	capnslog.SetGlobalLogLevel(capnslog.DEBUG)
	os.Setenv("ROOK_LOG_LEVEL", "DEBUG")

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
					BucketHost: object.BuildDomainName(testStoreName, testNamespace),
				},
			},
		},
		Status: bktv1alpha1.ObjectBucketStatus{
			Phase: bktv1alpha1.ObjectBucketStatusPhaseBound,
		},
	}
	req := reconcile.Request{
		NamespacedName: types.NamespacedName{
			Name:      testBucketName,
			Namespace: testNamespace,
		},
	}

	s := scheme.Scheme
	s.AddKnownTypes(
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

	c := &clusterd.Context{
		Executor:      &exectest.MockExecutor{},
		RookClientset: rookclient.NewSimpleClientset(),
		Clientset:     test.New(t, 3),
	}

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

	_, err := c.Clientset.CoreV1().Secrets(testNamespace).Create(ctx, secret, metav1.CreateOptions{})
	assert.NoError(t, err)

	t.Run("provision OBC with notification label with no ob", func(t *testing.T) {
		objects := []runtime.Object{
			cephCluster,
			obc,
		}

		cl := fake.NewClientBuilder().WithScheme(s).WithRuntimeObjects(objects...).Build()

		r := &ReconcileOBCLabels{client: cl, context: c, opManagerContext: ctx}

		res, err := r.Reconcile(ctx, req)
		assert.NoError(t, err)
		assert.True(t, res.Requeue)
		assert.False(t, createWasInvoked)
	})

	t.Run("provision OBC with notification label with not ready ob", func(t *testing.T) {
		objects := []runtime.Object{
			cephCluster,
			obc,
			ob,
		}

		cl := fake.NewClientBuilder().WithScheme(s).WithRuntimeObjects(objects...).Build()

		r := &ReconcileOBCLabels{client: cl, context: c, opManagerContext: ctx}

		res, err := r.Reconcile(ctx, req)
		assert.NoError(t, err)
		assert.True(t, res.Requeue)
		assert.False(t, createWasInvoked)
	})

	obc.Spec.ObjectBucketName = testBucketName
	obc.Status.Phase = bktv1alpha1.ObjectBucketClaimStatusPhaseBound

	t.Run("provision OBC with notification label with no notification", func(t *testing.T) {
		objects := []runtime.Object{
			cephCluster,
			obc,
			ob,
		}

		cl := fake.NewClientBuilder().WithScheme(s).WithRuntimeObjects(objects...).Build()

		r := &ReconcileOBCLabels{client: cl, context: c, opManagerContext: ctx}

		res, err := r.Reconcile(ctx, req)
		assert.NoError(t, err)
		assert.True(t, res.Requeue)
		assert.False(t, createWasInvoked)
	})

	t.Run("provision OBC with notification label and notification with no topic", func(t *testing.T) {
		objects := []runtime.Object{
			cephCluster,
			obc,
			ob,
			bucketNotification,
		}

		cl := fake.NewClientBuilder().WithScheme(s).WithRuntimeObjects(objects...).Build()

		r := &ReconcileOBCLabels{client: cl, context: c, opManagerContext: ctx}

		res, err := r.Reconcile(ctx, req)
		assert.NoError(t, err)
		assert.True(t, res.Requeue)
		assert.False(t, createWasInvoked)
	})

	t.Run("provision OBC with notification label", func(t *testing.T) {
		objects := []runtime.Object{
			cephCluster,
			obc,
			ob,
			bucketNotification,
			bucketTopic,
		}

		cl := fake.NewClientBuilder().WithScheme(s).WithRuntimeObjects(objects...).Build()

		r := &ReconcileOBCLabels{client: cl, context: c, opManagerContext: ctx}

		res, err := r.Reconcile(ctx, req)
		assert.NoError(t, err)
		assert.False(t, res.Requeue)
		assert.True(t, createWasInvoked)
	})
}
