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
	"reflect"
	"testing"

	cephv1 "github.com/rook/rook/pkg/apis/ceph.rook.io/v1"
	"github.com/stretchr/testify/assert"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func TestValidatePeerToken(t *testing.T) {
	// Error: map is empty
	b := &cephv1.CephBlockPool{}
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

	// Success CephBlockPool
	data["pool"] = []byte("foo")
	err = ValidatePeerToken(b, data)
	assert.NoError(t, err)

	// Success CephFilesystem
	data["pool"] = []byte("foo")
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
