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

package storageclass

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	smithy "github.com/aws/smithy-go"
	"github.com/ceph/go-ceph/rgw/admin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	cephv1 "github.com/rook/rook/pkg/apis/ceph.rook.io/v1"
	"github.com/rook/rook/tests/framework/utils"
	"github.com/rook/rook/tests/integration/object/util/client"
	"github.com/rook/rook/tests/integration/object/util/fixture"
	"github.com/rook/rook/tests/integration/object/util/sharedstore"
	"github.com/rook/rook/tests/integration/object/util/wait4"
)

const Namespace = "test-userstorageclass"

func TestObjectStoreUserDefaultStorageClass(t *testing.T, k8sh *utils.K8sHelper, store *sharedstore.Sharedstore) {
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
				// "FOO" is the non-default storage class on the shared store's
				// "default" placement, which must be named too: a storage class
				// only has meaning within a placement target.
				DefaultPlacement:    "default",
				DefaultStorageClass: "FOO",
			},
		}

		osuClient  = k8sh.RookClientset.CephV1().CephObjectStoreUsers(ns.Name)
		bucketName = defaultName + "-bucket"
	)

	t.Run("ObjectStoreUser defaultStorageClass", func(t *testing.T) {
		ctx := t.Context()

		fixture.RequireNamespace(t, k8sh, ns)

		t.Run(fmt.Sprintf("create CephObjectStoreUser %q", osu1.Name), func(t *testing.T) {
			// user creation may be slow right after rgw start up
			wait4.RequireCreate(ctx, t, osuClient, &osu1, wait4.ObjectStoreUser, wait4.TimeoutLong)
		})

		// A bucket is the end-to-end assertion: it inherits the user's default
		// placement rule at creation, so "default/FOO" on the bucket proves the
		// storage class actually took effect, not merely that the rgw recorded it.
		t.Run(fmt.Sprintf("bucket created by %q inherits the %q storage class", osu1.Name, "FOO"), func(t *testing.T) {
			ctx := t.Context()

			liveUser, err := adminClient.GetUser(ctx, admin.User{ID: osu1.Name})
			require.NoError(t, err)
			require.NotEmpty(t, liveUser.Keys)

			s3agent, err := client.NewS3Agent(objectStore, k8sh, store.TLSEnabled(), liveUser.Keys[0].AccessKey, liveUser.Keys[0].SecretKey)
			require.NoError(t, err)

			require.NoError(t, s3agent.CreateBucket(ctx, bucketName))
			t.Cleanup(func() {
				// a leaked bucket blocks the user's deletion, and with it the
				// shared store's teardown
				_, err := s3agent.Client.DeleteBucket(context.Background(), &s3.DeleteBucketInput{Bucket: aws.String(bucketName)})
				if err == nil {
					return
				}

				var apiErr smithy.APIError
				if errors.As(err, &apiErr) && apiErr.ErrorCode() == "NoSuchBucket" {
					return
				}
				t.Errorf("failed to delete bucket %q: %v", bucketName, err)
			})

			var placementRule string
			wait4.AssertEventually(ctx, t, wait4.TimeoutShort, fmt.Sprintf("bucket %q placement uses the %q storage class", bucketName, "FOO"), func(ctx context.Context) error {
				info, err := adminClient.GetBucketInfo(ctx, admin.Bucket{Bucket: bucketName})
				if err != nil {
					return err
				}

				if _, storageClass, _ := strings.Cut(info.PlacementRule, "/"); storageClass != "FOO" {
					return fmt.Errorf("bucket placement_rule storage class not yet %q: %q", "FOO", info.PlacementRule)
				}

				placementRule = info.PlacementRule
				return nil
			})

			assert.Equal(t, "default/FOO", placementRule)
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
