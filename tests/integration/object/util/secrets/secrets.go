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

// Package secrets holds verification helpers for the Secret references that
// object CRDs publish in their status, shared by more than one object test
// package. Verification helpers for a different resource belong in a package
// named for that resource, not here.
package secrets

import (
	"context"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"

	cephv1 "github.com/rook/rook/pkg/apis/ceph.rook.io/v1"
	"github.com/rook/rook/tests/framework/utils"
	"github.com/rook/rook/tests/integration/object/util/wait4"
)

// RequireStatusRefs runs a subtest named desc that waits for the status secret
// references returned by refsOf to reach len(expected) — the controller
// repopulates them asynchronously after a reconcile — and then asserts each ref
// against its live secret via AssertRefs. refsOf must tolerate an unpopulated
// status (return nil).
func RequireStatusRefs[T, L runtime.Object](
	t *testing.T,
	k8sh *utils.K8sHelper,
	client wait4.NamespacedWatcher[T, L],
	name, desc string,
	refsOf func(T) []cephv1.SecretReference,
	expected ...*corev1.Secret,
) {
	t.Helper()
	t.Run(desc, func(t *testing.T) {
		ctx := t.Context()
		live := wait4.RequireCondition(ctx, t, client, name, func(obj T) bool {
			return len(refsOf(obj)) == len(expected)
		}, wait4.TimeoutShort)
		AssertRefs(ctx, t, k8sh, refsOf(live), expected...)
	})
}

// AssertRefs verifies that refs contains exactly one SecretReference per
// expected Secret and that each matches the live secret's Name, Namespace,
// UID, and ResourceVersion. Live secrets are fetched from each expected
// secret's own namespace.
func AssertRefs(ctx context.Context, t *testing.T, k8sh *utils.K8sHelper, refs []cephv1.SecretReference, expected ...*corev1.Secret) {
	t.Helper()

	assert.Len(t, refs, len(expected))

	for _, secret := range expected {
		secretRef, err := findSecretRef(refs, secret.Name)
		require.NoError(t, err)

		// fetch the live secret for UID and ResourceVersion
		liveSecret, err := k8sh.Clientset.CoreV1().Secrets(secret.Namespace).Get(ctx, secret.Name, metav1.GetOptions{})
		require.NoError(t, err)

		assert.Equal(t, liveSecret.Name, secretRef.Name)
		assert.Equal(t, liveSecret.Namespace, secretRef.Namespace)
		assert.Equal(t, liveSecret.UID, secretRef.UID)
		assert.Equal(t, liveSecret.ResourceVersion, secretRef.ResourceVersion)
	}
}

// findSecretRef returns the SecretReference for the named secret.
func findSecretRef(refs []cephv1.SecretReference, name string) (cephv1.SecretReference, error) {
	for _, ref := range refs {
		if ref.Name == name {
			return ref, nil
		}
	}
	return cephv1.SecretReference{}, fmt.Errorf("secretReference for secret %q not found", name)
}
