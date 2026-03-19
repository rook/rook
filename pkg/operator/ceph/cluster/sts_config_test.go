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

	t.Run("generates key and configures STS", func(t *testing.T) {
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

		// Verify CephConfig was updated with generated key
		assert.NotNil(t, c.Spec.CephConfig["global"])
		stsKey := c.Spec.CephConfig["global"]["rgw_sts_key"]
		assert.NotEmpty(t, stsKey)
		assert.Len(t, stsKey, STSKeyLength*2)
		err = ValidateSTSKey(stsKey)
		assert.NoError(t, err)
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

// Made with Bob
