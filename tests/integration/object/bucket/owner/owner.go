/*
Copyright 2025 The Rook Authors. All rights reserved.

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

package owner

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/ceph/go-ceph/rgw/admin"
	bktv1alpha1 "github.com/kube-object-storage/lib-bucket-provisioner/pkg/apis/objectbucket.io/v1alpha1"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"

	cephv1 "github.com/rook/rook/pkg/apis/ceph.rook.io/v1"
	"github.com/rook/rook/tests/framework/utils"
	"github.com/rook/rook/tests/integration/object/util/fixture"
	"github.com/rook/rook/tests/integration/object/util/obc"
	"github.com/rook/rook/tests/integration/object/util/sharedstore"
	"github.com/rook/rook/tests/integration/object/util/wait4"
)

const Namespace = "test-bucketowner"

func TestObjectBucketClaimBucketOwner(t *testing.T, k8sh *utils.K8sHelper, store *sharedstore.Sharedstore) {
	var (
		defaultName = Namespace
		objectStore = store.ObjectStore()
		adminClient = store.AdminClient()

		ns = &corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{
				Name: defaultName,
			},
		}

		storageClass = obc.StorageClass(defaultName, objectStore)

		// test user without any quotas set
		osu1 = cephv1.CephObjectStoreUser{
			ObjectMeta: metav1.ObjectMeta{
				Name:      defaultName + "-user1",
				Namespace: ns.Name,
			},
			Spec: cephv1.ObjectStoreUserSpec{
				Store:            objectStore.Name,
				ClusterNamespace: objectStore.Namespace,
			},
		}

		// test user with quotas set
		osu2 = cephv1.CephObjectStoreUser{
			ObjectMeta: metav1.ObjectMeta{
				Name:      defaultName + "-user2",
				Namespace: ns.Name,
			},
			Spec: cephv1.ObjectStoreUserSpec{
				Store:            objectStore.Name,
				ClusterNamespace: objectStore.Namespace,
				Quotas: &cephv1.ObjectUserQuotaSpec{
					MaxBuckets: func(i int) *int { return &i }(1111),
					MaxSize:    resource.NewQuantity(2222, resource.DecimalSI),
					MaxObjects: func(i int64) *int64 { return &i }(3333),
				},
			},
		}

		obc1 = bktv1alpha1.ObjectBucketClaim{
			ObjectMeta: metav1.ObjectMeta{
				Name:      defaultName + "-obc1",
				Namespace: ns.Name,
			},
			Spec: bktv1alpha1.ObjectBucketClaimSpec{
				BucketName:       defaultName + "-obc1",
				StorageClassName: storageClass.Name,
				AdditionalConfig: map[string]string{
					"bucketOwner": osu1.Name,
				},
			},
		}

		obc2 = bktv1alpha1.ObjectBucketClaim{
			ObjectMeta: metav1.ObjectMeta{
				Name:      defaultName + "-obc2",
				Namespace: ns.Name,
			},
			Spec: bktv1alpha1.ObjectBucketClaimSpec{
				BucketName:       defaultName + "-obc2",
				StorageClassName: storageClass.Name,
				AdditionalConfig: map[string]string{
					"bucketOwner": osu1.Name,
				},
			},
		}

		obcBogusOwner = bktv1alpha1.ObjectBucketClaim{
			ObjectMeta: metav1.ObjectMeta{
				Name:      defaultName + "-bogus-owner",
				Namespace: ns.Name,
			},
			Spec: bktv1alpha1.ObjectBucketClaimSpec{
				BucketName:       defaultName,
				StorageClassName: storageClass.Name,
				AdditionalConfig: map[string]string{
					"bucketOwner": defaultName + "-bogus-user",
				},
			},
		}

		osuClient = k8sh.RookClientset.CephV1().CephObjectStoreUsers(ns.Name)
		obcClient = k8sh.BucketClientset.ObjectbucketV1alpha1().ObjectBucketClaims(ns.Name)
		obClient  = k8sh.BucketClientset.ObjectbucketV1alpha1().ObjectBuckets()
	)
	t.Run("OBC bucketOwner", func(t *testing.T) {
		ctx := t.Context()

		fixture.RequireNamespace(t, k8sh, ns)
		fixture.RequireStorageClass(t, k8sh, storageClass)

		// since this is an obc specific subtest we assume that CephObjectStoreUser
		// is working and the rgw service state does not need to be inspected to
		// confirm user creation.
		t.Run(fmt.Sprintf("create CephObjectStoreUser %q", osu1.Name), func(t *testing.T) {
			// user creation may be slow right after rgw start up
			wait4.RequireCreate(ctx, t, osuClient, &osu1, wait4.ObjectStoreUser, wait4.TimeoutLong)
		})

		t.Run(fmt.Sprintf("create obc %q with bucketOwner %q", obc1.Name, obc1.Spec.AdditionalConfig["bucketOwner"]), func(t *testing.T) {
			_, err := obcClient.Create(ctx, &obc1, metav1.CreateOptions{})
			require.NoError(t, err)
		})

		t.Run(fmt.Sprintf("obc %q has bucketOwner %q set", obc1.Name, obc1.Spec.AdditionalConfig["bucketOwner"]), func(t *testing.T) {
			liveObc := wait4.RequireCondition(ctx, t, obcClient, obc1.Name, wait4.OBCBound, wait4.TimeoutShort)

			// verify that bucketOwner is set on the live obc
			assert.Equal(t, obc1.Spec.AdditionalConfig["bucketOwner"], liveObc.Spec.AdditionalConfig["bucketOwner"])
		})

		t.Run(fmt.Sprintf("ob for obc %q has bucketOwner %q set", obc1.Name, obc1.Spec.AdditionalConfig["bucketOwner"]), func(t *testing.T) {
			liveObc, err := obcClient.Get(ctx, obc1.Name, metav1.GetOptions{})
			require.NoError(t, err)
			obName := liveObc.Spec.ObjectBucketName

			liveOb := wait4.RequireCondition(ctx, t, obClient, obName, wait4.OBBound, wait4.TimeoutShort)

			// verify that bucketOwner is set on the live ob
			assert.Equal(t, obc1.Spec.AdditionalConfig["bucketOwner"], liveOb.Spec.Connection.AdditionalState["bucketOwner"])
		})

		t.Run(fmt.Sprintf("bucket created with owner %q", obc1.Spec.AdditionalConfig["bucketOwner"]), func(t *testing.T) {
			bucket, err := adminClient.GetBucketInfo(ctx, admin.Bucket{Bucket: obc1.Spec.BucketName})
			require.NoError(t, err)

			assert.Equal(t, obc1.Spec.AdditionalConfig["bucketOwner"], bucket.Owner)
		})

		// obc should not modify pre-existing users
		t.Run(fmt.Sprintf("no user quota was set on %q", osu1.Name), func(t *testing.T) {
			liveUser, err := adminClient.GetUser(ctx, admin.User{ID: osu1.Name})
			require.NoError(t, err)

			assert.Equal(t, int(1000), *liveUser.MaxBuckets)
			assert.False(t, *liveUser.UserQuota.Enabled)
			assert.Equal(t, int64(-1), *liveUser.UserQuota.MaxSize)
			assert.Equal(t, int64(-1), *liveUser.UserQuota.MaxObjects)
		})

		t.Run(fmt.Sprintf("create CephObjectStoreUser %q", osu2.Name), func(t *testing.T) {
			// create user2
			wait4.RequireCreate(ctx, t, osuClient, &osu2, wait4.ObjectStoreUser, wait4.TimeoutLong)
		})

		t.Run(fmt.Sprintf("update obc %q to bucketOwner %q", obc1.Name, osu2.Name), func(t *testing.T) {
			// update obc bucketOwner
			liveObc, err := obcClient.Get(ctx, obc1.Name, metav1.GetOptions{})
			require.NoError(t, err)

			liveObc.Spec.AdditionalConfig["bucketOwner"] = osu2.Name

			_, err = obcClient.Update(ctx, liveObc, metav1.UpdateOptions{})
			require.NoError(t, err)
		})

		t.Run(fmt.Sprintf("obc %q has bucketOwner %q set", obc1.Name, osu2.Name), func(t *testing.T) {
			// obc .Status.Phase does not appear to change when updating the obc
			liveObc, err := obcClient.Get(ctx, obc1.Name, metav1.GetOptions{})
			require.NoError(t, err)

			// verify that bucketOwner is set on the live obc
			assert.Equal(t, osu2.Name, liveObc.Spec.AdditionalConfig["bucketOwner"])
		})

		t.Run(fmt.Sprintf("ob for obc %q has bucketOwner %q set", obc1.Name, osu2.Name), func(t *testing.T) {
			// ob .Status.Phase does not appear to change when updating the obc
			liveObc, err := obcClient.Get(ctx, obc1.Name, metav1.GetOptions{})
			require.NoError(t, err)
			obName := liveObc.Spec.ObjectBucketName

			liveOb := wait4.RequireCondition(ctx, t, obClient, obName,
				func(ob *bktv1alpha1.ObjectBucket) bool {
					return ob.Spec.Connection.AdditionalState["bucketOwner"] == osu2.Name
				},
				wait4.TimeoutShort)

			// verify that bucketOwner is set on the live ob
			assert.Equal(t, osu2.Name, liveOb.Spec.Connection.AdditionalState["bucketOwner"])
		})

		t.Run(fmt.Sprintf("bucket owner changed to %q", osu2.Name), func(t *testing.T) {
			wait4.AssertEventually(ctx, t, wait4.TimeoutShort, fmt.Sprintf("bucket %q owner is %q", obc1.Name, osu2.Name), func(ctx context.Context) error {
				bucket, err := adminClient.GetBucketInfo(ctx, admin.Bucket{Bucket: obc1.Name})
				if err != nil {
					return err
				}

				if bucket.Owner != osu2.Name {
					return fmt.Errorf("bucket owner is %q, want %q", bucket.Owner, osu2.Name)
				}
				return nil
			})
		})

		// obc should not modify pre-existing users
		t.Run(fmt.Sprintf("existing user quota on %q has not changed", osu2.Name), func(t *testing.T) {
			liveUser, err := adminClient.GetUser(ctx, admin.User{ID: osu2.Name})
			require.NoError(t, err)

			assert.Equal(t, int(1111), *liveUser.MaxBuckets)
			assert.True(t, *liveUser.UserQuota.Enabled)
			assert.Equal(t, int64(2222), *liveUser.UserQuota.MaxSize)
			assert.Equal(t, int64(3333), *liveUser.UserQuota.MaxObjects)
		})

		t.Run(fmt.Sprintf("remove obc %q bucketOwner", obc1.Name), func(t *testing.T) {
			// update/remove obc bucketOwner
			liveObc, err := obcClient.Get(ctx, obc1.Name, metav1.GetOptions{})
			require.NoError(t, err)

			liveObc.Spec.AdditionalConfig = map[string]string{}

			_, err = obcClient.Update(ctx, liveObc, metav1.UpdateOptions{})
			require.NoError(t, err)
		})

		t.Run(fmt.Sprintf("obc %q has no bucketOwner", obc1.Name), func(t *testing.T) {
			// verify that bucketOwner is unset on the live obc
			wait4.AssertCondition(ctx, t, obcClient, obc1.Name,
				func(liveObc *bktv1alpha1.ObjectBucketClaim) bool {
					_, ok := liveObc.Spec.AdditionalConfig["bucketOwner"]
					return !ok
				},
				wait4.TimeoutShort)
		})

		t.Run(fmt.Sprintf("ob for obc %q has no bucketOwner", obc1.Name), func(t *testing.T) {
			liveObc, err := obcClient.Get(ctx, obc1.Name, metav1.GetOptions{})
			require.NoError(t, err)
			obName := liveObc.Spec.ObjectBucketName

			wait4.AssertCondition(ctx, t, obClient, obName,
				func(ob *bktv1alpha1.ObjectBucket) bool {
					_, ok := ob.Spec.Connection.AdditionalState["bucketOwner"]
					return !ok
				},
				wait4.TimeoutShort)
		})

		// the ob should retain the existing owner and not revert to a generated user
		t.Run(fmt.Sprintf("bucket owner is still %q", osu2.Name), func(t *testing.T) {
			wait4.AssertEventually(ctx, t, wait4.TimeoutShort, fmt.Sprintf("bucket %q owner is still %q", obc1.Name, osu2.Name), func(ctx context.Context) error {
				bucket, err := adminClient.GetBucketInfo(ctx, admin.Bucket{Bucket: obc1.Name})
				if err != nil {
					return err
				}

				if bucket.Owner != osu2.Name {
					return fmt.Errorf("bucket owner is %q, want %q", bucket.Owner, osu2.Name)
				}
				return nil
			})
		})

		// this covers setting bucketOwner on an obc initially created without an explicit owner
		t.Run(fmt.Sprintf("update obc %q to bucketOwner %q", obc1.Name, osu1.Name), func(t *testing.T) {
			// update obc bucketOwner
			liveObc, err := obcClient.Get(ctx, obc1.Name, metav1.GetOptions{})
			require.NoError(t, err)

			liveObc.Spec.AdditionalConfig = map[string]string{"bucketOwner": osu1.Name}

			_, err = obcClient.Update(ctx, liveObc, metav1.UpdateOptions{})
			require.NoError(t, err)
		})

		t.Run(fmt.Sprintf("obc %q has bucketOwner %q set", obc1.Name, osu1.Name), func(t *testing.T) {
			// obc .Status.Phase does not appear to change when updating the obc
			liveObc, err := obcClient.Get(ctx, obc1.Name, metav1.GetOptions{})
			require.NoError(t, err)

			// verify that bucketOwner is set on the live obc
			assert.Equal(t, osu1.Name, liveObc.Spec.AdditionalConfig["bucketOwner"])
		})

		t.Run(fmt.Sprintf("ob for obc %q has bucketOwner %q set", obc1.Name, osu1.Name), func(t *testing.T) {
			// ob .Status.Phase does not appear to change when updating the obc
			liveObc, err := obcClient.Get(ctx, obc1.Name, metav1.GetOptions{})
			require.NoError(t, err)
			obName := liveObc.Spec.ObjectBucketName

			liveOb := wait4.RequireCondition(ctx, t, obClient, obName,
				func(ob *bktv1alpha1.ObjectBucket) bool {
					return ob.Spec.Connection.AdditionalState["bucketOwner"] == osu1.Name
				},
				wait4.TimeoutShort)

			// verify that bucketOwner is set on the live ob
			assert.Equal(t, osu1.Name, liveOb.Spec.Connection.AdditionalState["bucketOwner"])
		})

		t.Run(fmt.Sprintf("bucket owner changed to %q", osu1.Name), func(t *testing.T) {
			wait4.AssertEventually(ctx, t, wait4.TimeoutShort, fmt.Sprintf("bucket %q owner is %q", obc1.Name, osu1.Name), func(ctx context.Context) error {
				bucket, err := adminClient.GetBucketInfo(ctx, admin.Bucket{Bucket: obc1.Name})
				if err != nil {
					return err
				}

				if bucket.Owner != osu1.Name {
					return fmt.Errorf("bucket owner is %q, want %q", bucket.Owner, osu1.Name)
				}
				return nil
			})
		})

		t.Run(fmt.Sprintf("create obc %q with bucketOwner %q", obc2.Name, obc2.Spec.AdditionalConfig["bucketOwner"]), func(t *testing.T) {
			_, err := obcClient.Create(ctx, &obc2, metav1.CreateOptions{})
			require.NoError(t, err)
		})

		t.Run(fmt.Sprintf("obc %q has bucketOwner %q set", obc2.Name, obc2.Spec.AdditionalConfig["bucketOwner"]), func(t *testing.T) {
			liveObc := wait4.RequireCondition(ctx, t, obcClient, obc2.Name, wait4.OBCBound, wait4.TimeoutShort)

			// verify that bucketOwner is set on the live obc
			assert.Equal(t, obc2.Spec.AdditionalConfig["bucketOwner"], liveObc.Spec.AdditionalConfig["bucketOwner"])
		})

		t.Run(fmt.Sprintf("ob for obc %q has bucketOwner %q set", obc2.Name, obc2.Spec.AdditionalConfig["bucketOwner"]), func(t *testing.T) {
			liveObc, err := obcClient.Get(ctx, obc2.Name, metav1.GetOptions{})
			require.NoError(t, err)
			obName := liveObc.Spec.ObjectBucketName

			liveOb := wait4.RequireCondition(ctx, t, obClient, obName, wait4.OBBound, wait4.TimeoutShort)

			// verify that bucketOwner is set on the live ob
			assert.Equal(t, obc2.Spec.AdditionalConfig["bucketOwner"], liveOb.Spec.Connection.AdditionalState["bucketOwner"])
		})

		t.Run(fmt.Sprintf("bucket %q and %q share the same owner", obc1.Spec.BucketName, obc2.Spec.BucketName), func(t *testing.T) {
			bucket1, err := adminClient.GetBucketInfo(ctx, admin.Bucket{Bucket: obc1.Spec.BucketName})
			require.NoError(t, err)

			bucket2, err := adminClient.GetBucketInfo(ctx, admin.Bucket{Bucket: obc2.Spec.BucketName})
			require.NoError(t, err)

			assert.Equal(t, obc1.Spec.AdditionalConfig["bucketOwner"], bucket1.Owner)
			assert.Equal(t, obc1.Spec.AdditionalConfig["bucketOwner"], bucket2.Owner)
		})

		t.Run(fmt.Sprintf("delete obc %q", obc1.Name), func(t *testing.T) {
			// lookup ob name
			liveObc, err := obcClient.Get(ctx, obc1.Name, metav1.GetOptions{})
			require.NoError(t, err)
			obName := liveObc.Spec.ObjectBucketName

			// delete obc; the backing ob is garbage-collected by the provisioner
			wait4.AssertDelete(ctx, t, obcClient, obc1.Name, wait4.TimeoutShort)

			wait4.AssertAbsent(ctx, t, obClient, obName, wait4.TimeoutShort)
		})

		t.Run(fmt.Sprintf("delete obc %q", obc2.Name), func(t *testing.T) {
			// lookup ob name
			liveObc, err := obcClient.Get(ctx, obc2.Name, metav1.GetOptions{})
			require.NoError(t, err)
			obName := liveObc.Spec.ObjectBucketName

			// delete obc; the backing ob is garbage-collected by the provisioner
			wait4.AssertDelete(ctx, t, obcClient, obc2.Name, wait4.TimeoutShort)

			wait4.AssertAbsent(ctx, t, obClient, obName, wait4.TimeoutShort)
		})

		t.Run(fmt.Sprintf("user %q was not deleted by obc %q", osu1.Name, obc1.Name), func(t *testing.T) {
			user, err := adminClient.GetUser(ctx, admin.User{ID: osu1.Name})
			require.NoError(t, err)

			assert.Equal(t, osu1.Name, user.ID)
		})

		t.Run(fmt.Sprintf("user %q was not deleted by obc %q", osu2.Name, obc1.Name), func(t *testing.T) {
			user, err := adminClient.GetUser(ctx, admin.User{ID: osu2.Name})
			require.NoError(t, err)

			assert.Equal(t, osu2.Name, user.ID)
		})

		// test obc creation with bucketOwner set to a non-existent user, which should fail
		// "failure" means the obc remains in Pending state
		t.Run(fmt.Sprintf("create obc %q with non-existent bucketOwner %q", obcBogusOwner.Name, obcBogusOwner.Spec.AdditionalConfig["bucketOwner"]), func(t *testing.T) {
			_, err := obcClient.Create(ctx, &obcBogusOwner, metav1.CreateOptions{})
			require.NoError(t, err)
		})

		t.Run(fmt.Sprintf("operator logs failed lookup for user %q", obcBogusOwner.Spec.AdditionalConfig["bucketOwner"]), func(t *testing.T) {
			selector := labels.SelectorFromSet(labels.Set{
				"app": "rook-ceph-operator",
			})
			// match on the operator's "unable to get user ... creds" error
			// mentioning the bogus owner rather than a hardcoded full log line,
			// so the logger's quote encoding cannot break the match
			bogusOwner := obcBogusOwner.Spec.AdditionalConfig["bucketOwner"]
			wait4.RequirePodLog(ctx, t, k8sh, "object-ns-system", selector, wait4.TimeoutShort, func(line string) bool {
				return strings.Contains(line, "unable to get user") && strings.Contains(line, bogusOwner)
			})
		})

		t.Run(fmt.Sprintf("obc %q stays Pending", obcBogusOwner.Name), func(t *testing.T) {
			liveObc, err := obcClient.Get(ctx, obcBogusOwner.Name, metav1.GetOptions{})
			require.NoError(t, err)

			assert.True(t, bktv1alpha1.ObjectBucketClaimStatusPhasePending == liveObc.Status.Phase)
		})

		t.Run(fmt.Sprintf("user %q does not exist", obcBogusOwner.Spec.AdditionalConfig["bucketOwner"]), func(t *testing.T) {
			_, err := adminClient.GetUser(ctx, admin.User{ID: obcBogusOwner.Spec.AdditionalConfig["bucketOwner"]})
			require.ErrorIs(t, err, admin.ErrNoSuchUser)
		})

		t.Run(fmt.Sprintf("delete obc %q", obcBogusOwner.Name), func(t *testing.T) {
			// lookup ob name
			liveObc, err := obcClient.Get(ctx, obcBogusOwner.Name, metav1.GetOptions{})
			require.NoError(t, err)
			obName := liveObc.Spec.ObjectBucketName

			// delete obc; the backing ob is garbage-collected by the provisioner
			wait4.AssertDelete(ctx, t, obcClient, obcBogusOwner.Name, wait4.TimeoutShort)

			wait4.AssertAbsent(ctx, t, obClient, obName, wait4.TimeoutShort)
		})

		t.Run(fmt.Sprintf("delete CephObjectStoreUser %q", osu2.Name), func(t *testing.T) {
			wait4.AssertDelete(ctx, t, osuClient, osu2.Name, wait4.TimeoutShort)
		})

		t.Run(fmt.Sprintf("delete CephObjectStoreUser %q", osu1.Name), func(t *testing.T) {
			wait4.AssertDelete(ctx, t, osuClient, osu1.Name, wait4.TimeoutShort)
		})
	})
}
