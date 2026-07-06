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
	"fmt"
	"testing"
	"time"

	bktv1alpha1 "github.com/kube-object-storage/lib-bucket-provisioner/pkg/apis/objectbucket.io/v1alpha1"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	cephv1 "github.com/rook/rook/pkg/apis/ceph.rook.io/v1"
	"github.com/rook/rook/tests/framework/utils"
	"github.com/rook/rook/tests/integration/object/util/fixture"
	"github.com/rook/rook/tests/integration/object/util/obc"
	"github.com/rook/rook/tests/integration/object/util/sharedstore"
	"github.com/rook/rook/tests/integration/object/util/wait4"
)

const Namespace = "test-dependents"

func TestCephObjectStoreDependents(t *testing.T, k8sh *utils.K8sHelper, store *sharedstore.Sharedstore) {
	var (
		defaultName = Namespace
		clusterNS   = store.ObjectStore().Namespace

		ns = &corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{
				Name: defaultName,
			},
		}

		// deleting a store is destructive, so these tests get a private store
		// rather than the shared fixture
		privateStore = &cephv1.CephObjectStore{
			ObjectMeta: metav1.ObjectMeta{
				Name:      defaultName,
				Namespace: clusterNS,
			},
			Spec: cephv1.ObjectStoreSpec{
				MetadataPool: cephv1.PoolSpec{
					Replicated:      cephv1.ReplicatedSpec{Size: 1, RequireSafeReplicaSize: false},
					CompressionMode: "passive",
				},
				DataPool: cephv1.PoolSpec{
					Replicated: cephv1.ReplicatedSpec{Size: 1, RequireSafeReplicaSize: false},
				},
				Gateway: cephv1.GatewaySpec{
					Port:      80,
					Instances: 1,
				},
			},
		}

		storageClass = obc.StorageClass(defaultName, privateStore)

		user1 = &cephv1.CephObjectStoreUser{
			ObjectMeta: metav1.ObjectMeta{
				Name:      defaultName + "-user1",
				Namespace: clusterNS,
			},
			Spec: cephv1.ObjectStoreUserSpec{
				Store:       privateStore.Name,
				DisplayName: "dependents test user",
			},
		}

		obc1 = &bktv1alpha1.ObjectBucketClaim{
			ObjectMeta: metav1.ObjectMeta{
				Name:      defaultName + "-obc1",
				Namespace: ns.Name,
			},
			Spec: bktv1alpha1.ObjectBucketClaimSpec{
				BucketName:       defaultName + "-obc1",
				StorageClassName: storageClass.Name,
			},
		}

		cosClient = k8sh.RookClientset.CephV1().CephObjectStores(clusterNS)
		osuClient = k8sh.RookClientset.CephV1().CephObjectStoreUsers(clusterNS)
	)

	t.Run("CephObjectStore dependents", func(t *testing.T) {
		ctx := t.Context()

		fixture.RequireNamespace(t, k8sh, ns)
		fixture.RequireStorageClass(t, k8sh, storageClass)

		t.Run(fmt.Sprintf("create CephObjectStore %q", privateStore.Name), func(t *testing.T) {
			// store creation races rgw startup; same bespoke timeout as the
			// shared store fixture
			wait4.RequireCreate(ctx, t, cosClient, privateStore, wait4.ObjectStore, 3*time.Minute)
		})

		t.Run(fmt.Sprintf("create CephObjectStoreUser %q", user1.Name), func(t *testing.T) {
			wait4.RequireCreate(ctx, t, osuClient, user1, wait4.ObjectStoreUser, wait4.TimeoutLong)
		})

		obc.RequireBound(ctx, t, k8sh, obc1)

		t.Run(fmt.Sprintf("deleting CephObjectStore %q is blocked by its dependents", privateStore.Name), func(t *testing.T) {
			err := cosClient.Delete(ctx, privateStore.Name, metav1.DeleteOptions{})
			require.NoError(t, err)

			live := wait4.RequireCondition(ctx, t, cosClient, privateStore.Name, wait4.ObjectStoreDeletionBlocked, wait4.TimeoutShort)

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

		t.Run(fmt.Sprintf("CephObjectStore %q deletes once its dependents are gone", privateStore.Name), func(t *testing.T) {
			wait4.RequireAbsent(ctx, t, cosClient, privateStore.Name, 3*time.Minute)
		})

		t.Run("mgrs are not in a crashloop", func(t *testing.T) {
			assert.True(t, k8sh.CheckPodCountAndState("rook-ceph-mgr", clusterNS, 1, "Running"))
		})
	})
}
