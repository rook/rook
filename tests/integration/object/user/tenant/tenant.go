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

package tenant

import (
	"fmt"
	"testing"

	"github.com/ceph/go-ceph/rgw/admin"
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

const Namespace = "test-usertenant"

func TestObjectStoreUserTenant(t *testing.T, k8sh *utils.K8sHelper, store *sharedstore.Sharedstore) {
	var (
		defaultName = Namespace
		objectStore = store.ObjectStore()
		adminClient = store.AdminClient()

		ns = &corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{
				Name: defaultName,
			},
		}

		tenant = "tenantA"

		osu1 = cephv1.CephObjectStoreUser{
			ObjectMeta: metav1.ObjectMeta{
				Name:      defaultName + "-user1",
				Namespace: ns.Name,
			},
			Spec: cephv1.ObjectStoreUserSpec{
				Store:            objectStore.Name,
				ClusterNamespace: objectStore.Namespace,
				Tenant:           tenant,
			},
		}

		osuClient = k8sh.RookClientset.CephV1().CephObjectStoreUsers(ns.Name)

		rgwUserID = tenant + "$" + osu1.Name
	)

	t.Run("ObjectStoreUser tenant", func(t *testing.T) {
		ctx := t.Context()

		fixture.RequireNamespace(t, k8sh, ns)

		t.Run(fmt.Sprintf("create CephObjectStoreUser %q", osu1.Name), func(t *testing.T) {
			// user creation may be slow right after rgw start up
			wait4.RequireCreate(ctx, t, osuClient, &osu1, wait4.ObjectStoreUser, wait4.TimeoutLong)
		})

		t.Run(fmt.Sprintf("rgw user is addressed as %q", rgwUserID), func(t *testing.T) {
			ctx := t.Context()

			liveUser, err := adminClient.GetUser(ctx, admin.User{ID: rgwUserID})
			require.NoError(t, err)

			assert.Equal(t, rgwUserID, liveUser.ID)
		})

		t.Run(fmt.Sprintf("bare name %q does not resolve to the tenanted user", osu1.Name), func(t *testing.T) {
			ctx := t.Context()

			_, err := adminClient.GetUser(ctx, admin.User{ID: osu1.Name})
			assert.ErrorIs(t, err, admin.ErrNoSuchUser)
		})

		t.Run(fmt.Sprintf("delete CephObjectStoreUser %q", osu1.Name), func(t *testing.T) {
			wait4.AssertDelete(ctx, t, osuClient, osu1.Name, wait4.TimeoutShort)
		})

		t.Run(fmt.Sprintf("no CephObjectStoreUsers in ns %q", ns.Name), func(t *testing.T) {
			osus, err := osuClient.List(ctx, metav1.ListOptions{})
			require.NoError(t, err)

			assert.Len(t, osus.Items, 0)
		})
	})
}
