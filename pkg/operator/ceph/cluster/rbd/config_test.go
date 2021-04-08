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

package rbd

import (
	"testing"

	cephv1 "github.com/rook/rook/pkg/apis/ceph.rook.io/v1"
	"github.com/stretchr/testify/assert"
)

func TestValidateSpec(t *testing.T) {
	// Invalid count
	r := &cephv1.RBDMirroringSpec{Count: 0}
	err := validateSpec(r)
	assert.Error(t, err)

	// Correct count
	r.Count = 1
	err = validateSpec(r)
	assert.NoError(t, err)

	// Valid only a single peer
	r.Peers.SecretNames = append(r.Peers.SecretNames, "foo")
	err = validateSpec(r)
	assert.NoError(t, err)

	// Multiple pools mirroring are supported with the same peer is supported
	r.Peers.SecretNames = append(r.Peers.SecretNames, "bar")
	err = validateSpec(r)
	assert.NoError(t, err)
}
