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

package caps

import (
	"context"
	"fmt"
	"testing"

	"github.com/ceph/go-ceph/rgw/admin"
	"github.com/google/go-cmp/cmp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	cephv1 "github.com/rook/rook/pkg/apis/ceph.rook.io/v1"
	"github.com/rook/rook/tests/framework/utils"
	"github.com/rook/rook/tests/integration/object/util/fixture"
	"github.com/rook/rook/tests/integration/object/util/sharedstore"
	"github.com/rook/rook/tests/integration/object/util/wait4"
)

const Namespace = "test-usercaps" // a namespace other than the object store's

func TestObjectStoreUserCaps(t *testing.T, k8sh *utils.K8sHelper, store *sharedstore.Sharedstore) {
	var (
		objectStore = store.ObjectStore()
		adminClient = store.AdminClient()
		storeNS     = objectStore.Namespace

		otherNS = &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: Namespace}}

		// A user in the object store's own namespace; capabilities are always
		// permitted, so this exercises capability propagation to RGW
		// independent of the namespaced-caps setting.
		sameNSUser = cephv1.CephObjectStoreUser{
			ObjectMeta: metav1.ObjectMeta{Name: "test-usercaps-samens", Namespace: storeNS},
			Spec:       cephv1.ObjectStoreUserSpec{Store: objectStore.Name},
		}
		sameNSUserClient = k8sh.RookClientset.CephV1().CephObjectStoreUsers(storeNS)

		// A user in another namespace, created via allowUsersInNamespaces.
		otherNSUser = cephv1.CephObjectStoreUser{
			ObjectMeta: metav1.ObjectMeta{Name: "test-usercaps-otherns", Namespace: Namespace},
			Spec:       cephv1.ObjectStoreUserSpec{Store: objectStore.Name, ClusterNamespace: storeNS},
		}
		otherNSUserClient = k8sh.RookClientset.CephV1().CephObjectStoreUsers(Namespace)
	)

	t.Run("ObjectStoreUser caps", func(t *testing.T) {
		ctx := t.Context()

		// capability propagation works for a user in the store's own namespace
		t.Run("create user in store namespace", func(t *testing.T) {
			wait4.RequireCreate(ctx, t, sameNSUserClient, &sameNSUser, wait4.ObjectStoreUser, wait4.TimeoutLong)
		})

		t.Run("store-namespace user caps default to empty", func(t *testing.T) {
			liveUser, err := adminClient.GetUser(ctx, admin.User{ID: sameNSUser.Name})
			require.NoError(t, err)
			assert.Empty(t, liveUser.Caps)
		})

		t.Run("grant caps to store-namespace user", func(t *testing.T) {
			live, err := sameNSUserClient.Get(ctx, sameNSUser.Name, metav1.GetOptions{})
			require.NoError(t, err)
			live.Spec.Capabilities = &cephv1.ObjectUserCapSpec{Buckets: "*", Usage: "read"}
			_, err = sameNSUserClient.Update(ctx, live, metav1.UpdateOptions{})
			require.NoError(t, err)
		})

		t.Run("store-namespace user caps sync to rgw", func(t *testing.T) {
			wait4.AssertEventually(ctx, t, wait4.TimeoutShort, "rgw user caps match spec", func(ctx context.Context) error {
				liveUser, err := adminClient.GetUser(ctx, admin.User{ID: sameNSUser.Name})
				if err != nil {
					return err
				}
				if !cmp.Equal(liveUser.Caps, []admin.UserCapSpec{
					{Type: "buckets", Perm: "*"},
					{Type: "usage", Perm: "read"},
				}) {
					return fmt.Errorf("caps not yet in sync: %+v", liveUser.Caps)
				}
				return nil
			})
		})

		t.Run("delete store-namespace user", func(t *testing.T) {
			wait4.AssertDelete(ctx, t, sameNSUserClient, sameNSUser.Name, wait4.TimeoutShort)
		})

		// a user in another namespace must not be granted store-wide admin caps
		// by default
		fixture.RequireNamespace(t, k8sh, otherNS)

		t.Run("create user in another namespace without caps", func(t *testing.T) {
			wait4.RequireCreate(ctx, t, otherNSUserClient, &otherNSUser, wait4.ObjectStoreUser, wait4.TimeoutLong)
		})

		t.Run("granting admin caps to a user in another namespace is rejected", func(t *testing.T) {
			live, err := otherNSUserClient.Get(ctx, otherNSUser.Name, metav1.GetOptions{})
			require.NoError(t, err)
			live.Spec.Capabilities = &cephv1.ObjectUserCapSpec{Users: "*"}
			_, err = otherNSUserClient.Update(ctx, live, metav1.UpdateOptions{})
			require.NoError(t, err)

			// the operator refuses the spec; the CR reports ReconcileFailed
			wait4.RequireCondition(ctx, t, otherNSUserClient, otherNSUser.Name,
				wait4.ObjectStoreUserPhase(string(cephv1.ReconcileFailed)), wait4.TimeoutShort)

			// and the caps are never applied to the live rgw user
			liveUser, err := adminClient.GetUser(ctx, admin.User{ID: otherNSUser.Name})
			require.NoError(t, err)
			assert.Empty(t, liveUser.Caps)
		})

		t.Run("delete user in another namespace", func(t *testing.T) {
			wait4.AssertDelete(ctx, t, otherNSUserClient, otherNSUser.Name, wait4.TimeoutShort)
		})
	})
}
