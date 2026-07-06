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

package dependents

import (
	"context"
	"fmt"
	"testing"

	bktv1alpha1 "github.com/kube-object-storage/lib-bucket-provisioner/pkg/apis/objectbucket.io/v1alpha1"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	cephv1 "github.com/rook/rook/pkg/apis/ceph.rook.io/v1"
	"github.com/rook/rook/tests/framework/installer"
	"github.com/rook/rook/tests/framework/utils"
	"github.com/rook/rook/tests/integration/object/util/fixture"
	"github.com/rook/rook/tests/integration/object/util/obc"
	"github.com/rook/rook/tests/integration/object/util/sharedstore"
	"github.com/rook/rook/tests/integration/object/util/wait4"
)

const Namespace = "test-dependents"

// TestCephObjectStoreDependents verifies a CephObjectStore's deletion is blocked
// while it has dependents (a CephObjectStoreUser and an OBC bucket) and completes
// once they are gone. Deleting a store is destructive, so — unlike the other
// object packages — it does not take the shared fixture; it builds its own store
// with sharedstore.Create. That store coexists with the shared store in the same
// cluster namespace and is non-default-realm so it leaves the shared store's
// realm alone.
func TestCephObjectStoreDependents(t *testing.T, k8sh *utils.K8sHelper, installer *installer.CephInstaller, namespace string, tlsEnable bool) {
	t.Run("CephObjectStore dependents", func(t *testing.T) {
		ctx := t.Context()

		store := sharedstore.Create(t, k8sh, installer, tlsEnable, namespace, Namespace, 1, false)
		objectStore := store.ObjectStore()

		ns := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: Namespace}}
		storageClass := obc.StorageClass(Namespace, objectStore)

		user1 := &cephv1.CephObjectStoreUser{
			ObjectMeta: metav1.ObjectMeta{
				Name:      Namespace + "-user1",
				Namespace: namespace,
			},
			Spec: cephv1.ObjectStoreUserSpec{
				Store:       objectStore.Name,
				DisplayName: "dependents test user",
			},
		}

		obc1 := &bktv1alpha1.ObjectBucketClaim{
			ObjectMeta: metav1.ObjectMeta{
				Name:      Namespace + "-obc1",
				Namespace: ns.Name,
			},
			Spec: bktv1alpha1.ObjectBucketClaimSpec{
				BucketName:       Namespace + "-obc1",
				StorageClassName: storageClass.Name,
			},
		}

		obcClient := k8sh.BucketClientset.ObjectbucketV1alpha1().ObjectBucketClaims(ns.Name)
		cosClient := k8sh.RookClientset.CephV1().CephObjectStores(namespace)
		osuClient := k8sh.RookClientset.CephV1().CephObjectStoreUsers(namespace)

		// The scenario tears the store down itself, via Destroy, as its final
		// assertion. This fallback fires only if it aborts partway: it sweeps the
		// dependents so the store can delete, then tears the store and its
		// fixed-name multisite CRs down so they cannot strand into the other suite
		// pass. It uses Teardown, not Destroy, because Destroy's t.Run would panic
		// inside a t.Cleanup.
		torndown := false
		t.Cleanup(func() {
			if torndown {
				return
			}
			bg := context.Background()
			_ = obcClient.Delete(bg, obc1.Name, metav1.DeleteOptions{})
			_ = osuClient.Delete(bg, user1.Name, metav1.DeleteOptions{})
			store.Teardown(t)
		})

		fixture.RequireNamespace(t, k8sh, ns)
		fixture.RequireStorageClass(t, k8sh, storageClass)

		t.Run(fmt.Sprintf("create CephObjectStoreUser %q", user1.Name), func(t *testing.T) {
			wait4.RequireCreate(ctx, t, osuClient, user1, wait4.ObjectStoreUser, wait4.TimeoutLong)
		})

		obc.RequireBound(ctx, t, k8sh, obc1)

		t.Run(fmt.Sprintf("deleting CephObjectStore %q is blocked by its dependents", objectStore.Name), func(t *testing.T) {
			err := cosClient.Delete(ctx, objectStore.Name, metav1.DeleteOptions{})
			require.NoError(t, err)

			live := wait4.RequireCondition(ctx, t, cosClient, objectStore.Name, wait4.ObjectStoreDeletionBlocked, wait4.TimeoutShort)

			assert.Equal(t, cephv1.ConditionDeleting, live.Status.Phase)

			cond := cephv1.FindStatusCondition(live.Status.Conditions, cephv1.ConditionDeletionIsBlocked)
			require.NotNil(t, cond)
			assert.Equal(t, cephv1.ObjectHasDependentsReason, cond.Reason)
			assert.Contains(t, cond.Message, "CephObjectStoreUsers")
			assert.Contains(t, cond.Message, user1.Name)
			assert.Contains(t, cond.Message, "buckets")
			assert.Contains(t, cond.Message, obc1.Spec.BucketName)
		})

		t.Run(fmt.Sprintf("delete obc %q", obc1.Name), func(t *testing.T) {
			obc.DeleteAndWait(ctx, t, k8sh, ns.Name, obc1.Name)
		})

		t.Run(fmt.Sprintf("delete CephObjectStoreUser %q", user1.Name), func(t *testing.T) {
			wait4.RequireDelete(ctx, t, osuClient, user1.Name, wait4.TimeoutShort)
		})

		// With the dependents gone, the store — already terminating from the
		// blocked delete above — finishes deleting. Destroy asserts that and tears
		// down the realm/zone/pools/service it leaves behind.
		store.Destroy()
		torndown = true

		t.Run("mgrs are not in a crashloop", func(t *testing.T) {
			assert.True(t, k8sh.CheckPodCountAndState("rook-ceph-mgr", namespace, 1, "Running"))
		})
	})
}
