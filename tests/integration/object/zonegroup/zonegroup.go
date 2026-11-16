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

package zonegroup

import (
	"context"
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/rook/rook/tests/framework/utils"
	"github.com/rook/rook/tests/integration/object/util/multisite"
	"github.com/rook/rook/tests/integration/object/util/sharedstore"
	"github.com/rook/rook/tests/integration/object/util/wait4"
)

// TestCephObjectZoneGroupDependents verifies a CephObjectZoneGroup's deletion
// is blocked while a CephObjectZone references it, and completes on its own
// once the zone is gone. It deletes one chain one CR at a time, and another
// all at once, the way a kubectl delete of all their CR types does. The test
// builds its own chains; it takes the shared store only to find the cluster
// namespace, which multisite CRs must live in, and to run ceph commands in the
// cluster.
func TestCephObjectZoneGroupDependents(t *testing.T, k8sh *utils.K8sHelper, store *sharedstore.Sharedstore) {
	var (
		namespace = store.ObjectStore().Namespace

		realm, zoneGroup, zone             = multisite.NewChain("test-zonegroup-dependents", namespace)
		bulkRealm, bulkZoneGroup, bulkZone = multisite.NewChain("test-zonegroup-bulk", namespace)

		realmClient     = k8sh.RookClientset.CephV1().CephObjectRealms(namespace)
		zoneGroupClient = k8sh.RookClientset.CephV1().CephObjectZoneGroups(namespace)
		zoneClient      = k8sh.RookClientset.CephV1().CephObjectZones(namespace)
	)

	t.Run("CephObjectZoneGroup dependents", func(t *testing.T) {
		ctx := t.Context()

		// The scenarios delete their CRs themselves. This fallback is for an abort
		// partway through; deleting all of them is enough, as each chain unwinds
		// in order on its own.
		t.Cleanup(func() {
			bg := context.Background()
			for _, name := range []string{zone.Name, bulkZone.Name} {
				_ = zoneClient.Delete(bg, name, metav1.DeleteOptions{})
			}
			for _, name := range []string{zoneGroup.Name, bulkZoneGroup.Name} {
				_ = zoneGroupClient.Delete(bg, name, metav1.DeleteOptions{})
			}
			for _, name := range []string{realm.Name, bulkRealm.Name} {
				_ = realmClient.Delete(bg, name, metav1.DeleteOptions{})
			}
		})

		t.Run("one at a time", func(t *testing.T) {
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

			t.Run(fmt.Sprintf("delete CephObjectZoneGroup %q", zoneGroup.Name), func(t *testing.T) {
				require.NoError(t, zoneGroupClient.Delete(ctx, zoneGroup.Name, metav1.DeleteOptions{}))
			})

			multisite.CheckDeletionBlocked(t, zoneGroupClient, "CephObjectZoneGroup", zoneGroup.Name, wait4.ObjectZoneGroupDeletionBlocked,
				"CephObjectZones", zone.Name)

			// The zone has no object stores, so nothing holds it back. It still finds
			// its terminating zone group, so it deletes its pools and the Ceph zone.
			t.Run(fmt.Sprintf("delete CephObjectZone %q", zone.Name), func(t *testing.T) {
				wait4.RequireDelete(ctx, t, zoneClient, zone.Name, wait4.TimeoutLong)
			})

			// The blocked zone group requeues on a fixed interval and is released on
			// the first reconcile that finds no zone referencing it.
			t.Run(fmt.Sprintf("CephObjectZoneGroup %q is deleted once its zones are gone", zoneGroup.Name), func(t *testing.T) {
				wait4.AssertAbsent(ctx, t, zoneGroupClient, zoneGroup.Name, wait4.TimeoutMedium)
			})

			t.Run(fmt.Sprintf("delete CephObjectRealm %q", realm.Name), func(t *testing.T) {
				wait4.AssertDelete(ctx, t, realmClient, realm.Name, wait4.TimeoutMedium)
			})

			// The zone group was still there while the zone was deleted, so the zone
			// could clean up in Ceph.
			multisite.CheckPoolsDeleted(t, store, zone.Name)
		})

		t.Run("all at once", func(t *testing.T) {
			t.Run(fmt.Sprintf("create CephObjectRealm %q", bulkRealm.Name), func(t *testing.T) {
				wait4.RequireCreate(ctx, t, realmClient, bulkRealm, wait4.ObjectRealm, wait4.TimeoutMedium)
			})

			t.Run(fmt.Sprintf("create CephObjectZoneGroup %q", bulkZoneGroup.Name), func(t *testing.T) {
				wait4.RequireCreate(ctx, t, zoneGroupClient, bulkZoneGroup, wait4.ObjectZoneGroup, wait4.TimeoutMedium)
			})

			// The zone creates its pools before it becomes Ready.
			t.Run(fmt.Sprintf("create CephObjectZone %q", bulkZone.Name), func(t *testing.T) {
				wait4.RequireCreate(ctx, t, zoneClient, bulkZone, wait4.ObjectZone, wait4.TimeoutLong)
			})

			t.Run("delete the realm, zone group and zone at once", func(t *testing.T) {
				require.NoError(t, realmClient.Delete(ctx, bulkRealm.Name, metav1.DeleteOptions{}))
				require.NoError(t, zoneGroupClient.Delete(ctx, bulkZoneGroup.Name, metav1.DeleteOptions{}))
				require.NoError(t, zoneClient.Delete(ctx, bulkZone.Name, metav1.DeleteOptions{}))
			})

			// Nothing holds the zone back, but it takes several seconds to delete
			// its pools, and a blocked zone group or realm checks its dependents
			// again only on its next requeue, 10s later. So both are blocked long
			// enough to be seen.
			multisite.CheckDeletionBlocked(t, realmClient, "CephObjectRealm", bulkRealm.Name, wait4.ObjectRealmDeletionBlocked,
				"CephObjectZoneGroups", bulkZoneGroup.Name)
			multisite.CheckDeletionBlocked(t, zoneGroupClient, "CephObjectZoneGroup", bulkZoneGroup.Name, wait4.ObjectZoneGroupDeletionBlocked,
				"CephObjectZones", bulkZone.Name)

			t.Run(fmt.Sprintf("CephObjectZone %q is deleted", bulkZone.Name), func(t *testing.T) {
				wait4.AssertAbsent(ctx, t, zoneClient, bulkZone.Name, wait4.TimeoutLong)
			})

			t.Run(fmt.Sprintf("CephObjectZoneGroup %q is deleted once its zones are gone", bulkZoneGroup.Name), func(t *testing.T) {
				wait4.AssertAbsent(ctx, t, zoneGroupClient, bulkZoneGroup.Name, wait4.TimeoutMedium)
			})

			t.Run(fmt.Sprintf("CephObjectRealm %q is deleted once its zone groups are gone", bulkRealm.Name), func(t *testing.T) {
				wait4.AssertAbsent(ctx, t, realmClient, bulkRealm.Name, wait4.TimeoutMedium)
			})

			// The zone group was still there while the zone was deleted, so the zone
			// could clean up in Ceph.
			multisite.CheckPoolsDeleted(t, store, bulkZone.Name)
		})
	})
}
