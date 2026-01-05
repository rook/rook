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

package k8sutil

import (
	"context"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	k8sfake "k8s.io/client-go/kubernetes/fake"
	k8stesting "k8s.io/client-go/testing"
	"k8s.io/utils/ptr"
)

func TestIsSameOwnerReference(t *testing.T) {
	// Matching owner references
	refA := metav1.OwnerReference{
		APIVersion: "ceph.rook.io/v1",
		Kind:       "CephClient",
		Name:       "my-client",
	}
	refB := metav1.OwnerReference{
		APIVersion: "ceph.rook.io/v1",
		Kind:       "CephClient",
		Name:       "my-client",
	}

	// Mismatched owner references
	refC := metav1.OwnerReference{
		APIVersion: "ceph.rook.io/v1",
		Kind:       "CephClient",
		Name:       "different-client",
	}

	// Test matching references
	assert.True(t, IsSameOwnerReference(refA, refB), "expected owner references to match")

	// Test mismatched references
	assert.False(t, IsSameOwnerReference(refA, refC), "expected owner references to not match")
}

func TestDeleteSecretIfOwnedBy(t *testing.T) {
	owner := metav1.OwnerReference{
		APIVersion: "v1",
		Kind:       "CephCluster",
		Name:       "ceph-cluster",
		Controller: ptr.To(true),
	}

	tests := []struct {
		name            string
		secret          *corev1.Secret
		getSecretErr    error
		deleteSecretErr error
		expectedDelete  bool
		expectedErr     bool
	}{
		{
			name:           "secret not found",
			getSecretErr:   k8serrors.NewNotFound(schema.GroupResource{Group: "", Resource: "secrets"}, "my-secret"),
			expectedDelete: false,
			expectedErr:    false,
		},
		{
			name:           "error getting secret",
			getSecretErr:   fmt.Errorf("API error"),
			expectedDelete: false,
			expectedErr:    true,
		},
		{
			name: "secret has no owner",
			secret: &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "my-secret",
					Namespace: "default",
				},
			},
			expectedDelete: false,
			expectedErr:    false,
		},
		{
			name: "secret has different owner",
			secret: &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "my-secret",
					Namespace: "default",
					OwnerReferences: []metav1.OwnerReference{{
						APIVersion: "v1",
						Kind:       "CephCluster",
						Name:       "other-ceph-cluster",
						Controller: ptr.To(true),
					}},
				},
			},
			expectedDelete: false,
			expectedErr:    false,
		},
		{
			name: "secret owned by caller, deletion succeeds",
			secret: &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "my-secret",
					Namespace: "default",
					OwnerReferences: []metav1.OwnerReference{{
						APIVersion: "v1",
						Kind:       "CephCluster",
						Name:       "my-ceph-cluster",
						Controller: ptr.To(true),
					}},
				},
			},
			expectedDelete: true,
			expectedErr:    false,
		},
		{
			name: "secret owned by caller, deletion returns error",
			secret: &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:            "my-secret",
					Namespace:       "default",
					OwnerReferences: []metav1.OwnerReference{owner},
				},
			},
			deleteSecretErr: fmt.Errorf("delete error"),
			expectedDelete:  true,
			expectedErr:     true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client := k8sfake.NewClientset()

			if tt.getSecretErr != nil || tt.secret != nil {
				client.Fake.PrependReactor("get", "secrets", func(action k8stesting.Action) (handled bool, ret runtime.Object, err error) {
					if tt.getSecretErr != nil {
						return true, nil, tt.getSecretErr
					}
					return true, tt.secret, nil
				})
			}

			if tt.expectedDelete {
				client.Fake.PrependReactor("delete", "secrets", func(action k8stesting.Action) (handled bool, ret runtime.Object, err error) {
					return true, nil, tt.deleteSecretErr
				})
			}

			err := DeleteSecretIfOwnedBy(context.TODO(), client, "my-secret", "default", owner)
			if tt.expectedErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestUpdateSecretIfOwnedBy(t *testing.T) {
	ctx := context.TODO()
	client := k8sfake.NewClientset()

	expectedOwner := metav1.OwnerReference{
		APIVersion: "v1",
		Kind:       "CephCluster",
		Name:       "rook-ceph",
		Controller: ptr.To(true),
	}

	namespace := "default"
	// 1. Secret owned by expected controller → should be updated
	secret1 := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-secret",
			Namespace: namespace,
			OwnerReferences: []metav1.OwnerReference{
				expectedOwner,
			},
		},
		Data: map[string][]byte{
			"foo": []byte("bar"),
		},
	}

	// Create the secret
	_, err := client.CoreV1().Secrets(namespace).Create(ctx, secret1, metav1.CreateOptions{})
	assert.NoError(t, err)

	// Modify it locally before update
	secret1.Data["foo"] = []byte("updated")

	err = UpdateSecretIfOwnedBy(ctx, client, secret1)
	assert.NoError(t, err)

	updated, _ := client.CoreV1().Secrets(namespace).Get(ctx, secret1.Name, metav1.GetOptions{})
	assert.Equal(t, []byte("updated"), updated.Data["foo"])

	// 2. Secret without any owner
	secret2 := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "without-owner",
			Namespace: namespace,
		},
		Data: map[string][]byte{
			"key": []byte("val"),
		},
	}
	_, err = client.CoreV1().Secrets(namespace).Create(ctx, secret2, metav1.CreateOptions{})
	assert.NoError(t, err)

	secret2.Data["key"] = []byte("should-not-update")

	err = UpdateSecretIfOwnedBy(ctx, client, secret2)
	assert.Error(t, err)

	// 3. Secret owned by someone else → should not be updated
	otherOwner := metav1.OwnerReference{
		APIVersion: "v1",
		Kind:       "OtherController",
		Name:       "other",
		Controller: ptr.To(true),
	}
	secret3 := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "owned-by-others",
			Namespace: namespace,
			OwnerReferences: []metav1.OwnerReference{
				otherOwner,
			},
		},
		Data: map[string][]byte{
			"key": []byte("before"),
		},
	}
	_, err = client.CoreV1().Secrets(namespace).Create(ctx, secret3, metav1.CreateOptions{})
	assert.NoError(t, err)

	secret3.Data["key"] = []byte("should-not-update")

	secret3.OwnerReferences = []metav1.OwnerReference{expectedOwner}
	err = UpdateSecretIfOwnedBy(ctx, client, secret3)
	assert.Error(t, err)

	// 4. Secret does not exist → should return error
	secret4 := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "nonexistent",
			Namespace: namespace,
		},
	}
	err = UpdateSecretIfOwnedBy(ctx, client, secret4)
	assert.Error(t, err)
}
