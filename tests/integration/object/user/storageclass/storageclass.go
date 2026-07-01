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
	"fmt"
	"strings"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/ceph/go-ceph/rgw/admin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"

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
				// "default" placement. RGW stores a user's default storage class
				// as part of its default placement rule, so the placement must
				// be set too.
				DefaultPlacement:    ptr.To("default"),
				DefaultStorageClass: ptr.To("FOO"),
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

		// The rgw admin user API accepts default_storage_class on write but does
		// not report it back via GetUser, so the override is verified on a bucket
		// the user owns: a bucket created without an explicit placement inherits
		// the user's default placement rule, here "default/FOO".
		t.Run(fmt.Sprintf("bucket created by %q inherits the FOO storage class", osu1.Name), func(t *testing.T) {
			liveUser, err := adminClient.GetUser(ctx, admin.User{ID: osu1.Name})
			require.NoError(t, err)
			require.NotEmpty(t, liveUser.Keys)

			s3agent, err := client.NewS3Agent(objectStore, k8sh, store.TLSEnabled(), liveUser.Keys[0].AccessKey, liveUser.Keys[0].SecretKey)
			require.NoError(t, err)

			require.NoError(t, s3agent.CreateBucket(ctx, bucketName))
			t.Cleanup(func() {
				_, _ = s3agent.Client.DeleteBucket(context.Background(), &s3.DeleteBucketInput{Bucket: aws.String(bucketName)})
			})

			wait4.AssertEventually(ctx, t, wait4.TimeoutShort, `bucket placement uses the "FOO" storage class`, func(ctx context.Context) error {
				info, err := adminClient.GetBucketInfo(ctx, admin.Bucket{Bucket: bucketName})
				if err != nil {
					return err
				}

				_, storageClass, _ := strings.Cut(info.PlacementRule, "/")
				if storageClass != "FOO" {
					return fmt.Errorf("bucket placement_rule storage class not yet %q: %q", "FOO", info.PlacementRule)
				}
				return nil
			})
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
