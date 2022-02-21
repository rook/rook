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

// Package topic to manage a rook bucket topics.
package topic

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/coreos/pkg/capnslog"
	cephv1 "github.com/rook/rook/pkg/apis/ceph.rook.io/v1"
	rookclient "github.com/rook/rook/pkg/client/clientset/versioned/fake"
	"github.com/rook/rook/pkg/operator/test"

	"github.com/rook/rook/pkg/clusterd"
	cephclient "github.com/rook/rook/pkg/daemon/ceph/client"
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

var (
	name           = "topic-a"
	namespace      = "rook-ceph"
	store          = "test-store"
	userCreateJSON = `{
		"user_id": "rgw-admin-ops-user",
		"display_name": "RGW Admin Ops User",
		"email": "",
		"suspended": 0,
		"max_buckets": 0,
		"subusers": [],
		"keys": [
			{
				"user": "rgw-admin-ops-user",
				"access_key": "EOE7FYCNOBZJ5VFV909G",
				"secret_key": "qmIqpWm8HxCzmynCrD6U6vKWi4hnDBndOnmxXNsV"
			}
		]
	}`
)

func TestCephBucketTopicController(t *testing.T) {
	ctx := context.TODO()
	capnslog.SetGlobalLogLevel(capnslog.DEBUG)
	os.Setenv("ROOK_LOG_LEVEL", "DEBUG")

	bucketTopic := &cephv1.CephBucketTopic{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
		TypeMeta: metav1.TypeMeta{
			Kind: "CephBucketTopic",
		},
		Spec: cephv1.BucketTopicSpec{
			ObjectStoreName:      store,
			ObjectStoreNamespace: namespace,
			Endpoint: cephv1.TopicEndpointSpec{
				HTTP: &cephv1.HTTPEndpointSpec{
					URI: "http://localhost",
				},
			},
		},
	}
	clusterInfo := cephclient.AdminClusterInfo(ctx, namespace, "rook")
	clusterSpec := cephv1.ClusterSpec{}
	req := reconcile.Request{
		NamespacedName: types.NamespacedName{
			Name:      name,
			Namespace: namespace,
		},
	}

	t.Run("do nothing since there is no CephCluster", func(t *testing.T) {
		// Objects to track in the fake client.
		objects := []runtime.Object{
			bucketTopic,
		}

		c := &clusterd.Context{
			Executor:      &exectest.MockExecutor{},
			RookClientset: rookclient.NewSimpleClientset(),
			Clientset:     test.New(t, 3),
		}

		s := scheme.Scheme
		s.AddKnownTypes(cephv1.SchemeGroupVersion, &cephv1.CephBucketTopic{}, &cephv1.CephBucketTopicList{})

		cl := fake.NewClientBuilder().WithScheme(s).WithRuntimeObjects(objects...).Build()

		r := &ReconcileBucketTopic{client: cl, context: c, clusterInfo: clusterInfo, clusterSpec: &clusterSpec, opManagerContext: ctx}

		res, err := r.Reconcile(ctx, req)
		assert.NoError(t, err)
		assert.True(t, res.Requeue)
	})

	t.Run("CephCluster is not ready", func(t *testing.T) {
		// Objects to track in the fake client.
		objects := []runtime.Object{
			bucketTopic,
			&cephv1.CephCluster{
				ObjectMeta: metav1.ObjectMeta{
					Name:      namespace,
					Namespace: namespace,
				},
				Status: cephv1.ClusterStatus{
					Phase: "",
					CephStatus: &cephv1.CephStatus{
						Health: "",
					},
				},
			},
		}

		c := &clusterd.Context{
			Executor:      &exectest.MockExecutor{},
			RookClientset: rookclient.NewSimpleClientset(),
			Clientset:     test.New(t, 3),
		}

		s := scheme.Scheme
		s.AddKnownTypes(cephv1.SchemeGroupVersion, &cephv1.CephBucketTopic{}, &cephv1.CephBucketTopicList{}, &cephv1.CephCluster{}, &cephv1.CephClusterList{})
		cl := fake.NewClientBuilder().WithScheme(s).WithRuntimeObjects(objects...).Build()

		r := &ReconcileBucketTopic{client: cl, context: c, clusterInfo: clusterInfo, clusterSpec: &clusterSpec, opManagerContext: ctx}
		res, err := r.Reconcile(ctx, req)
		assert.NoError(t, err)
		assert.True(t, res.Requeue)
	})

	t.Run("creating a topic", func(t *testing.T) {
		// Objects to track in the fake client.
		objects := []runtime.Object{
			bucketTopic,
			&cephv1.CephCluster{
				ObjectMeta: metav1.ObjectMeta{
					Name:      namespace,
					Namespace: namespace,
				},
				Status: cephv1.ClusterStatus{
					Phase: k8sutil.ReadyStatus,
					CephStatus: &cephv1.CephStatus{
						Health: "HEALTH_OK",
					},
				},
			},
		}

		executor := &exectest.MockExecutor{
			MockExecuteCommandWithTimeout: func(timeout time.Duration, command string, args ...string) (string, error) {
				if args[0] == "user" && args[1] == "create" {
					return userCreateJSON, nil
				}
				return "", nil
			},
		}

		c := &clusterd.Context{
			Executor:      executor,
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
				Namespace: namespace,
			},
			Data: secrets,
			Type: k8sutil.RookType,
		}
		_, err := c.Clientset.CoreV1().Secrets(namespace).Create(ctx, secret, metav1.CreateOptions{})
		assert.NoError(t, err)

		s := scheme.Scheme
		s.AddKnownTypes(cephv1.SchemeGroupVersion, &cephv1.CephBucketTopic{}, &cephv1.CephBucketTopicList{}, &cephv1.CephCluster{}, &cephv1.CephClusterList{})

		cl := fake.NewClientBuilder().WithScheme(s).WithRuntimeObjects(objects...).Build()

		cephObjectStore := &cephv1.CephObjectStore{
			ObjectMeta: metav1.ObjectMeta{
				Name:      store,
				Namespace: namespace,
			},
			TypeMeta: metav1.TypeMeta{
				Kind: "CephObjectStore"},
			Spec: cephv1.ObjectStoreSpec{
				Gateway: cephv1.GatewaySpec{
					Port: int32(80),
				},
			},
		}

		_, err = c.RookClientset.CephV1().CephObjectStores(namespace).Create(ctx, cephObjectStore, metav1.CreateOptions{})
		assert.NoError(t, err)
		r := &ReconcileBucketTopic{client: cl, context: c, clusterInfo: clusterInfo, clusterSpec: &clusterSpec, opManagerContext: ctx}

		err = r.client.Get(ctx, types.NamespacedName{Name: name, Namespace: namespace}, bucketTopic)
		assert.NoError(t, err, bucketTopic)

		// mock the provisioner
		expectedARN := "arn:aws:sns:" + store + "::" + bucketTopic.Name
		createTopicFunc = func(p provisioner, topic *cephv1.CephBucketTopic) (*string, error) {
			return &expectedARN, nil
		}
		defer func() { createTopicFunc = createTopic }()
		res, err := r.Reconcile(ctx, req)
		assert.NoError(t, err)
		assert.False(t, res.Requeue)
		err = r.client.Get(ctx, req.NamespacedName, bucketTopic)
		assert.NoError(t, err)
		assert.NotNil(t, bucketTopic.Status.ARN)
		assert.Equal(t, *bucketTopic.Status.ARN, expectedARN)
	})
}
