/*
Copyright 2024 The Rook Authors. All rights reserved.

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

package cluster

import (
	"context"
	"testing"

	cephv1 "github.com/rook/rook/pkg/apis/ceph.rook.io/v1"
	"github.com/rook/rook/pkg/clusterd"
	"github.com/rook/rook/pkg/daemon/ceph/client"
	"github.com/rook/rook/pkg/operator/k8sutil"
	"github.com/stretchr/testify/assert"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes/fake"
)

func TestGenerateSTSKey(t *testing.T) {
	key, err := generateSTSKey()
	assert.NoError(t, err)
	assert.Len(t, key, STSKeyLength*2, "STS key should be 16 hex characters")

	// Validate the key
	err = ValidateSTSKey(key)
	assert.NoError(t, err)

	// Generate another key and ensure they're different
	key2, err := generateSTSKey()
	assert.NoError(t, err)
	assert.NotEqual(t, key, key2, "Generated keys should be unique")
}

func TestValidateSTSKey(t *testing.T) {
	tests := []struct {
		name    string
		key     string
		wantErr bool
	}{
		{
			name:    "valid key",
			key:     "a1b2c3d4e5f6a7b8",
			wantErr: false,
		},
		{
			name:    "too short",
			key:     "a1b2c3d4",
			wantErr: true,
		},
		{
			name:    "too long",
			key:     "a1b2c3d4e5f6a7b8c9d0",
			wantErr: true,
		},
		{
			name:    "invalid hex",
			key:     "g1h2i3j4k5l6m7n8",
			wantErr: true,
		},
		{
			name:    "empty",
			key:     "",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateSTSKey(tt.key)
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestEnsureSTSConfiguration(t *testing.T) {
	ctx := context.TODO()
	ns := "rook-ceph"

	t.Run("creates secret and configures STS", func(t *testing.T) {
		clientset := fake.NewSimpleClientset()
		clusterInfo := client.AdminClusterInfo(ctx, ns, "test-cluster")
		context := &clusterd.Context{
			Clientset: clientset,
		}

		c := &cluster{
			context:   context,
			Namespace: ns,
			Spec: &cephv1.ClusterSpec{
				CephConfig: make(map[string]map[string]string),
			},
			ownerInfo:   &k8sutil.OwnerInfo{},
			ClusterInfo: clusterInfo,
		}

		// Ensure STS configuration
		err := c.ensureSTSConfiguration()
		assert.NoError(t, err)

		// Verify secret was created
		secret, err := clientset.CoreV1().Secrets(ns).Get(ctx, STSKeySecretName, metav1.GetOptions{})
		assert.NoError(t, err)
		assert.NotNil(t, secret)

		// Verify secret contains valid STS key
		stsKey, ok := secret.Data[STSKeySecretKey]
		assert.True(t, ok)
		assert.Len(t, stsKey, STSKeyLength*2)
		err = ValidateSTSKey(string(stsKey))
		assert.NoError(t, err)

		// Verify CephConfig was updated
		assert.NotNil(t, c.Spec.CephConfig["global"])
		assert.Equal(t, string(stsKey), c.Spec.CephConfig["global"]["rgw_sts_key"])
		assert.Equal(t, "true", c.Spec.CephConfig["global"]["rgw_s3_auth_use_sts"])
	})

	t.Run("uses existing secret", func(t *testing.T) {
		existingKey := "1234567890abcdef"
		secret := &v1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name:      STSKeySecretName,
				Namespace: ns,
			},
			Data: map[string][]byte{
				STSKeySecretKey: []byte(existingKey),
			},
		}

		clientset := fake.NewSimpleClientset(secret)
		clusterInfo := client.AdminClusterInfo(ctx, ns, "test-cluster")
		context := &clusterd.Context{
			Clientset: clientset,
		}

		c := &cluster{
			context:   context,
			Namespace: ns,
			Spec: &cephv1.ClusterSpec{
				CephConfig: make(map[string]map[string]string),
			},
			ownerInfo:   &k8sutil.OwnerInfo{},
			ClusterInfo: clusterInfo,
		}

		// Ensure STS configuration
		err := c.ensureSTSConfiguration()
		assert.NoError(t, err)

		// Verify existing key is used
		assert.Equal(t, existingKey, c.Spec.CephConfig["global"]["rgw_sts_key"])
		assert.Equal(t, "true", c.Spec.CephConfig["global"]["rgw_s3_auth_use_sts"])
	})

	t.Run("preserves user-configured values", func(t *testing.T) {
		userKey := "fedcba0987654321"
		clientset := fake.NewSimpleClientset()
		clusterInfo := client.AdminClusterInfo(ctx, ns, "test-cluster")
		context := &clusterd.Context{
			Clientset: clientset,
		}

		c := &cluster{
			context:   context,
			Namespace: ns,
			Spec: &cephv1.ClusterSpec{
				CephConfig: map[string]map[string]string{
					"global": {
						"rgw_sts_key":         userKey,
						"rgw_s3_auth_use_sts": "false", // User explicitly disabled
					},
				},
			},
			ownerInfo:   &k8sutil.OwnerInfo{},
			ClusterInfo: clusterInfo,
		}

		// Ensure STS configuration
		err := c.ensureSTSConfiguration()
		assert.NoError(t, err)

		// Verify user values are preserved
		assert.Equal(t, userKey, c.Spec.CephConfig["global"]["rgw_sts_key"])
		assert.Equal(t, "false", c.Spec.CephConfig["global"]["rgw_s3_auth_use_sts"])
	})
}

func TestGetSTSKey(t *testing.T) {
	ns := "rook-ceph"
	expectedKey := "a1b2c3d4e5f6a7b8"

	secret := &v1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      STSKeySecretName,
			Namespace: ns,
		},
		Data: map[string][]byte{
			STSKeySecretKey: []byte(expectedKey),
		},
	}

	clientset := fake.NewSimpleClientset(secret)
	context := &clusterd.Context{
		Clientset: clientset,
	}

	key, err := GetSTSKey(context, ns)
	assert.NoError(t, err)
	assert.Equal(t, expectedKey, key)
}

func TestGetSTSKeyNotFound(t *testing.T) {
	ns := "rook-ceph"
	clientset := fake.NewSimpleClientset()
	context := &clusterd.Context{
		Clientset: clientset,
	}

	_, err := GetSTSKey(context, ns)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to get STS key secret")
}

func createFakeCluster(t *testing.T, objects ...runtime.Object) *cluster {
	ctx := context.TODO()
	ns := "rook-ceph"
	clientset := fake.NewSimpleClientset(objects...)
	clusterInfo := client.AdminClusterInfo(ctx, ns, "test-cluster")
	context := &clusterd.Context{
		Clientset: clientset,
	}

	return &cluster{
		context:   context,
		Namespace: ns,
		Spec: &cephv1.ClusterSpec{
			CephConfig: make(map[string]map[string]string),
		},
		ownerInfo:   &k8sutil.OwnerInfo{},
		ClusterInfo: clusterInfo,
	}
}

// Made with Bob
