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

package realm

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

// TestCephObjectRealmDependents verifies a CephObjectRealm's deletion is
// blocked while a CephObjectZoneGroup references it, and completes on its own
// once the zone group is gone. The test builds its own realm and zone group; it
// takes the shared store only to find the cluster namespace, which multisite
// CRs must live in.
func TestCephObjectRealmDependents(t *testing.T, k8sh *utils.K8sHelper, store *sharedstore.Sharedstore) {
	var (
		namespace = store.ObjectStore().Namespace

		// The realm waits for its zone group alone, so the chain's zone is not
		// created.
		realm, zoneGroup, _ = multisite.NewChain("test-realm-dependents", namespace)

		realmClient     = k8sh.RookClientset.CephV1().CephObjectRealms(namespace)
		zoneGroupClient = k8sh.RookClientset.CephV1().CephObjectZoneGroups(namespace)
	)

	t.Run("CephObjectRealm dependents", func(t *testing.T) {
		ctx := t.Context()

		// The scenario deletes both CRs itself. This fallback is for an abort
		// partway through; deleting both is enough, as the realm is released
		// once the zone group is gone.
		t.Cleanup(func() {
			bg := context.Background()
			_ = zoneGroupClient.Delete(bg, zoneGroup.Name, metav1.DeleteOptions{})
			_ = realmClient.Delete(bg, realm.Name, metav1.DeleteOptions{})
		})

		t.Run(fmt.Sprintf("create CephObjectRealm %q", realm.Name), func(t *testing.T) {
			wait4.RequireCreate(ctx, t, realmClient, realm, wait4.ObjectRealm, wait4.TimeoutMedium)
		})

		t.Run(fmt.Sprintf("create CephObjectZoneGroup %q", zoneGroup.Name), func(t *testing.T) {
			wait4.RequireCreate(ctx, t, zoneGroupClient, zoneGroup, wait4.ObjectZoneGroup, wait4.TimeoutMedium)
		})

		t.Run(fmt.Sprintf("delete CephObjectRealm %q", realm.Name), func(t *testing.T) {
			require.NoError(t, realmClient.Delete(ctx, realm.Name, metav1.DeleteOptions{}))
		})

		multisite.CheckDeletionBlocked(t, realmClient, "CephObjectRealm", realm.Name, wait4.ObjectRealmDeletionBlocked,
			"CephObjectZoneGroups", zoneGroup.Name)

		// The zone group has no zones, so nothing holds it back.
		t.Run(fmt.Sprintf("delete CephObjectZoneGroup %q", zoneGroup.Name), func(t *testing.T) {
			wait4.RequireDelete(ctx, t, zoneGroupClient, zoneGroup.Name, wait4.TimeoutShort)
		})

		// The blocked realm requeues on a fixed interval and is released on the
		// first reconcile that finds no zone group referencing it.
		t.Run(fmt.Sprintf("CephObjectRealm %q is deleted once its zone groups are gone", realm.Name), func(t *testing.T) {
			wait4.AssertAbsent(ctx, t, realmClient, realm.Name, wait4.TimeoutMedium)
		})
	})
}
