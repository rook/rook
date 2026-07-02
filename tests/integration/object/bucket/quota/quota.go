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

package quota

import (
	"context"
	"fmt"
	"strings"
	"testing"

	bktv1alpha1 "github.com/kube-object-storage/lib-bucket-provisioner/pkg/apis/objectbucket.io/v1alpha1"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	rgw "github.com/rook/rook/pkg/operator/ceph/object"
	"github.com/rook/rook/tests/framework/utils"
	"github.com/rook/rook/tests/integration/object/util/fixture"
	"github.com/rook/rook/tests/integration/object/util/obc"
	"github.com/rook/rook/tests/integration/object/util/sharedstore"
	"github.com/rook/rook/tests/integration/object/util/wait4"
)

const (
	body        = "test bucket quota payload"
	contentType = "text/plain"

	key1 = "obj1"
	key2 = "obj2"
	key3 = "obj3"
	key4 = "obj4"
)

// obAdditionalConfig returns a predicate matching an ObjectBucket whose endpoint
// additionalConfig has key set to value, the sign that the provisioner has applied
// a quota change to the backing bucket.
func obAdditionalConfig(key, value string) func(*bktv1alpha1.ObjectBucket) bool {
	return func(ob *bktv1alpha1.ObjectBucket) bool {
		if ob.Spec.Connection == nil || ob.Spec.Connection.Endpoint == nil {
			return false
		}
		return ob.Spec.Connection.Endpoint.AdditionalConfigData[key] == value
	}
}

// requireQuotaEnforced waits until putting key into bucket is rejected. rgw
// enforces user quota against per-instance cached stats, so enforcement can lag
// recent writes; an unexpectedly-successful put is deleted so the next attempt
// creates a new object rather than overwriting.
func requireQuotaEnforced(ctx context.Context, t *testing.T, s3 *rgw.S3Agent, bucket, key string) {
	t.Helper()

	wait4.RequireEventually(ctx, t, wait4.TimeoutMedium, fmt.Sprintf("quota rejects object %q", key), func(ctx context.Context) error {
		_, err := s3.PutObjectInBucket(ctx, bucket, body, key, contentType)
		if err == nil {
			s3.DeleteObjectInBucket(ctx, bucket, key) //nolint:errcheck
			return fmt.Errorf("quota not yet enforced: put %q succeeded", key)
		}
		return nil
	})
}

const Namespace = "test-bucketquota"

func TestObjectBucketClaimQuota(t *testing.T, k8sh *utils.K8sHelper, store *sharedstore.Sharedstore) {
	var (
		defaultName = Namespace
		objectStore = store.ObjectStore()

		ns = &corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{
				Name: defaultName,
			},
		}

		storageClass = obc.StorageClass(defaultName, objectStore)

		obc1 = &bktv1alpha1.ObjectBucketClaim{
			ObjectMeta: metav1.ObjectMeta{
				Name:      defaultName + "-obc1",
				Namespace: ns.Name,
			},
			Spec: bktv1alpha1.ObjectBucketClaimSpec{
				BucketName:       defaultName + "-obc1",
				StorageClassName: storageClass.Name,
				AdditionalConfig: map[string]string{"maxObjects": "2"},
			},
		}

		obc2 = &bktv1alpha1.ObjectBucketClaim{
			ObjectMeta: metav1.ObjectMeta{
				Name:      defaultName + "-obc2",
				Namespace: ns.Name,
			},
			Spec: bktv1alpha1.ObjectBucketClaimSpec{
				BucketName:       defaultName + "-obc2",
				StorageClassName: storageClass.Name,
				AdditionalConfig: map[string]string{"bucketMaxObjects": "1"},
			},
		}

		obcClient = k8sh.BucketClientset.ObjectbucketV1alpha1().ObjectBucketClaims(ns.Name)
		obClient  = k8sh.BucketClientset.ObjectbucketV1alpha1().ObjectBuckets()
	)

	t.Run("ObjectBucketClaim quota", func(t *testing.T) {
		ctx := t.Context()

		fixture.RequireNamespace(t, k8sh, ns)
		fixture.RequireStorageClass(t, k8sh, storageClass)

		liveObc1 := obc.RequireBound(ctx, t, k8sh, obc1)
		s3a := obc.NewS3Agent(ctx, t, k8sh, store, ns.Name, obc1.Name)

		t.Run(fmt.Sprintf("maxObjects quota is enforced on obc %q", obc1.Name), func(t *testing.T) {
			_, err := s3a.PutObjectInBucket(ctx, obc1.Spec.BucketName, body, key1, contentType)
			require.NoError(t, err)
			_, err = s3a.PutObjectInBucket(ctx, obc1.Spec.BucketName, body, key2, contentType)
			require.NoError(t, err)

			requireQuotaEnforced(ctx, t, s3a, obc1.Spec.BucketName, key3)
		})

		t.Run(fmt.Sprintf("raise maxObjects on obc %q", obc1.Name), func(t *testing.T) {
			obc.Update(ctx, t, k8sh, ns.Name, obc1.Name, func(live *bktv1alpha1.ObjectBucketClaim) {
				live.Spec.AdditionalConfig["maxObjects"] = "3"
			})

			wait4.RequireCondition(ctx, t, obClient, liveObc1.Spec.ObjectBucketName,
				obAdditionalConfig("maxObjects", "3"), wait4.TimeoutShort)
		})

		t.Run(fmt.Sprintf("raised maxObjects quota admits object %q", key3), func(t *testing.T) {
			_, err := s3a.PutObjectInBucket(ctx, obc1.Spec.BucketName, body, key3, contentType)
			require.NoError(t, err)

			requireQuotaEnforced(ctx, t, s3a, obc1.Spec.BucketName, key4)
		})

		t.Run(fmt.Sprintf("delete objects in bucket %q", obc1.Spec.BucketName), func(t *testing.T) {
			for _, key := range []string{key1, key2, key3} {
				_, err := s3a.DeleteObjectInBucket(ctx, obc1.Spec.BucketName, key)
				assert.NoError(t, err)
			}
		})

		t.Run(fmt.Sprintf("delete obc %q", obc1.Name), func(t *testing.T) {
			obc.DeleteAndWait(ctx, t, k8sh, ns.Name, obc1.Name)
		})

		liveObc2 := obc.RequireBound(ctx, t, k8sh, obc2)
		s3b := obc.NewS3Agent(ctx, t, k8sh, store, ns.Name, obc2.Name)

		t.Run(fmt.Sprintf("bucketMaxObjects quota is enforced on obc %q", obc2.Name), func(t *testing.T) {
			_, err := s3b.PutObjectInBucket(ctx, obc2.Spec.BucketName, body, key1, contentType)
			require.NoError(t, err)

			// unlike the rgw-cached user quota, bucket quota applies immediately
			_, err = s3b.PutObjectInBucket(ctx, obc2.Spec.BucketName, body, key2, contentType)
			assert.Error(t, err)

			for _, key := range []string{key1, key2} {
				_, err := s3b.DeleteObjectInBucket(ctx, obc2.Spec.BucketName, key)
				assert.NoError(t, err)
			}
		})

		t.Run(fmt.Sprintf("switch obc %q to a bucketMaxSize quota", obc2.Name), func(t *testing.T) {
			obc.Update(ctx, t, k8sh, ns.Name, obc2.Name, func(live *bktv1alpha1.ObjectBucketClaim) {
				live.Spec.AdditionalConfig = map[string]string{"bucketMaxSize": "4Ki"}
			})

			wait4.RequireCondition(ctx, t, obClient, liveObc2.Spec.ObjectBucketName,
				obAdditionalConfig("bucketMaxSize", "4Ki"), wait4.TimeoutShort)
		})

		t.Run(fmt.Sprintf("bucketMaxSize quota is enforced on obc %q", obc2.Name), func(t *testing.T) {
			_, err := s3b.PutObjectInBucket(ctx, obc2.Spec.BucketName, strings.Repeat("1", 3072), key1, contentType)
			require.NoError(t, err)

			_, err = s3b.PutObjectInBucket(ctx, obc2.Spec.BucketName, strings.Repeat("2", 2048), key2, contentType)
			assert.Error(t, err)

			for _, key := range []string{key1, key2} {
				_, err := s3b.DeleteObjectInBucket(ctx, obc2.Spec.BucketName, key)
				assert.NoError(t, err)
			}
		})

		t.Run(fmt.Sprintf("delete obc %q", obc2.Name), func(t *testing.T) {
			obc.DeleteAndWait(ctx, t, k8sh, ns.Name, obc2.Name)
		})

		t.Run(fmt.Sprintf("no obc(s) in ns %q", ns.Name), func(t *testing.T) {
			list, err := obcClient.List(ctx, metav1.ListOptions{})
			require.NoError(t, err)
			assert.Len(t, list.Items, 0)
		})
	})
}
