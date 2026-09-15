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

// Package placement covers the OBC bucketPlacement and bucketStorageClass
// additionalConfig keys: the placement rule of a bucket the provisioner
// creates, the Warning Events it records for a request it or RGW rejects, and
// the mismatch check against a bucket that already exists.
package placement

import (
	"context"
	"fmt"
	"slices"
	"testing"

	"github.com/ceph/go-ceph/rgw/admin"
	bktv1alpha1 "github.com/kube-object-storage/lib-bucket-provisioner/pkg/apis/objectbucket.io/v1alpha1"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	eventsv1 "k8s.io/api/events/v1"
	storagev1 "k8s.io/api/storage/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/fields"

	cephv1 "github.com/rook/rook/pkg/apis/ceph.rook.io/v1"
	"github.com/rook/rook/tests/framework/utils"
	"github.com/rook/rook/tests/integration/object/util/fixture"
	"github.com/rook/rook/tests/integration/object/util/obc"
	"github.com/rook/rook/tests/integration/object/util/sharedstore"
	"github.com/rook/rook/tests/integration/object/util/wait4"
)

const Namespace = "test-bucketplacement"

// Warning Event reasons the provisioner records on an ObjectBucketClaim.
const (
	reasonInvalidPlacement  = "InvalidBucketPlacement"
	reasonPlacementRejected = "BucketPlacementRejected"
	reasonPlacementMismatch = "BucketPlacementMismatch"
)

// claim returns an ObjectBucketClaim that provisions a bucket of the same name
// through storageClassName.
func claim(name, storageClassName string, additionalConfig map[string]string) *bktv1alpha1.ObjectBucketClaim {
	return &bktv1alpha1.ObjectBucketClaim{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: Namespace,
		},
		Spec: bktv1alpha1.ObjectBucketClaimSpec{
			BucketName:       name,
			StorageClassName: storageClassName,
			AdditionalConfig: additionalConfig,
		},
	}
}

// grantClaim returns an ObjectBucketClaim that is granted access to the bucket
// storageClassName names, so it carries no bucket name of its own.
func grantClaim(name, storageClassName string, additionalConfig map[string]string) *bktv1alpha1.ObjectBucketClaim {
	c := claim(name, storageClassName, additionalConfig)
	c.Spec.BucketName = ""
	return c
}

// grantStorageClass returns a provisioner StorageClass whose bucketName
// parameter turns every claim through it into a Grant on that existing bucket
// instead of a provisioning request.
func grantStorageClass(name string, objectStore *cephv1.CephObjectStore, bucketName string) *storagev1.StorageClass {
	sc := obc.StorageClass(name, objectStore)
	sc.Parameters[bktv1alpha1.StorageClassBucket] = bucketName
	return sc
}

// checkPlacementRule asserts the placement rule rgw reports for the bucket.
func checkPlacementRule(t *testing.T, adminClient *admin.API, bucketName, want string) {
	t.Helper()

	t.Run(fmt.Sprintf("bucket %q has placement rule %q", bucketName, want), func(t *testing.T) {
		ctx := t.Context()

		bucket, err := adminClient.GetBucketInfo(ctx, admin.Bucket{Bucket: bucketName})
		require.NoError(t, err)

		assert.Equal(t, want, bucket.PlacementRule)
	})
}

// checkNoBucket asserts that rgw has no bucket of the given name.
func checkNoBucket(t *testing.T, adminClient *admin.API, bucketName string) {
	t.Helper()

	t.Run(fmt.Sprintf("no bucket %q exists", bucketName), func(t *testing.T) {
		ctx := t.Context()

		_, err := adminClient.GetBucketInfo(ctx, admin.Bucket{Bucket: bucketName})
		require.ErrorIs(t, err, admin.ErrNoSuchBucket)
	})
}

// checkPhase asserts the claim's live phase.
func checkPhase(t *testing.T, k8sh *utils.K8sHelper, claim *bktv1alpha1.ObjectBucketClaim, phase bktv1alpha1.ObjectBucketClaimStatusPhase) {
	t.Helper()

	t.Run(fmt.Sprintf("obc %q stays %s", claim.Name, phase), func(t *testing.T) {
		ctx := t.Context()

		live, err := k8sh.BucketClientset.ObjectbucketV1alpha1().ObjectBucketClaims(claim.Namespace).Get(ctx, claim.Name, metav1.GetOptions{})
		require.NoError(t, err)

		assert.Equal(t, phase, live.Status.Phase)
	})
}

// checkWarningEvent waits for a Warning Event with the given reason regarding
// the claim and asserts that its note names each of mentions.
func checkWarningEvent(t *testing.T, k8sh *utils.K8sHelper, claim *bktv1alpha1.ObjectBucketClaim, reason string, mentions ...string) {
	t.Helper()

	t.Run(fmt.Sprintf("obc %q has a warning event with reason %q", claim.Name, reason), func(t *testing.T) {
		ctx := t.Context()

		obcClient := k8sh.BucketClientset.ObjectbucketV1alpha1().ObjectBucketClaims(claim.Namespace)
		eventClient := k8sh.Clientset.EventsV1().Events(claim.Namespace)

		live, err := obcClient.Get(ctx, claim.Name, metav1.GetOptions{})
		require.NoError(t, err)

		// the uid ties the event to this claim rather than to an earlier
		// claim of the same name
		selector := fields.SelectorFromSet(fields.Set{
			"regarding.uid": string(live.UID),
			"reason":        reason,
		}).String()

		var event eventsv1.Event
		wait4.RequireEventually(ctx, t, wait4.TimeoutShort, fmt.Sprintf("%s event regarding obc %q", reason, claim.Name), func(ctx context.Context) error {
			list, err := eventClient.List(ctx, metav1.ListOptions{FieldSelector: selector})
			if err != nil {
				return err
			}
			i := slices.IndexFunc(list.Items, func(e eventsv1.Event) bool { return e.Regarding.Kind == "ObjectBucketClaim" })
			if i < 0 {
				return fmt.Errorf("no %s event regarding ObjectBucketClaim uid %q (%d events regard that uid)", reason, live.UID, len(list.Items))
			}
			event = list.Items[i]
			return nil
		})

		assert.Equal(t, corev1.EventTypeWarning, event.Type)
		for _, want := range mentions {
			assert.Contains(t, event.Note, want)
		}
	})
}

func TestObjectBucketClaimPlacement(t *testing.T, k8sh *utils.K8sHelper, store *sharedstore.Sharedstore) {
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

		obcLocA           = claim(defaultName+"-loc-a", storageClass.Name, map[string]string{"bucketPlacement": sharedstore.PlacementLocA})
		obcFoo            = claim(defaultName+"-foo", storageClass.Name, map[string]string{"bucketStorageClass": sharedstore.StorageClassFoo})
		obcBoth           = claim(defaultName+"-both", storageClass.Name, map[string]string{"bucketPlacement": sharedstore.PlacementDefault, "bucketStorageClass": sharedstore.StorageClassFoo})
		obcUnknown        = claim(defaultName+"-unknown", storageClass.Name, map[string]string{"bucketPlacement": "nowhere"})
		obcMalformed      = claim(defaultName+"-malformed", storageClass.Name, map[string]string{"bucketPlacement": "foo:bar"})
		obcMalformedClass = claim(defaultName+"-malformed-class", storageClass.Name, map[string]string{"bucketStorageClass": "FOO "})

		// brownfield: a Grant on obcLocA's bucket, which is on loc-a
		grantClass = grantStorageClass(defaultName+"-grant", objectStore, obcLocA.Spec.BucketName)
		obcGrant   = grantClaim(defaultName+"-grant", grantClass.Name, map[string]string{"bucketPlacement": sharedstore.PlacementDefault})

		fooRule = sharedstore.PlacementDefault + "/" + sharedstore.StorageClassFoo

		obcClient = k8sh.BucketClientset.ObjectbucketV1alpha1().ObjectBucketClaims(ns.Name)
		obClient  = k8sh.BucketClientset.ObjectbucketV1alpha1().ObjectBuckets()
	)

	t.Run("OBC bucketPlacement", func(t *testing.T) {
		ctx := t.Context()

		fixture.RequireNamespace(t, k8sh, ns)
		fixture.RequireStorageClass(t, k8sh, storageClass)
		fixture.RequireStorageClass(t, k8sh, grantClass)

		obc.RequireBound(ctx, t, k8sh, obcLocA)
		checkPlacementRule(t, adminClient, obcLocA.Spec.BucketName, sharedstore.PlacementLocA)

		// a class alone inherits the owner's default placement, which for a
		// generated user is the zonegroup default
		obc.RequireBound(ctx, t, k8sh, obcFoo)
		checkPlacementRule(t, adminClient, obcFoo.Spec.BucketName, fooRule)

		liveBoth := obc.RequireBound(ctx, t, k8sh, obcBoth)
		checkPlacementRule(t, adminClient, obcBoth.Spec.BucketName, fooRule)

		// "failure" means the obc remains in Pending state
		t.Run(fmt.Sprintf("create obc %q with unknown bucketPlacement %q", obcUnknown.Name, obcUnknown.Spec.AdditionalConfig["bucketPlacement"]), func(t *testing.T) {
			_, err := obcClient.Create(ctx, obcUnknown, metav1.CreateOptions{})
			require.NoError(t, err)
		})
		checkWarningEvent(t, k8sh, obcUnknown, reasonPlacementRejected, obcUnknown.Spec.AdditionalConfig["bucketPlacement"])
		checkPhase(t, k8sh, obcUnknown, bktv1alpha1.ObjectBucketClaimStatusPhasePending)
		checkNoBucket(t, adminClient, obcUnknown.Spec.BucketName)
		// deleted now rather than at teardown: a claim that fails every
		// reconcile accumulates the provisioner's per-claim backoff, and its
		// deletion waits behind that timer
		t.Run(fmt.Sprintf("delete obc %q", obcUnknown.Name), func(t *testing.T) {
			obc.DeleteAndWait(ctx, t, k8sh, ns.Name, obcUnknown.Name)
		})

		t.Run(fmt.Sprintf("create obc %q with malformed bucketPlacement %q", obcMalformed.Name, obcMalformed.Spec.AdditionalConfig["bucketPlacement"]), func(t *testing.T) {
			_, err := obcClient.Create(ctx, obcMalformed, metav1.CreateOptions{})
			require.NoError(t, err)
		})
		checkWarningEvent(t, k8sh, obcMalformed, reasonInvalidPlacement, obcMalformed.Spec.AdditionalConfig["bucketPlacement"])
		checkPhase(t, k8sh, obcMalformed, bktv1alpha1.ObjectBucketClaimStatusPhasePending)
		checkNoBucket(t, adminClient, obcMalformed.Spec.BucketName)
		// deleted now, as obcUnknown was
		t.Run(fmt.Sprintf("delete obc %q", obcMalformed.Name), func(t *testing.T) {
			obc.DeleteAndWait(ctx, t, k8sh, ns.Name, obcMalformed.Name)
		})

		t.Run(fmt.Sprintf("create obc %q with malformed bucketStorageClass %q", obcMalformedClass.Name, obcMalformedClass.Spec.AdditionalConfig["bucketStorageClass"]), func(t *testing.T) {
			_, err := obcClient.Create(ctx, obcMalformedClass, metav1.CreateOptions{})
			require.NoError(t, err)
		})
		checkWarningEvent(t, k8sh, obcMalformedClass, reasonInvalidPlacement, obcMalformedClass.Spec.AdditionalConfig["bucketStorageClass"])
		checkPhase(t, k8sh, obcMalformedClass, bktv1alpha1.ObjectBucketClaimStatusPhasePending)
		checkNoBucket(t, adminClient, obcMalformedClass.Spec.BucketName)
		t.Run(fmt.Sprintf("delete obc %q", obcMalformedClass.Name), func(t *testing.T) {
			obc.DeleteAndWait(ctx, t, k8sh, ns.Name, obcMalformedClass.Name)
		})

		// a Bound claim that changes its request cannot be re-provisioned:
		// the bucket's placement is immutable
		t.Run(fmt.Sprintf("update obc %q to bucketPlacement %q", obcBoth.Name, sharedstore.PlacementLocA), func(t *testing.T) {
			obc.Update(ctx, t, k8sh, ns.Name, obcBoth.Name, func(live *bktv1alpha1.ObjectBucketClaim) {
				live.Spec.AdditionalConfig["bucketPlacement"] = sharedstore.PlacementLocA
				// the quota rides along to show whether a re-provision gets
				// past the placement check
				live.Spec.AdditionalConfig["bucketMaxObjects"] = "1234"
			})
		})
		checkWarningEvent(t, k8sh, obcBoth, reasonPlacementMismatch, sharedstore.PlacementLocA, sharedstore.PlacementDefault)
		checkPhase(t, k8sh, obcBoth, bktv1alpha1.ObjectBucketClaimStatusPhaseBound)

		t.Run(fmt.Sprintf("bucket %q is untouched while the placement mismatches", obcBoth.Spec.BucketName), func(t *testing.T) {
			bucket, err := adminClient.GetBucketInfo(ctx, admin.Bucket{Bucket: obcBoth.Spec.BucketName})
			require.NoError(t, err)

			assert.Equal(t, fooRule, bucket.PlacementRule)
			require.NotNil(t, bucket.BucketQuota.Enabled)
			assert.False(t, *bucket.BucketQuota.Enabled)
		})

		t.Run(fmt.Sprintf("revert obc %q to bucketPlacement %q", obcBoth.Name, sharedstore.PlacementDefault), func(t *testing.T) {
			obc.Update(ctx, t, k8sh, ns.Name, obcBoth.Name, func(live *bktv1alpha1.ObjectBucketClaim) {
				live.Spec.AdditionalConfig["bucketPlacement"] = sharedstore.PlacementDefault
			})
		})

		t.Run(fmt.Sprintf("obc %q re-provisions once the placement matches", obcBoth.Name), func(t *testing.T) {
			// the ob is only rewritten after a successful provision, and the
			// provisioner's per-claim retry backoff from the failed attempts
			// delays the next one
			wait4.RequireCondition(ctx, t, obClient, liveBoth.Spec.ObjectBucketName, wait4.OBAdditionalConfig("bucketMaxObjects", "1234"), wait4.TimeoutLong)

			bucket, err := adminClient.GetBucketInfo(ctx, admin.Bucket{Bucket: obcBoth.Spec.BucketName})
			require.NoError(t, err)

			assert.Equal(t, fooRule, bucket.PlacementRule)
			require.NotNil(t, bucket.BucketQuota.Enabled)
			assert.True(t, *bucket.BucketQuota.Enabled)
			require.NotNil(t, bucket.BucketQuota.MaxObjects)
			assert.Equal(t, int64(1234), *bucket.BucketQuota.MaxObjects)

			live, err := obcClient.Get(ctx, obcBoth.Name, metav1.GetOptions{})
			require.NoError(t, err)
			// the lib's phase constants are untyped, so compare as strings
			assert.Equal(t, bktv1alpha1.ObjectBucketClaimStatusPhaseBound, string(live.Status.Phase))
		})

		// a brownfield Grant is checked against the existing bucket the same way
		t.Run(fmt.Sprintf("create obc %q granting bucket %q with bucketPlacement %q", obcGrant.Name, obcLocA.Spec.BucketName, obcGrant.Spec.AdditionalConfig["bucketPlacement"]), func(t *testing.T) {
			// Grant manages the bucket policy with the claim user's own S3
			// credentials, which a user that does not own the bucket cannot
			// read; claim the bucket as its owner
			bucket, err := adminClient.GetBucketInfo(ctx, admin.Bucket{Bucket: obcLocA.Spec.BucketName})
			require.NoError(t, err)
			obcGrant.Spec.AdditionalConfig["bucketOwner"] = bucket.Owner

			_, err = obcClient.Create(ctx, obcGrant, metav1.CreateOptions{})
			require.NoError(t, err)
		})
		checkWarningEvent(t, k8sh, obcGrant, reasonPlacementMismatch, sharedstore.PlacementDefault, sharedstore.PlacementLocA)
		checkPhase(t, k8sh, obcGrant, bktv1alpha1.ObjectBucketClaimStatusPhasePending)

		t.Run(fmt.Sprintf("update obc %q to bucketPlacement %q", obcGrant.Name, sharedstore.PlacementLocA), func(t *testing.T) {
			obc.Update(ctx, t, k8sh, ns.Name, obcGrant.Name, func(live *bktv1alpha1.ObjectBucketClaim) {
				live.Spec.AdditionalConfig["bucketPlacement"] = sharedstore.PlacementLocA
			})
		})

		t.Run(fmt.Sprintf("obc %q binds once the placement matches", obcGrant.Name), func(t *testing.T) {
			// the provisioner's per-claim retry backoff from the failed
			// attempts delays the next one
			live := wait4.RequireCondition(ctx, t, obcClient, obcGrant.Name, wait4.OBCBound, wait4.TimeoutLong)
			wait4.RequireCondition(ctx, t, obClient, live.Spec.ObjectBucketName, wait4.OBBound, wait4.TimeoutShort)

			bucket, err := adminClient.GetBucketInfo(ctx, admin.Bucket{Bucket: obcLocA.Spec.BucketName})
			require.NoError(t, err)

			assert.Equal(t, sharedstore.PlacementLocA, bucket.PlacementRule)
			assert.Equal(t, obcGrant.Spec.AdditionalConfig["bucketOwner"], bucket.Owner)
		})

		// the grant claim goes first: deleting obcLocA removes the bucket they
		// share
		for _, name := range []string{obcGrant.Name, obcLocA.Name, obcFoo.Name, obcBoth.Name} {
			t.Run(fmt.Sprintf("delete obc %q", name), func(t *testing.T) {
				obc.DeleteAndWait(ctx, t, k8sh, ns.Name, name)
			})
		}

		t.Run(fmt.Sprintf("no obc(s) in ns %q", ns.Name), func(t *testing.T) {
			list, err := obcClient.List(ctx, metav1.ListOptions{})
			require.NoError(t, err)
			assert.Len(t, list.Items, 0)
		})
	})
}
