/*
Copyright 2026 The Rook Authors. All rights reserved.

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

// Package sharedstore provides a shared CephObjectStore fixture for
// tests under tests/integration/object. A single store is created once
// and torn down after all sub-package tests complete, avoiding the
// per-package setup/teardown overhead.
package sharedstore

import (
	"context"
	"testing"
	"time"

	cephv1 "github.com/rook/rook/pkg/apis/ceph.rook.io/v1"
	"github.com/rook/rook/tests/framework/utils"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
)

// Setup creates a shared CephObjectStore and its NodePort Service in
// ObjectStoreNamespace, waits for the store to become Ready, and returns a
// teardown function that deletes both resources. The teardown function should
// be deferred by the caller.
func Setup(t *testing.T, k8sh *utils.K8sHelper) (*cephv1.CephObjectStore, func()) {
	t.Helper()
	ctx := context.TODO()

	objectStore := &cephv1.CephObjectStore{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-shared",
			Namespace: "object-ns",
		},
		Spec: cephv1.ObjectStoreSpec{
			MetadataPool: cephv1.PoolSpec{
				Replicated: cephv1.ReplicatedSpec{
					Size:                   1,
					RequireSafeReplicaSize: false,
				},
			},
			DataPool: cephv1.PoolSpec{
				Replicated: cephv1.ReplicatedSpec{
					Size:                   1,
					RequireSafeReplicaSize: false,
				},
			},
			Gateway: cephv1.GatewaySpec{
				Port:      80,
				Instances: 1,
			},
			// Allow CephObjectStoreUser resources from every namespace used by
			// the sub-package tests under tests/integration/object.
			AllowUsersInNamespaces: []string{
				"test-bucketowner",
				"test-topickafka",
				"test-userkeys",
				"test-useropmask",
			},
		},
	}

	svc := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			// The service name must match ObjectStoreName so that
			// util/s3.GetS3Endpoint can locate it by objectStore.Name.
			Name:      objectStore.Name,
			Namespace: objectStore.Namespace,
		},
		Spec: corev1.ServiceSpec{
			Selector: map[string]string{
				"app":               "rook-ceph-rgw",
				"rook_cluster":      objectStore.Namespace,
				"rook_object_store": objectStore.Name,
			},
			Ports: []corev1.ServicePort{
				{
					Name:       "http",
					Port:       80,
					Protocol:   corev1.ProtocolTCP,
					TargetPort: intstr.FromInt(8080),
				},
			},
			SessionAffinity: corev1.ServiceAffinityNone,
			Type:            corev1.ServiceTypeNodePort,
		},
	}

	_, err := k8sh.RookClientset.CephV1().CephObjectStores(objectStore.Namespace).Create(ctx, objectStore, metav1.CreateOptions{})
	require.NoError(t, err)

	osReady := utils.Retry(180, time.Second, "shared CephObjectStore is Ready", func() bool {
		liveOs, err := k8sh.RookClientset.CephV1().CephObjectStores(objectStore.Namespace).Get(ctx, objectStore.Name, metav1.GetOptions{})
		if err != nil {
			return false
		}
		if liveOs.Status == nil {
			return false
		}
		return liveOs.Status.Phase == cephv1.ConditionReady
	})
	require.True(t, osReady, "shared CephObjectStore did not become Ready")

	_, err = k8sh.Clientset.CoreV1().Services(objectStore.Namespace).Create(ctx, svc, metav1.CreateOptions{})
	require.NoError(t, err)

	return objectStore, func() {
		if err := k8sh.Clientset.CoreV1().Services(objectStore.Namespace).Delete(ctx, objectStore.Name, metav1.DeleteOptions{}); err != nil {
			t.Logf("failed to delete shared object store service: %v", err)
		}
		if err := k8sh.RookClientset.CephV1().CephObjectStores(objectStore.Namespace).Delete(ctx, objectStore.Name, metav1.DeleteOptions{}); err != nil {
			t.Logf("failed to delete shared CephObjectStore: %v", err)
		}
	}
}
