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

package opmask

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/ceph/go-ceph/rgw/admin"
	"github.com/coreos/pkg/capnslog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"

	cephv1 "github.com/rook/rook/pkg/apis/ceph.rook.io/v1"
	"github.com/rook/rook/tests/framework/installer"
	"github.com/rook/rook/tests/framework/utils"
	utiladmin "github.com/rook/rook/tests/integration/object/util/admin"
)

var (
	defaultName = "test-cosu-opmask"

	ns = &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: defaultName,
		},
	}

	objectStore = &cephv1.CephObjectStore{
		ObjectMeta: metav1.ObjectMeta{
			Name: defaultName,
			// the CephObjectstore must be in the same ns as the CephCluster
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
			AllowUsersInNamespaces: []string{ns.Name},
		},
	}

	objectStoreSvc = &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
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

	osu1 = cephv1.CephObjectStoreUser{
		ObjectMeta: metav1.ObjectMeta{
			Name:      defaultName + "-user1",
			Namespace: ns.Name,
		},
		Spec: cephv1.ObjectStoreUserSpec{
			Store:            objectStore.Name,
			ClusterNamespace: objectStore.Namespace,
			OpMask:           "read, delete",
		},
	}
)

func TestObjectStoreUserOpMask(t *testing.T, k8sh *utils.K8sHelper, installer *installer.CephInstaller, logger *capnslog.PackageLogger, tlsEnable bool) {
	t.Run("ObjectStoreUser keys", func(t *testing.T) {
		if tlsEnable {
			// Skip testing with and without TLS to reduce test time
			t.Skip("skipping test for TLS enabled clusters")
		}

		var adminClient *admin.API
		ctx := context.TODO()

		t.Run(fmt.Sprintf("create ns %q", ns.Name), func(t *testing.T) {
			_, err := k8sh.Clientset.CoreV1().Namespaces().Create(ctx, ns, metav1.CreateOptions{})
			require.NoError(t, err)
		})

		t.Run(fmt.Sprintf("create CephObjectStore %q", objectStore.Name), func(t *testing.T) {
			objectStore, err := k8sh.RookClientset.CephV1().CephObjectStores(objectStore.Namespace).Create(ctx, objectStore, metav1.CreateOptions{})
			require.NoError(t, err)

			osReady := utils.Retry(180, time.Second, "CephObjectStore is Ready", func() bool {
				liveOs, err := k8sh.RookClientset.CephV1().CephObjectStores(objectStore.Namespace).Get(ctx, objectStore.Name, metav1.GetOptions{})
				if err != nil {
					return false
				}

				if liveOs.Status == nil {
					return false
				}

				return liveOs.Status.Phase == cephv1.ConditionReady
			})
			require.True(t, osReady)
		})

		t.Run(fmt.Sprintf("create svc %q", objectStoreSvc.Name), func(t *testing.T) {
			_, err := k8sh.Clientset.CoreV1().Services(objectStore.Namespace).Create(ctx, objectStoreSvc, metav1.CreateOptions{})
			require.NoError(t, err)
		})

		t.Run(fmt.Sprintf("create CephObjectStoreUser %q", osu1.Name), func(t *testing.T) {
			_, err := k8sh.RookClientset.CephV1().CephObjectStoreUsers(ns.Name).Create(ctx, &osu1, metav1.CreateOptions{})
			require.NoError(t, err)

			// user creation may be slow right after rgw start up
			osuReady := utils.Retry(120, time.Second, "CephObjectStoreUser is Ready", func() bool {
				liveOsu, err := k8sh.RookClientset.CephV1().CephObjectStoreUsers(ns.Name).Get(ctx, osu1.Name, metav1.GetOptions{})
				if err != nil {
					return false
				}

				if liveOsu.Status == nil {
					return false
				}

				return liveOsu.Status.Phase == string(cephv1.ConditionReady)
			})
			require.True(t, osuReady)
		})

		// wait for cosu user to be ready so we know rgw admin api is ready
		t.Run("setup rgw admin api client", func(t *testing.T) {
			var err error
			adminClient, err = utiladmin.NewAdminClient(objectStore, installer, k8sh, tlsEnable)
			require.NoError(t, err)
		})

		t.Run(fmt.Sprintf("verify op_mask set on CephObjectStoreUser %q", osu1.Name), func(t *testing.T) {
			liveUser, err := adminClient.GetUser(ctx, admin.User{ID: osu1.Name})
			require.NoError(t, err)
			assert.Equal(t, "read, delete", liveUser.OpMask)
		})

		t.Run(fmt.Sprintf("update op-mask on CephObjectStoreUser %q", osu1.Name), func(t *testing.T) {
			liveOsu, err := k8sh.RookClientset.CephV1().CephObjectStoreUsers(ns.Name).Get(ctx, osu1.Name, metav1.GetOptions{})
			require.NoError(t, err)

			liveOsu.Spec.OpMask = "read"

			_, err = k8sh.RookClientset.CephV1().CephObjectStoreUsers(ns.Name).Update(ctx, liveOsu, metav1.UpdateOptions{})
			require.NoError(t, err)
		})

		t.Run(fmt.Sprintf("verify op_mask set on CephObjectStoreUser %q", osu1.Name), func(t *testing.T) {
			converged := utils.Retry(30, time.Second, "OpMask updated", func() bool {
				liveUser, err := adminClient.GetUser(ctx, admin.User{ID: osu1.Name})
				if err != nil {
					return false
				}
				fmt.Println("Current op_mask:", liveUser.OpMask)

				return liveUser.OpMask == "read"
			})

			assert.True(t, converged)
		})

		t.Run(fmt.Sprintf("delete CephObjectStoreUser %q", osu1.Name), func(t *testing.T) {
			err := k8sh.RookClientset.CephV1().CephObjectStoreUsers(ns.Name).Delete(ctx, osu1.Name, metav1.DeleteOptions{})
			require.NoError(t, err)

			absent := utils.Retry(40, time.Second, "CephObjectStoreUser is absent", func() bool {
				_, err := k8sh.RookClientset.CephV1().CephObjectStoreUsers(ns.Name).Get(ctx, osu1.Name, metav1.GetOptions{})
				return err != nil
			})
			assert.True(t, absent)
		})

		t.Run(fmt.Sprintf("no CephObjectStoreUsers in ns %q", ns.Name), func(t *testing.T) {
			osus, err := k8sh.RookClientset.CephV1().CephObjectStoreUsers(ns.Name).List(ctx, metav1.ListOptions{})
			require.NoError(t, err)

			assert.Len(t, osus.Items, 0)
		})

		t.Run(fmt.Sprintf("delete svc %q", objectStoreSvc.Name), func(t *testing.T) {
			err := k8sh.Clientset.CoreV1().Services(objectStore.Namespace).Delete(ctx, objectStoreSvc.Name, metav1.DeleteOptions{})
			require.NoError(t, err)
		})

		t.Run(fmt.Sprintf("delete CephObjectStore %q", objectStore.Name), func(t *testing.T) {
			err := k8sh.RookClientset.CephV1().CephObjectStores(objectStore.Namespace).Delete(ctx, objectStore.Name, metav1.DeleteOptions{})
			require.NoError(t, err)
		})

		t.Run(fmt.Sprintf("delete ns %q", ns.Name), func(t *testing.T) {
			err := k8sh.Clientset.CoreV1().Namespaces().Delete(ctx, ns.Name, metav1.DeleteOptions{})
			require.NoError(t, err)
		})
	})
}
