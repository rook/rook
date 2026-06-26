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
	"fmt"
	"testing"
	"time"

	"github.com/ceph/go-ceph/rgw/admin"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"

	cephv1 "github.com/rook/rook/pkg/apis/ceph.rook.io/v1"
	"github.com/rook/rook/tests/framework/installer"
	"github.com/rook/rook/tests/framework/utils"
	utiladmin "github.com/rook/rook/tests/integration/object/util/admin"
	"github.com/rook/rook/tests/integration/object/util/wait4"
)

type Sharedstore struct {
	adminClient *admin.API
	objectStore *cephv1.CephObjectStore
	destroy     func()
}

func (s *Sharedstore) AdminClient() *admin.API {
	return s.adminClient
}

func (s *Sharedstore) ObjectStore() *cephv1.CephObjectStore {
	return s.objectStore
}

func (s *Sharedstore) Destroy() {
	s.destroy()
}

// Setup creates a shared CephObjectStore and its NodePort Service in
// ObjectStoreNamespace, waits for the store to become Ready, and returns a
// teardown function that deletes both resources. The teardown function should
// be deferred by the caller.
func Create(t *testing.T, k8sh *utils.K8sHelper, installer *installer.CephInstaller) *Sharedstore {
	t.Helper()

	s := &Sharedstore{}
	ctx := context.TODO()
	ns := "object-ns"
	storeName := "sharedstore"

	realm := &cephv1.CephObjectRealm{
		ObjectMeta: metav1.ObjectMeta{
			Name:      storeName,
			Namespace: ns,
		},
		Spec: cephv1.ObjectRealmSpec{
			DefaultRealm: true,
		},
	}

	zoneGroup := &cephv1.CephObjectZoneGroup{
		ObjectMeta: metav1.ObjectMeta{
			Name:      storeName,
			Namespace: ns,
		},
		Spec: cephv1.ObjectZoneGroupSpec{
			Realm: realm.Name,
		},
	}

	zone := &cephv1.CephObjectZone{
		ObjectMeta: metav1.ObjectMeta{
			Name:      storeName,
			Namespace: ns,
		},
		Spec: cephv1.ObjectZoneSpec{
			ZoneGroup: zoneGroup.Name,
			SharedPools: cephv1.ObjectSharedPoolsSpec{
				PoolPlacements: []cephv1.PoolPlacementSpec{
					{
						Name:             "default",
						Default:          true,
						MetadataPoolName: storeName + ".rgw.buckets.index",
						DataPoolName:     storeName + ".rgw.buckets.data",
						StorageClasses: []cephv1.PlacementStorageClassSpec{
							{
								Name:         "FOO",
								DataPoolName: storeName + ".rgw.buckets.data.foo",
							},
						},
					},
				},
			},
		},
	}

	poolNames := map[string]string{
		"rgw.root":                          ".rgw.root",
		storeName + ".rgw.control":          "",
		storeName + ".rgw.meta":             "",
		storeName + ".rgw.log":              "",
		storeName + ".rgw.otp":              "",
		storeName + ".rgw.buckets.index":    "",
		storeName + ".rgw.buckets.data":     "",
		storeName + ".rgw.buckets.data.foo": "",
	}

	objectStore := &cephv1.CephObjectStore{
		ObjectMeta: metav1.ObjectMeta{
			Name:      storeName,
			Namespace: ns,
		},
		Spec: cephv1.ObjectStoreSpec{
			Zone: cephv1.ZoneSpec{
				Name: zone.Name,
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
				"test-usercaps",
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

	canaryUser := &cephv1.CephObjectStoreUser{
		ObjectMeta: metav1.ObjectMeta{
			Name:      storeName + "-canary",
			Namespace: ns,
		},
		Spec: cephv1.ObjectStoreUserSpec{
			Store:            objectStore.Name,
			ClusterNamespace: objectStore.Namespace,
		},
	}

	var pools []*cephv1.CephBlockPool

	for k, v := range poolNames {
		pools = append(pools, &cephv1.CephBlockPool{
			ObjectMeta: metav1.ObjectMeta{
				Name:      k,
				Namespace: ns,
			},
			Spec: cephv1.NamedBlockPoolSpec{
				Name: v,
				PoolSpec: cephv1.PoolSpec{
					Replicated: cephv1.ReplicatedSpec{
						Size:                   1,
						RequireSafeReplicaSize: false,
					},
					Parameters: map[string]string{
						"pg_autoscale_mode": "off",
						"pg_num":            "1",
					},
				},
			},
		})
	}

	ceph := k8sh.RookClientset.CephV1()

	{
		_, err := ceph.CephObjectRealms(ns).Create(ctx, realm, metav1.CreateOptions{})
		require.NoError(t, err)
	}

	{
		_, err := ceph.CephObjectZoneGroups(ns).Create(ctx, zoneGroup, metav1.CreateOptions{})
		require.NoError(t, err)
	}

	{
		_, err := ceph.CephObjectZones(ns).Create(ctx, zone, metav1.CreateOptions{})
		require.NoError(t, err)
	}

	for _, p := range pools {
		_, err := ceph.CephBlockPools(ns).Create(ctx, p, metav1.CreateOptions{})
		require.NoError(t, err)
	}

	{
		_, err := ceph.CephObjectStores(ns).Create(ctx, objectStore, metav1.CreateOptions{})
		require.NoError(t, err)
	}

	osReady := utils.Retry(180, time.Second, "shared CephObjectStore is Ready", func() bool {
		liveOs, err := ceph.CephObjectStores(ns).Get(ctx, objectStore.Name, metav1.GetOptions{})
		if err != nil {
			return false
		}
		if liveOs.Status == nil {
			return false
		}
		return liveOs.Status.Phase == cephv1.ConditionReady
	})
	require.True(t, osReady, "shared CephObjectStore did not become Ready")

	{
		_, err := k8sh.Clientset.CoreV1().Services(ns).Create(ctx, svc, metav1.CreateOptions{})
		require.NoError(t, err)
	}

	t.Run("setup rgw admin api client", func(t *testing.T) {
		_, err := ceph.CephObjectStoreUsers(ns).Create(ctx, canaryUser, metav1.CreateOptions{})
		require.NoError(t, err)

		// user creation may be slow right after rgw start up
		osuReady := utils.Retry(120, time.Second, "CephObjectStoreUser is Ready", func() bool {
			liveOsu, err := ceph.CephObjectStoreUsers(ns).Get(ctx, canaryUser.Name, metav1.GetOptions{})
			if err != nil {
				return false
			}

			if liveOsu.Status == nil {
				return false
			}

			return liveOsu.Status.Phase == string(cephv1.ConditionReady)
		})
		require.True(t, osuReady)

		// wait for cosu user to be ready so we know rgw admin api is ready
		s.adminClient, err = utiladmin.NewAdminClient(objectStore, installer, k8sh, false)
		require.NoError(t, err)
	})

	s.objectStore = objectStore

	s.destroy = func() {
		t.Run(fmt.Sprintf("destroy CephObjectStore %s", objectStore.Name), func(t *testing.T) {
			wait4.AssertDelete(ctx, t, k8sh.Clientset.CoreV1().Services(ns), svc.Name, 30*time.Second)
			wait4.AssertDelete(ctx, t, ceph.CephObjectStores(ns), objectStore.Name, 5*time.Minute)

			for _, p := range pools {
				wait4.AssertDelete(ctx, t, ceph.CephBlockPools(ns), p.Name, time.Minute)
			}

			wait4.AssertDelete(ctx, t, ceph.CephObjectZones(ns), zone.Name, 2*time.Minute)
			wait4.AssertDelete(ctx, t, ceph.CephObjectZoneGroups(ns), zoneGroup.Name, time.Minute)
			wait4.AssertDelete(ctx, t, ceph.CephObjectRealms(ns), realm.Name, time.Minute)
		})
	}

	return s
}
