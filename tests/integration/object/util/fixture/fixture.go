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

// Package fixture provides setup helpers for resources whose teardown is pure
// cleanup rather than part of what a test asserts. Each helper creates the
// resource and registers its deletion via t.Cleanup, so the resource is
// removed even when the test fails partway through and never reaches its
// trailing teardown steps — important because the tests share one cluster.
//
// Deletions that are themselves assertions (e.g. "deleting the OBC must not
// delete the user") do not belong here; keep those as ordered subtests.
package fixture

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	storagev1 "k8s.io/api/storage/v1"
	kerrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/rook/rook/tests/framework/utils"
)

// The helpers manage their own contexts rather than taking one from the
// caller: cleanup runs after the test finishes, when a test-scoped context
// (e.g. t.Context()) has already been canceled.

// RequireNamespace creates ns and registers its deletion via t.Cleanup.
// Deletion is issued without waiting for it to complete; namespace teardown is
// asynchronous and nothing in these tests depends on it finishing.
func RequireNamespace(t *testing.T, k8sh *utils.K8sHelper, ns *corev1.Namespace) {
	t.Helper()

	_, err := k8sh.Clientset.CoreV1().Namespaces().Create(context.Background(), ns, metav1.CreateOptions{})
	require.NoError(t, err)

	t.Cleanup(func() {
		err := k8sh.Clientset.CoreV1().Namespaces().Delete(context.Background(), ns.Name, metav1.DeleteOptions{})
		if err != nil && !kerrors.IsNotFound(err) {
			t.Errorf("failed to delete namespace %q: %v", ns.Name, err)
		}
	})
}

// RequireStorageClass creates sc and registers its deletion via t.Cleanup.
func RequireStorageClass(t *testing.T, k8sh *utils.K8sHelper, sc *storagev1.StorageClass) {
	t.Helper()

	_, err := k8sh.Clientset.StorageV1().StorageClasses().Create(context.Background(), sc, metav1.CreateOptions{})
	require.NoError(t, err)

	t.Cleanup(func() {
		err := k8sh.Clientset.StorageV1().StorageClasses().Delete(context.Background(), sc.Name, metav1.DeleteOptions{})
		if err != nil && !kerrors.IsNotFound(err) {
			t.Errorf("failed to delete storageclass %q: %v", sc.Name, err)
		}
	})
}
