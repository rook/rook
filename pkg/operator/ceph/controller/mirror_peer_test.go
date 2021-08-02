/*
Copyright 2021 The Rook Authors. All rights reserved.

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

// Package controller provides Kubernetes controller/pod/container spec items used for many Ceph daemons
package controller

import (
	"encoding/base64"
	"reflect"
	"testing"

	"github.com/pkg/errors"
	cephv1 "github.com/rook/rook/pkg/apis/ceph.rook.io/v1"
	"github.com/rook/rook/pkg/clusterd"
	cephclient "github.com/rook/rook/pkg/daemon/ceph/client"
	exectest "github.com/rook/rook/pkg/util/exec/test"
	"github.com/stretchr/testify/assert"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func TestValidatePeerToken(t *testing.T) {
	// Error: map is empty
	b := &cephv1.CephRBDMirror{}
	data := map[string][]byte{}
	err := ValidatePeerToken(b, data)
	assert.Error(t, err)

	// Error: map is missing pool and site
	data["token"] = []byte("foo")
	err = ValidatePeerToken(b, data)
	assert.Error(t, err)

	// Error: map is missing pool
	data["site"] = []byte("foo")
	err = ValidatePeerToken(b, data)
	assert.Error(t, err)

	// Success CephRBDMirror
	data["pool"] = []byte("foo")
	err = ValidatePeerToken(b, data)
	assert.NoError(t, err)

	// Success CephFilesystem
	// "pool" is not required here
	delete(data, "pool")
	err = ValidatePeerToken(&cephv1.CephFilesystemMirror{}, data)
	assert.NoError(t, err)

	// Success CephFilesystem
	err = ValidatePeerToken(&cephv1.CephFilesystemMirror{}, data)
	assert.NoError(t, err)
}

func TestGenerateStatusInfo(t *testing.T) {
	type args struct {
		object client.Object
	}
	tests := []struct {
		name string
		args args
		want map[string]string
	}{
		// TODO: Add test cases.
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := GenerateStatusInfo(tt.args.object); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("GenerateStatusInfo() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestExpandBootstrapPeerToken(t *testing.T) {
	executor := &exectest.MockExecutor{
		MockExecuteCommandWithOutput: func(command string, args ...string) (string, error) {
			if reflect.DeepEqual(args[0:5], []string{"osd", "pool", "get", "pool", "all"}) {
				return `{"pool_id":13}`, nil
			}

			return "", errors.Errorf("unknown command args: %s", args[0:5])
		},
	}
	c := &clusterd.Context{
		Executor: executor,
	}

	newToken, err := expandBootstrapPeerToken(c, cephclient.AdminClusterInfo("mu-cluster"), []byte(`eyJmc2lkIjoiYzZiMDg3ZjItNzgyOS00ZGJiLWJjZmMtNTNkYzM0ZTBiMzVkIiwiY2xpZW50X2lkIjoicmJkLW1pcnJvci1wZWVyIiwia2V5IjoiQVFBV1lsWmZVQ1Q2RGhBQVBtVnAwbGtubDA5YVZWS3lyRVV1NEE9PSIsIm1vbl9ob3N0IjoiW3YyOjE5Mi4xNjguMTExLjEwOjMzMDAsdjE6MTkyLjE2OC4xMTEuMTA6Njc4OV0sW3YyOjE5Mi4xNjguMTExLjEyOjMzMDAsdjE6MTkyLjE2OC4xMTEuMTI6Njc4OV0sW3YyOjE5Mi4xNjguMTExLjExOjMzMDAsdjE6MTkyLjE2OC4xMTEuMTE6Njc4OV0ifQ==`))
	assert.NoError(t, err)
	newTokenDecoded, err := base64.StdEncoding.DecodeString(string(newToken))
	assert.NoError(t, err)
	assert.Contains(t, string(newTokenDecoded), "namespace")
}
