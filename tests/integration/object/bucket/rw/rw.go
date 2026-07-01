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

package rw

import (
	"fmt"
	"testing"

	bktv1alpha1 "github.com/kube-object-storage/lib-bucket-provisioner/pkg/apis/objectbucket.io/v1alpha1"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/rook/rook/tests/framework/utils"
	"github.com/rook/rook/tests/integration/object/util/fixture"
	"github.com/rook/rook/tests/integration/object/util/obc"
	"github.com/rook/rook/tests/integration/object/util/sharedstore"
	"github.com/rook/rook/tests/integration/object/util/wait4"
)

const Namespace = "test-bucketrw"

func TestObjectBucketClaimReadWrite(t *testing.T, k8sh *utils.K8sHelper, store *sharedstore.Sharedstore) {
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
			},
		}

		body        = "test bucket read/write payload"
		contentType = "text/plain"
		key         = "obj1"

		obcClient = k8sh.BucketClientset.ObjectbucketV1alpha1().ObjectBucketClaims(ns.Name)
	)

	t.Run("ObjectBucketClaim read/write", func(t *testing.T) {
		ctx := t.Context()

		fixture.RequireNamespace(t, k8sh, ns)
		fixture.RequireStorageClass(t, k8sh, storageClass)

		obc.RequireBound(ctx, t, k8sh, obc1)
		s3 := obc.NewS3Agent(ctx, t, k8sh, store, ns.Name, obc1.Name)

		t.Run(fmt.Sprintf("put object %q", key), func(t *testing.T) {
			_, err := s3.PutObjectInBucket(ctx, obc1.Spec.BucketName, body, key, contentType)
			require.NoError(t, err)
		})

		t.Run(fmt.Sprintf("get object %q round-trips", key), func(t *testing.T) {
			read, err := s3.GetObjectInBucket(ctx, obc1.Spec.BucketName, key)
			require.NoError(t, err)
			assert.Equal(t, body, read)
		})

		t.Run(fmt.Sprintf("delete object %q", key), func(t *testing.T) {
			_, err := s3.DeleteObjectInBucket(ctx, obc1.Spec.BucketName, key)
			assert.NoError(t, err)
		})

		t.Run(fmt.Sprintf("obc %q stays Bound", obc1.Name), func(t *testing.T) {
			wait4.AssertCondition(ctx, t, obcClient, obc1.Name, wait4.OBCBound, wait4.TimeoutShort)
		})

		t.Run(fmt.Sprintf("delete obc %q", obc1.Name), func(t *testing.T) {
			obc.DeleteAndWait(ctx, t, k8sh, ns.Name, obc1.Name)
		})

		t.Run(fmt.Sprintf("no obc(s) in ns %q", ns.Name), func(t *testing.T) {
			list, err := obcClient.List(ctx, metav1.ListOptions{})
			require.NoError(t, err)
			assert.Len(t, list.Items, 0)
		})
	})
}
