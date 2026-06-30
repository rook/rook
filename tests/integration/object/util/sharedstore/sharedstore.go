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

	"github.com/aws/aws-sdk-go-v2/service/sns"
	"github.com/ceph/go-ceph/rgw/admin"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"

	cephv1 "github.com/rook/rook/pkg/apis/ceph.rook.io/v1"
	"github.com/rook/rook/tests/framework/installer"
	"github.com/rook/rook/tests/framework/utils"
	"github.com/rook/rook/tests/integration/object/util/client"
	"github.com/rook/rook/tests/integration/object/util/wait4"
)

type Sharedstore struct {
	adminClient *admin.API
	snsClient   *sns.Client
	objectStore *cephv1.CephObjectStore
	tlsEnable   bool
	destroy     func()
}

func (s *Sharedstore) AdminClient() *admin.API {
	return s.adminClient
}

func (s *Sharedstore) SnsClient() *sns.Client {
	return s.snsClient
}

func (s *Sharedstore) TLSEnabled() bool {
	return s.tlsEnable
}

func (s *Sharedstore) ObjectStore() *cephv1.CephObjectStore {
	return s.objectStore
}

func (s *Sharedstore) Destroy() {
	s.destroy()
}

// Create creates a CephObjectStore named storeName in namespace (with its
// realm, zone, shared pools, and NodePort Service), waits for it to become
// Ready, and returns a Sharedstore whose Destroy method tears it all down;
// Destroy should be deferred by the caller. instances sets the RGW gateway
// count and allowedNamespaces is propagated to AllowUsersInNamespaces.
func Create(t *testing.T, k8sh *utils.K8sHelper, installer *installer.CephInstaller, tlsEnable bool, namespace, storeName string, instances int32, allowedNamespaces ...string) *Sharedstore {
	t.Helper()

	s := &Sharedstore{tlsEnable: tlsEnable}
	ctx := context.TODO()
	ns := namespace

	// securePort is the in-container RGW TLS listener port. The NodePort Service
	// below exposes the conventional 443 externally and forwards to it.
	const securePort int32 = 8443
	certSecretName := storeName + "-tls"
	rgwServiceName := "rook-ceph-rgw-" + storeName

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
				Instances: instances,
			},
			AllowUsersInNamespaces: allowedNamespaces,
		},
	}

	if tlsEnable {
		// TLS-only store: omit the plain Port and serve https on securePort,
		// referencing the cert secret generated below.
		objectStore.Spec.Gateway = cephv1.GatewaySpec{
			SecurePort:        securePort,
			SSLCertificateRef: certSecretName,
			Instances:         instances,
		}
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

	if tlsEnable {
		// Expose the conventional https port externally, forwarding to the RGW
		// TLS container port.
		svc.Spec.Ports = []corev1.ServicePort{
			{
				Name:       "https",
				Port:       443,
				Protocol:   corev1.ProtocolTCP,
				TargetPort: intstr.FromInt(int(securePort)),
			},
		}
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
		// .rgw.root is the cluster-global realm metadata pool shared by every
		// object store. The operator sets pg_num_min=8 on it when reconciling a
		// normal CephObjectStore (e.g. the COSI test store), so it must have
		// pg_num>=8 or that reconcile fails with "pg_num_min 8 > pg_num 1". The
		// store-private pools are not shared and can stay at the minimal pg_num.
		pgNum := "1"
		if v == ".rgw.root" {
			pgNum = "8"
		}
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
						"pg_num":            pgNum,
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

	// the cert secret must exist before the store is created so the operator can
	// mount it when the rgw pod first reconciles
	if tlsEnable {
		client.GenerateRgwTLSCertSecret(t, k8sh, ns, certSecretName, rgwServiceName)
	}

	wait4.RequireCreate(ctx, t, ceph.CephObjectStores(ns), objectStore, wait4.ObjectStore,
		3*time.Minute, "shared CephObjectStore did not become Ready")

	{
		_, err := k8sh.Clientset.CoreV1().Services(ns).Create(ctx, svc, metav1.CreateOptions{})
		require.NoError(t, err)
	}

	t.Run("setup rgw admin api client", func(t *testing.T) {
		// user creation may be slow right after rgw start up
		wait4.RequireCreate(ctx, t, ceph.CephObjectStoreUsers(ns), canaryUser, wait4.ObjectStoreUser, 2*time.Minute)

		// cleanup the canary user immediately so it does not block the
		// CephObjectStore's deletion later
		wait4.AssertDelete(ctx, t, ceph.CephObjectStoreUsers(ns), canaryUser.Name, time.Minute)

		// the canary user becoming ready tells us the rgw admin api is ready
		var err error
		s.adminClient, err = client.NewAdminClient(objectStore, installer, k8sh, tlsEnable)
		require.NoError(t, err)

		s.snsClient, err = client.NewSNSClient(objectStore, k8sh, installer, tlsEnable)
		require.NoError(t, err)
	})

	s.objectStore = objectStore

	s.destroy = func() {
		t.Run(fmt.Sprintf("destroy CephObjectStore %s", objectStore.Name), func(t *testing.T) {
			wait4.AssertDelete(ctx, t, k8sh.Clientset.CoreV1().Services(ns), svc.Name, 30*time.Second)
			wait4.AssertDelete(ctx, t, ceph.CephObjectStores(ns), objectStore.Name, 5*time.Minute)

			// The multisite CRs must be deleted before the pools. Their deletion
			// finalizers run radosgw-admin, which reads the realm/zonegroup/zone
			// metadata stored in the .rgw.root pool. Deleting the pools first
			// removes that metadata, causing radosgw-admin to fail (exit status 2)
			// and the zone controller to loop forever, so the CephObjectZone CR
			// never finishes deleting.
			wait4.AssertDelete(ctx, t, ceph.CephObjectZones(ns), zone.Name, 2*time.Minute)
			wait4.AssertDelete(ctx, t, ceph.CephObjectZoneGroups(ns), zoneGroup.Name, time.Minute)
			wait4.AssertDelete(ctx, t, ceph.CephObjectRealms(ns), realm.Name, time.Minute)

			for _, p := range pools {
				wait4.AssertDelete(ctx, t, ceph.CephBlockPools(ns), p.Name, time.Minute)
			}

			if tlsEnable {
				wait4.AssertDelete(ctx, t, k8sh.Clientset.CoreV1().Secrets(ns), certSecretName, 30*time.Second)
			}
		})
	}

	return s
}
