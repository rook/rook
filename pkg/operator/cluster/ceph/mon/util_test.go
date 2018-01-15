/*
Copyright 2018 The Rook Authors. All rights reserved.

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

	"github.com/stretchr/testify/assert"
)

func TestCheckQuorumConsensusForRemoval(t *testing.T) {
	// Quorum sizes with failure tolerance can be found here: https://www.consul.io/docs/internals/consensus.html#deployment-table
	// for quorum below three nodes false is returned
	assert.False(t, checkQuorumConsensusForRemoval(0, 1))
	// one mon should not allow a mon removal
	assert.False(t, checkQuorumConsensusForRemoval(1, 1))
	// two mons should not allow a mon removal
	assert.False(t, checkQuorumConsensusForRemoval(2, 1))
	// three mons should allow one mon removal
	assert.True(t, checkQuorumConsensusForRemoval(3, 1))
	// three mons should allow one mon removal
	assert.True(t, checkQuorumConsensusForRemoval(4, 1))
	// four mons should allow one mons removal
	assert.False(t, checkQuorumConsensusForRemoval(4, 2))

	for i := 5; i <= 10; i++ {
		// i mons, 1 should be allowed to be removed
		assert.True(t, checkQuorumConsensusForRemoval(i, 1))
		// i mons, 2 should be allowed to be removed
		assert.True(t, checkQuorumConsensusForRemoval(i, 2))
	}
}
