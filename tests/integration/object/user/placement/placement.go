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

package placement

import (
	"context"
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

const Namespace = "test-userplacement"

func TestObjectStoreUserDefaultPlacement(t *testing.T, k8sh *utils.K8sHelper, store *sharedstore.Sharedstore) {
	var (
		defaultName = Namespace
		objectStore = store.ObjectStore()
		adminClient = store.AdminClient()

		ns = &corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{
				Name: defaultName,
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
				// "bar" is the non-default placement added to the shared store's
				// zone; the zonegroup default is "default".
				DefaultPlacement: "bar",
			},
		}

		osuClient = k8sh.RookClientset.CephV1().CephObjectStoreUsers(ns.Name)
	)

	t.Run("ObjectStoreUser defaultPlacement", func(t *testing.T) {
		ctx := t.Context()

		fixture.RequireNamespace(t, k8sh, ns)

		t.Run(fmt.Sprintf("create CephObjectStoreUser %q", osu1.Name), func(t *testing.T) {
			// user creation may be slow right after rgw start up
			wait4.RequireCreate(ctx, t, osuClient, &osu1, wait4.ObjectStoreUser, wait4.TimeoutLong)
		})

		checkUserPlacement(t, adminClient, osu1.Name, "bar")

		t.Run(fmt.Sprintf("CephObjectStoreUser %q reports its placement in status", osu1.Name), func(t *testing.T) {
			ctx := t.Context()

			liveOsu, err := osuClient.Get(ctx, osu1.Name, metav1.GetOptions{})
			require.NoError(t, err)
			require.NotNil(t, liveOsu.Status)

			assert.Equal(t, "bar", liveOsu.Status.Info["defaultPlacement"])
		})

		t.Run(fmt.Sprintf("update defaultPlacement on CephObjectStoreUser %q", osu1.Name), func(t *testing.T) {
			ctx := t.Context()

			liveOsu, err := osuClient.Get(ctx, osu1.Name, metav1.GetOptions{})
			require.NoError(t, err)

			liveOsu.Spec.DefaultPlacement = "default"

			_, err = osuClient.Update(ctx, liveOsu, metav1.UpdateOptions{})
			require.NoError(t, err)
		})

		checkUserPlacement(t, adminClient, osu1.Name, "default")

		t.Run(fmt.Sprintf("remove defaultPlacement from CephObjectStoreUser %q", osu1.Name), func(t *testing.T) {
			ctx := t.Context()

			liveOsu, err := osuClient.Get(ctx, osu1.Name, metav1.GetOptions{})
			require.NoError(t, err)

			liveOsu.Spec.DefaultPlacement = ""

			updatedOsu, err := osuClient.Update(ctx, liveOsu, metav1.UpdateOptions{})
			require.NoError(t, err)

			generation := updatedOsu.Generation
			wait4.RequireCondition(ctx, t, osuClient, osu1.Name, func(u *cephv1.CephObjectStoreUser) bool {
				return wait4.ObjectStoreUser(u) && u.Status.ObservedGeneration >= generation
			}, wait4.TimeoutShort)
		})

		t.Run(fmt.Sprintf("removing defaultPlacement leaves rgw user %q on %q", osu1.Name, "default"), func(t *testing.T) {
			ctx := t.Context()

			liveUser, err := adminClient.GetUser(ctx, admin.User{ID: osu1.Name})
			require.NoError(t, err)

			assert.Equal(t, "default", liveUser.DefaultPlacement)
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

func checkUserPlacement(t *testing.T, adminClient *admin.API, userName, placement string) {
	t.Run(fmt.Sprintf("rgw user %q has default placement %q", userName, placement), func(t *testing.T) {
		ctx := t.Context()

		var livePlacement string
		wait4.AssertEventually(ctx, t, wait4.TimeoutShort, fmt.Sprintf("rgw user %q default placement is %q", userName, placement), func(ctx context.Context) error {
			liveUser, err := adminClient.GetUser(ctx, admin.User{ID: userName})
			if err != nil {
				return err
			}

			if liveUser.DefaultPlacement != placement {
				return fmt.Errorf("default placement not yet %q: %q", placement, liveUser.DefaultPlacement)
			}

			livePlacement = liveUser.DefaultPlacement
			return nil
		})

		assert.Equal(t, placement, livePlacement)
	})
}
