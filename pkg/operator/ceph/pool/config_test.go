/*
Copyright 2020 The Rook Authors. All rights reserved.

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

// Package pool to manage a rook pool.
package pool

import (
	"encoding/base64"
	"reflect"
	"testing"

	"github.com/pkg/errors"
	cephv1 "github.com/rook/rook/pkg/apis/ceph.rook.io/v1"
	rookclient "github.com/rook/rook/pkg/client/clientset/versioned/fake"
	"github.com/rook/rook/pkg/clusterd"
	cephclient "github.com/rook/rook/pkg/daemon/ceph/client"
	testop "github.com/rook/rook/pkg/operator/test"
	exectest "github.com/rook/rook/pkg/util/exec/test"
	"github.com/stretchr/testify/assert"
	"github.com/tevino/abool"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
)

func TestGenerateStatusInfo(t *testing.T) {
	p := &cephv1.CephBlockPool{
		ObjectMeta: v1.ObjectMeta{
			Name:      "foo",
			Namespace: "rook-ceph",
		},
	}

	info := generateStatusInfo(p)
	secretName := info["rbdMirrorBootstrapPeerSecretName"]
	assert.NotEmpty(t, secretName)
	assert.Equal(t, "pool-peer-token-foo", secretName)
}

func TestExpandBootstrapPeerToken(t *testing.T) {
	executor := &exectest.MockExecutor{
		MockExecuteCommandWithOutputFile: func(command, outfile string, args ...string) (string, error) {
			if reflect.DeepEqual(args[0:5], []string{"osd", "pool", "get", "pool", "all"}) {
				return `{"pool_id":13}`, nil
			}

			return "", errors.Errorf("unknown command args: %s", args[0:5])
		},
	}
	c := &clusterd.Context{
		Executor:                   executor,
		Clientset:                  testop.New(t, 1),
		RookClientset:              rookclient.NewSimpleClientset(),
		RequestCancelOrchestration: abool.New(),
	}

	// Create a ReconcileCephBlockPool object with the scheme and fake client.
	r := &ReconcileCephBlockPool{
		context:     c,
		clusterInfo: &cephclient.ClusterInfo{Namespace: "rook-ceph"},
	}

	newToken, err := r.expandBootstrapPeerToken(&cephv1.CephBlockPool{ObjectMeta: v1.ObjectMeta{Name: "pool", Namespace: "rook-ceph"}}, types.NamespacedName{Namespace: "rook-ceph"}, []byte(`eyJmc2lkIjoiYzZiMDg3ZjItNzgyOS00ZGJiLWJjZmMtNTNkYzM0ZTBiMzVkIiwiY2xpZW50X2lkIjoicmJkLW1pcnJvci1wZWVyIiwia2V5IjoiQVFBV1lsWmZVQ1Q2RGhBQVBtVnAwbGtubDA5YVZWS3lyRVV1NEE9PSIsIm1vbl9ob3N0IjoiW3YyOjE5Mi4xNjguMTExLjEwOjMzMDAsdjE6MTkyLjE2OC4xMTEuMTA6Njc4OV0sW3YyOjE5Mi4xNjguMTExLjEyOjMzMDAsdjE6MTkyLjE2OC4xMTEuMTI6Njc4OV0sW3YyOjE5Mi4xNjguMTExLjExOjMzMDAsdjE6MTkyLjE2OC4xMTEuMTE6Njc4OV0ifQ==`))
	assert.NoError(t, err)
	newTokenDecoded, err := base64.StdEncoding.DecodeString(string(newToken))
	assert.NoError(t, err)
	assert.Contains(t, string(newTokenDecoded), "pool_id")
	assert.Contains(t, string(newTokenDecoded), "namespace")
}
