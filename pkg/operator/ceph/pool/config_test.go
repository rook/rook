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
	"testing"

	cephv1 "github.com/rook/rook/pkg/apis/ceph.rook.io/v1"
	"github.com/stretchr/testify/assert"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
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
