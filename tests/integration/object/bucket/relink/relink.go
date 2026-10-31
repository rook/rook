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

package relink

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
	kerrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"

	cephv1 "github.com/rook/rook/pkg/apis/ceph.rook.io/v1"
	rgw "github.com/rook/rook/pkg/operator/ceph/object"
	"github.com/rook/rook/tests/framework/installer"
	"github.com/rook/rook/tests/framework/utils"
	"github.com/rook/rook/tests/integration/object/util/fixture"
	"github.com/rook/rook/tests/integration/object/util/obc"
	"github.com/rook/rook/tests/integration/object/util/sharedstore"
	"github.com/rook/rook/tests/integration/object/util/wait4"
)

// The victim's and the thief's OBCs live in separate namespaces because a
// namespace is the boundary an OBC author is confined to.
const (
	Namespace      = "test-bucketrelink"
	ThiefNamespace = "test-bucketrelink-thief"
)

// TestObjectBucketClaimRelink verifies that an OBC cannot take over a bucket
// owned by another user. Bucket names are global to the object store and an
// OBC's author chooses spec.bucketName, so if the operator relinked any bucket
// an OBC named, an OBC in another namespace could take a tenant's bucket and
// its objects, and purge them by deleting the OBC. The test claims the victim's
// bucket from a second namespace, once plainly and once with bucketOwner naming
// a user of its own: each OBC must stay Pending without credentials, and the
// bucket must keep its owner and object, also after both OBCs are deleted.
// Finally it checks that bucketOwner can still move an OBC's own bucket to
// another user.
func TestObjectBucketClaimRelink(t *testing.T, k8sh *utils.K8sHelper, store *sharedstore.Sharedstore) {
	var (
		defaultName = Namespace
		objectStore = store.ObjectStore()
		adminClient = store.AdminClient()

		victimNs = &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: Namespace}}
		thiefNs  = &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: ThiefNamespace}}

		storageClass = obc.StorageClass(defaultName, objectStore)

		bucketName = defaultName

		victimObc = &bktv1alpha1.ObjectBucketClaim{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "victim",
				Namespace: victimNs.Name,
			},
			Spec: bktv1alpha1.ObjectBucketClaimSpec{
				BucketName:       bucketName,
				StorageClassName: storageClass.Name,
			},
		}

		victimUser = cephv1.CephObjectStoreUser{
			ObjectMeta: metav1.ObjectMeta{
				Name:      defaultName + "-victim-user",
				Namespace: victimNs.Name,
			},
			Spec: cephv1.ObjectStoreUserSpec{
				Store:            objectStore.Name,
				ClusterNamespace: objectStore.Namespace,
			},
		}

		thiefObc = &bktv1alpha1.ObjectBucketClaim{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "thief",
				Namespace: thiefNs.Name,
			},
			Spec: bktv1alpha1.ObjectBucketClaimSpec{
				BucketName:       bucketName,
				StorageClassName: storageClass.Name,
			},
		}

		thiefUser = cephv1.CephObjectStoreUser{
			ObjectMeta: metav1.ObjectMeta{
				Name:      defaultName + "-thief-user",
				Namespace: thiefNs.Name,
			},
			Spec: cephv1.ObjectStoreUserSpec{
				Store:            objectStore.Name,
				ClusterNamespace: objectStore.Namespace,
			},
		}

		thiefOwnerObc = &bktv1alpha1.ObjectBucketClaim{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "thief-owner",
				Namespace: thiefNs.Name,
			},
			Spec: bktv1alpha1.ObjectBucketClaimSpec{
				BucketName:       bucketName,
				StorageClassName: storageClass.Name,
				AdditionalConfig: map[string]string{
					"bucketOwner": thiefUser.Name,
				},
			},
		}

		body        = "victim's payload"
		contentType = "text/plain"
		key         = "obj1"

		operatorNs       = installer.SystemNamespace(objectStore.Namespace)
		operatorSelector = labels.SelectorFromSet(labels.Set{"app": "rook-ceph-operator"})

		victimOsuClient = k8sh.RookClientset.CephV1().CephObjectStoreUsers(victimNs.Name)
		thiefOsuClient  = k8sh.RookClientset.CephV1().CephObjectStoreUsers(thiefNs.Name)
		thiefObcClient  = k8sh.BucketClientset.ObjectbucketV1alpha1().ObjectBucketClaims(thiefNs.Name)
		victimObcClient = k8sh.BucketClientset.ObjectbucketV1alpha1().ObjectBucketClaims(victimNs.Name)
		obClient        = k8sh.BucketClientset.ObjectbucketV1alpha1().ObjectBuckets()
	)

	t.Run("OBC bucket relink", func(t *testing.T) {
		ctx := t.Context()

		fixture.RequireNamespace(t, k8sh, victimNs)
		fixture.RequireNamespace(t, k8sh, thiefNs)
		fixture.RequireStorageClass(t, k8sh, storageClass)

		liveVictimObc := obc.RequireBound(ctx, t, k8sh, victimObc)

		victimOb, err := obClient.Get(ctx, liveVictimObc.Spec.ObjectBucketName, metav1.GetOptions{})
		require.NoError(t, err)
		victimOwner := victimOb.Spec.Connection.AdditionalState["cephUser"]
		require.NotEmpty(t, victimOwner)

		t.Run(fmt.Sprintf("bucket %q is owned by the user of obc %q", bucketName, victimObc.Name), func(t *testing.T) {
			bucket, err := adminClient.GetBucketInfo(ctx, admin.Bucket{Bucket: bucketName})
			require.NoError(t, err)
			assert.Equal(t, victimOwner, bucket.Owner)
		})

		victimS3 := obc.NewS3Agent(ctx, t, k8sh, store, victimNs.Name, victimObc.Name)

		t.Run(fmt.Sprintf("put object %q as obc %q", key, victimObc.Name), func(t *testing.T) {
			_, err := victimS3.PutObjectInBucket(ctx, bucketName, body, key, contentType)
			require.NoError(t, err)
		})

		t.Run(fmt.Sprintf("create obc %q for bucket %q", thiefObc.Name, bucketName), func(t *testing.T) {
			_, err := thiefObcClient.Create(ctx, thiefObc, metav1.CreateOptions{})
			require.NoError(t, err)
		})

		t.Run(fmt.Sprintf("operator refuses obc %q the bucket", thiefObc.Name), func(t *testing.T) {
			// the refusal names the OBC's generated user, obc-<ns>-<name>-<uid>. A
			// relink logs "already exists and is owned by user" too, so match on
			// wording only the refusal uses, and on substrings so the logger's
			// quote encoding cannot break the match.
			thiefObcUser := "obc-" + thiefNs.Name + "-" + thiefObc.Name + "-"
			wait4.AssertPodLog(ctx, t, k8sh, operatorNs, operatorSelector, wait4.TimeoutShort, func(line string) bool {
				return strings.Contains(line, "instead of this OBC") && strings.Contains(line, thiefObcUser)
			})
		})

		checkRefused(t, k8sh, thiefNs.Name, thiefObc.Name)
		checkBucketKept(t, adminClient, victimS3, bucketName, victimOwner, key, body)

		t.Run(fmt.Sprintf("create CephObjectStoreUser %q", thiefUser.Name), func(t *testing.T) {
			wait4.RequireCreate(ctx, t, thiefOsuClient, &thiefUser, wait4.ObjectStoreUser, wait4.TimeoutLong)
		})

		t.Run(fmt.Sprintf("create obc %q for bucket %q with bucketOwner %q", thiefOwnerObc.Name, bucketName, thiefUser.Name), func(t *testing.T) {
			_, err := thiefObcClient.Create(ctx, thiefOwnerObc, metav1.CreateOptions{})
			require.NoError(t, err)
		})

		t.Run(fmt.Sprintf("operator refuses obc %q the bucket", thiefOwnerObc.Name), func(t *testing.T) {
			wait4.AssertPodLog(ctx, t, k8sh, operatorNs, operatorSelector, wait4.TimeoutShort, func(line string) bool {
				return strings.Contains(line, "may only take over a bucket owned by") && strings.Contains(line, thiefUser.Name)
			})
		})

		checkRefused(t, k8sh, thiefNs.Name, thiefOwnerObc.Name)
		checkBucketKept(t, adminClient, victimS3, bucketName, victimOwner, key, body)

		// the StorageClass reclaims with Delete, so deleting an OBC that had been
		// handed the bucket would purge it
		t.Run(fmt.Sprintf("delete obc %q", thiefObc.Name), func(t *testing.T) {
			wait4.AssertDelete(ctx, t, thiefObcClient, thiefObc.Name, wait4.TimeoutShort)
		})

		t.Run(fmt.Sprintf("delete obc %q", thiefOwnerObc.Name), func(t *testing.T) {
			wait4.AssertDelete(ctx, t, thiefObcClient, thiefOwnerObc.Name, wait4.TimeoutShort)
		})

		checkBucketKept(t, adminClient, victimS3, bucketName, victimOwner, key, body)

		// bucketOwner may still move a bucket away from the user its own OBC
		// provisioned it with
		t.Run(fmt.Sprintf("create CephObjectStoreUser %q", victimUser.Name), func(t *testing.T) {
			wait4.RequireCreate(ctx, t, victimOsuClient, &victimUser, wait4.ObjectStoreUser, wait4.TimeoutLong)
		})

		t.Run(fmt.Sprintf("update obc %q to bucketOwner %q", victimObc.Name, victimUser.Name), func(t *testing.T) {
			obc.Update(ctx, t, k8sh, victimNs.Name, victimObc.Name, func(o *bktv1alpha1.ObjectBucketClaim) {
				o.Spec.AdditionalConfig = map[string]string{"bucketOwner": victimUser.Name}
			})
		})

		t.Run(fmt.Sprintf("bucket owner changed to %q", victimUser.Name), func(t *testing.T) {
			wait4.AssertEventually(ctx, t, wait4.TimeoutShort, fmt.Sprintf("bucket %q owner is %q", bucketName, victimUser.Name), func(ctx context.Context) error {
				bucket, err := adminClient.GetBucketInfo(ctx, admin.Bucket{Bucket: bucketName})
				if err != nil {
					return err
				}

				if bucket.Owner != victimUser.Name {
					return fmt.Errorf("bucket owner is %q, want %q", bucket.Owner, victimUser.Name)
				}
				return nil
			})
		})

		t.Run(fmt.Sprintf("obc %q stays Bound", victimObc.Name), func(t *testing.T) {
			wait4.AssertCondition(ctx, t, victimObcClient, victimObc.Name, wait4.OBCBound, wait4.TimeoutShort)
		})

		t.Run(fmt.Sprintf("delete obc %q", victimObc.Name), func(t *testing.T) {
			obc.DeleteAndWait(ctx, t, k8sh, victimNs.Name, victimObc.Name)
		})

		t.Run(fmt.Sprintf("delete CephObjectStoreUser %q", victimUser.Name), func(t *testing.T) {
			wait4.AssertDelete(ctx, t, victimOsuClient, victimUser.Name, wait4.TimeoutShort)
		})

		t.Run(fmt.Sprintf("delete CephObjectStoreUser %q", thiefUser.Name), func(t *testing.T) {
			wait4.AssertDelete(ctx, t, thiefOsuClient, thiefUser.Name, wait4.TimeoutShort)
		})
	})
}

// checkRefused verifies that the OBC was not provisioned: it has no
// ObjectBucket, and no Secret carries credentials for it.
func checkRefused(t *testing.T, k8sh *utils.K8sHelper, namespace, name string) {
	t.Helper()

	t.Run(fmt.Sprintf("obc %q stays Pending without credentials", name), func(t *testing.T) {
		ctx := t.Context()

		liveObc, err := k8sh.BucketClientset.ObjectbucketV1alpha1().ObjectBucketClaims(namespace).Get(ctx, name, metav1.GetOptions{})
		require.NoError(t, err)
		assert.EqualValues(t, bktv1alpha1.ObjectBucketClaimStatusPhasePending, liveObc.Status.Phase)
		assert.Empty(t, liveObc.Spec.ObjectBucketName)

		_, err = k8sh.Clientset.CoreV1().Secrets(namespace).Get(ctx, name, metav1.GetOptions{})
		assert.True(t, kerrors.IsNotFound(err), "secret %q exists or failed to get: %v", name, err)
	})
}

// checkBucketKept verifies that the bucket still belongs to owner, and that
// owner's object in it is intact and still readable with owner's credentials.
func checkBucketKept(t *testing.T, adminClient *admin.API, s3 *rgw.S3Agent, bucketName, owner, key, body string) {
	t.Helper()

	t.Run(fmt.Sprintf("bucket %q kept its owner and object %q", bucketName, key), func(t *testing.T) {
		ctx := t.Context()

		bucket, err := adminClient.GetBucketInfo(ctx, admin.Bucket{Bucket: bucketName})
		require.NoError(t, err)
		assert.Equal(t, owner, bucket.Owner)

		read, err := s3.GetObjectInBucket(ctx, bucketName, key)
		require.NoError(t, err)
		assert.Equal(t, body, read)
	})
}
