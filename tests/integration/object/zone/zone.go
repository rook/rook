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

package zone

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	appsv1 "k8s.io/api/apps/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	cephv1 "github.com/rook/rook/pkg/apis/ceph.rook.io/v1"
	"github.com/rook/rook/tests/framework/utils"
	"github.com/rook/rook/tests/integration/object/util/multisite"
	"github.com/rook/rook/tests/integration/object/util/sharedstore"
	"github.com/rook/rook/tests/integration/object/util/wait4"
)

// TestCephObjectZoneDependents verifies a CephObjectZone's deletion is blocked
// while a CephObjectStore references it. The zone is deleted on its own first,
// then the realm, zone group and object store in one go, the way a kubectl
// delete of all their CR types does. Each CR waits for the one below it. Once
// the object store is gone the chain unwinds, and the zone, whose zone group is
// still there, deletes its pools. The test builds its own chain; it takes the
// shared store only to find the cluster namespace, which multisite CRs must
// live in, and to run ceph commands in the cluster.
func TestCephObjectZoneDependents(t *testing.T, k8sh *utils.K8sHelper, store *sharedstore.Sharedstore) {
	var (
		defaultName = "test-zone-dependents"
		namespace   = store.ObjectStore().Namespace

		realm, zoneGroup, zone = multisite.NewChain(defaultName, namespace)
		objectStore            = &cephv1.CephObjectStore{
			ObjectMeta: metav1.ObjectMeta{Name: defaultName + "-objectstore", Namespace: namespace},
			Spec: cephv1.ObjectStoreSpec{
				Zone:    cephv1.ZoneSpec{Name: zone.Name},
				Gateway: cephv1.GatewaySpec{Port: 80, Instances: 1},
			},
		}
		// The deployment of the object store's only gateway.
		rgwDeploymentName = "rook-ceph-rgw-" + objectStore.Name + "-a"

		realmClient      = k8sh.RookClientset.CephV1().CephObjectRealms(namespace)
		zoneGroupClient  = k8sh.RookClientset.CephV1().CephObjectZoneGroups(namespace)
		zoneClient       = k8sh.RookClientset.CephV1().CephObjectZones(namespace)
		cosClient        = k8sh.RookClientset.CephV1().CephObjectStores(namespace)
		deploymentClient = k8sh.Clientset.AppsV1().Deployments(namespace)
	)

	t.Run("CephObjectZone dependents", func(t *testing.T) {
		ctx := t.Context()

		// The scenario deletes its CRs itself. This fallback is for an abort
		// partway through; deleting all of them is enough, as the chain unwinds in
		// order on its own.
		t.Cleanup(func() {
			bg := context.Background()
			_ = cosClient.Delete(bg, objectStore.Name, metav1.DeleteOptions{})
			_ = zoneClient.Delete(bg, zone.Name, metav1.DeleteOptions{})
			_ = zoneGroupClient.Delete(bg, zoneGroup.Name, metav1.DeleteOptions{})
			_ = realmClient.Delete(bg, realm.Name, metav1.DeleteOptions{})
		})

		t.Run(fmt.Sprintf("create CephObjectRealm %q", realm.Name), func(t *testing.T) {
			wait4.RequireCreate(ctx, t, realmClient, realm, wait4.ObjectRealm, wait4.TimeoutMedium)
		})

		t.Run(fmt.Sprintf("create CephObjectZoneGroup %q", zoneGroup.Name), func(t *testing.T) {
			wait4.RequireCreate(ctx, t, zoneGroupClient, zoneGroup, wait4.ObjectZoneGroup, wait4.TimeoutMedium)
		})

		// The zone creates its pools before it becomes Ready.
		t.Run(fmt.Sprintf("create CephObjectZone %q", zone.Name), func(t *testing.T) {
			wait4.RequireCreate(ctx, t, zoneClient, zone, wait4.ObjectZone, wait4.TimeoutLong)
		})

		t.Run(fmt.Sprintf("create CephObjectStore %q", objectStore.Name), func(t *testing.T) {
			live := wait4.RequireCreate(ctx, t, cosClient, objectStore, wait4.ObjectStore, 3*time.Minute)
			assert.NotEmpty(t, live.Status.Info["endpoint"],
				"CephObjectStore %q became Ready without publishing an endpoint", objectStore.Name)
		})

		// RGW exits once while it starts (#18226). Waiting until the gateway is
		// ready gets past that, so the object store's deletion below can list the
		// store's buckets through the RGW right away, instead of failing and
		// retrying with backoff while the rest of the chain waits for it.
		t.Run(fmt.Sprintf("deployment %q is ready", rgwDeploymentName), func(t *testing.T) {
			wait4.RequireCondition(ctx, t, deploymentClient, rgwDeploymentName,
				func(d *appsv1.Deployment) bool { return d.Status.ReadyReplicas >= 1 }, 5*time.Minute)
		})

		t.Run(fmt.Sprintf("delete CephObjectZone %q", zone.Name), func(t *testing.T) {
			require.NoError(t, zoneClient.Delete(ctx, zone.Name, metav1.DeleteOptions{}))
		})

		// The zone controller names the object stores it waits for with the
		// singular kind. Nothing deletes the object store until the next step, so
		// the zone stays blocked until then.
		multisite.CheckDeletionBlocked(t, zoneClient, "CephObjectZone", zone.Name, wait4.ObjectZoneDeletionBlocked,
			"CephObjectStore", objectStore.Name)

		t.Run("delete the realm, zone group and object store at once", func(t *testing.T) {
			require.NoError(t, realmClient.Delete(ctx, realm.Name, metav1.DeleteOptions{}))
			require.NoError(t, zoneGroupClient.Delete(ctx, zoneGroup.Name, metav1.DeleteOptions{}))
			require.NoError(t, cosClient.Delete(ctx, objectStore.Name, metav1.DeleteOptions{}))
		})

		// The zone waits for the object store's deletion, then for its own next
		// requeue, 10s later, and then deletes its pools, while a blocked zone
		// group or realm checks its dependents again only every 10s. So both are
		// blocked long enough to be seen.
		multisite.CheckDeletionBlocked(t, zoneGroupClient, "CephObjectZoneGroup", zoneGroup.Name, wait4.ObjectZoneGroupDeletionBlocked,
			"CephObjectZones", zone.Name)
		multisite.CheckDeletionBlocked(t, realmClient, "CephObjectRealm", realm.Name, wait4.ObjectRealmDeletionBlocked,
			"CephObjectZoneGroups", zoneGroup.Name)

		t.Run(fmt.Sprintf("CephObjectStore %q is deleted", objectStore.Name), func(t *testing.T) {
			wait4.AssertAbsent(ctx, t, cosClient, objectStore.Name, wait4.TimeoutLong)
		})

		t.Run(fmt.Sprintf("CephObjectZone %q is deleted once its object stores are gone", zone.Name), func(t *testing.T) {
			wait4.AssertAbsent(ctx, t, zoneClient, zone.Name, wait4.TimeoutLong)
		})

		t.Run(fmt.Sprintf("CephObjectZoneGroup %q is deleted once its zones are gone", zoneGroup.Name), func(t *testing.T) {
			wait4.AssertAbsent(ctx, t, zoneGroupClient, zoneGroup.Name, wait4.TimeoutMedium)
		})

		t.Run(fmt.Sprintf("CephObjectRealm %q is deleted once its zone groups are gone", realm.Name), func(t *testing.T) {
			wait4.AssertAbsent(ctx, t, realmClient, realm.Name, wait4.TimeoutMedium)
		})

		// The zone group was still there while the zone was deleted, so the zone
		// could clean up in Ceph.
		multisite.CheckPoolsDeleted(t, store, zone.Name)
	})
}
