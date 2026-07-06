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

// Package obc holds the ObjectBucketClaim test helpers: the provisioner
// StorageClass constructor, the bound/delete lifecycle waiters, and a per-OBC S3
// client.
package obc

import (
	"context"
	"fmt"
	"testing"

	bktv1alpha1 "github.com/kube-object-storage/lib-bucket-provisioner/pkg/apis/objectbucket.io/v1alpha1"
	"github.com/stretchr/testify/require"
	storagev1 "k8s.io/api/storage/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	cephv1 "github.com/rook/rook/pkg/apis/ceph.rook.io/v1"
	rgw "github.com/rook/rook/pkg/operator/ceph/object"
	"github.com/rook/rook/tests/framework/utils"
	"github.com/rook/rook/tests/integration/object/util/client"
	"github.com/rook/rook/tests/integration/object/util/sharedstore"
	"github.com/rook/rook/tests/integration/object/util/wait4"
)

// StorageClass returns a StorageClass for the lib-bucket-provisioner bucket
// provisioner backed by objectStore, for use by ObjectBucketClaims.
func StorageClass(name string, objectStore *cephv1.CephObjectStore) *storagev1.StorageClass {
	return &storagev1.StorageClass{
		ObjectMeta: metav1.ObjectMeta{
			Name: name,
		},
		Provisioner: objectStore.Namespace + ".ceph.rook.io/bucket",
		Parameters: map[string]string{
			"objectStoreName":      objectStore.Name,
			"objectStoreNamespace": objectStore.Namespace,
		},
	}
}

// RequireBound creates obc and waits for it and its backing ObjectBucket to
// bind, returning the live OBC; it aborts the caller if binding fails.
func RequireBound(ctx context.Context, t *testing.T, k8sh *utils.K8sHelper, obc *bktv1alpha1.ObjectBucketClaim) *bktv1alpha1.ObjectBucketClaim {
	t.Helper()

	obcClient := k8sh.BucketClientset.ObjectbucketV1alpha1().ObjectBucketClaims(obc.Namespace)
	obClient := k8sh.BucketClientset.ObjectbucketV1alpha1().ObjectBuckets()

	var live *bktv1alpha1.ObjectBucketClaim
	if !t.Run(fmt.Sprintf("create obc %q", obc.Name), func(t *testing.T) {
		live = wait4.RequireCreate(ctx, t, obcClient, obc, wait4.OBCBound, wait4.TimeoutLong)
		wait4.RequireCondition(ctx, t, obClient, live.Spec.ObjectBucketName, wait4.OBBound, wait4.TimeoutShort)
	}) {
		t.FailNow()
	}
	return live
}

// NewS3Agent builds an S3 client for the bucket provisioned by obcName, using the
// credentials in the OBC's secret and the store's endpoint.
func NewS3Agent(ctx context.Context, t *testing.T, k8sh *utils.K8sHelper, store *sharedstore.Sharedstore, namespace, obcName string) *rgw.S3Agent {
	t.Helper()

	secret, err := k8sh.Clientset.CoreV1().Secrets(namespace).Get(ctx, obcName, metav1.GetOptions{})
	require.NoError(t, err)

	agent, err := client.NewS3Agent(
		store.ObjectStore(), k8sh, store.TLSEnabled(),
		string(secret.Data["AWS_ACCESS_KEY_ID"]),
		string(secret.Data["AWS_SECRET_ACCESS_KEY"]))
	require.NoError(t, err)

	return agent
}

// DeleteAndWait deletes the OBC and waits for its backing ObjectBucket (and thus
// the rgw bucket) to be garbage-collected by the provisioner.
func DeleteAndWait(ctx context.Context, t *testing.T, k8sh *utils.K8sHelper, namespace, name string) {
	t.Helper()

	obcClient := k8sh.BucketClientset.ObjectbucketV1alpha1().ObjectBucketClaims(namespace)
	obClient := k8sh.BucketClientset.ObjectbucketV1alpha1().ObjectBuckets()

	live, err := obcClient.Get(ctx, name, metav1.GetOptions{})
	require.NoError(t, err)
	obName := live.Spec.ObjectBucketName

	wait4.AssertDelete(ctx, t, obcClient, name, wait4.TimeoutShort)
	wait4.AssertAbsent(ctx, t, obClient, obName, wait4.TimeoutShort)
}
