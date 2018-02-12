/*
Copyright 2016 The Rook Authors. All rights reserved.

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
package mon

import (
	"testing"

	"github.com/rook/rook/pkg/daemon/ceph/client"
	"github.com/stretchr/testify/assert"
)

func TestMonInQuourm(t *testing.T) {
	entry := client.MonMapEntry{Name: "foo", Rank: 23}
	quorum := []int{}
	// Nothing in quorum
	assert.False(t, monInQuorum(entry, quorum))

	// One or more members in quorum
	quorum = []int{23}
	assert.True(t, monInQuorum(entry, quorum))
	quorum = []int{5, 6, 7, 23, 8}
	assert.True(t, monInQuorum(entry, quorum))

	// Not in quorum
	entry.Rank = 1
	assert.False(t, monInQuorum(entry, quorum))
}

func TestGetMonID(t *testing.T) {
	// invalid
	id, err := GetMonID("m")
	assert.NotNil(t, err)
	assert.Equal(t, -1, id)
	id, err = GetMonID("mon")
	assert.NotNil(t, err)
	assert.Equal(t, -1, id)
	id, err = GetMonID("rook-ceph-mon-0")
	assert.NotNil(t, err)
	assert.Equal(t, -1, id)

	// valid
	id, err = GetMonID("rook-ceph-mon0")
	assert.Nil(t, err)
	assert.Equal(t, 0, id)
	id, err = GetMonID("rook-ceph-mon-1")
	assert.Nil(t, err)
	assert.Equal(t, 123, id)
}

// TODO Add tests for mon start, restart of operator, etc.
