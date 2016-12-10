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
package bootstrap

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestSelectDiscoveredNodes(t *testing.T) {
	var ignored []string
	nodeMap := map[uint64][]string{}
	upperIndex := uint64(0)
	size := 1

	// the first entry will be added because it's below size 1
	index5 := uint64(5)
	endpoints5 := []string{"http://10.1.1.5:5000"}
	upperIndex, ignored = addNodeToMap(nodeMap, endpoints5, size, index5, upperIndex, ignored)
	assert.Equal(t, index5, upperIndex)
	assert.Equal(t, 1, len(nodeMap))
	assert.Equal(t, endpoints5, nodeMap[index5])
	assert.Equal(t, 0, len(ignored))

	// the second entry is ignored since size is full and the index is higher
	index6 := uint64(6)
	endpoints6 := []string{"http://10.1.1.6:5000"}
	upperIndex, ignored = addNodeToMap(nodeMap, endpoints6, size, index6, upperIndex, ignored)
	assert.Equal(t, index5, upperIndex)
	assert.Equal(t, 1, len(nodeMap))
	assert.Equal(t, endpoints5, nodeMap[index5])
	assert.Equal(t, 1, len(ignored))
	assert.Equal(t, endpoints6[0], ignored[0])

	// the third entry replaces the first since its index is lower
	index4 := uint64(4)
	endpoints4 := []string{"http://1.1.1.4:5000"}
	upperIndex, ignored = addNodeToMap(nodeMap, endpoints4, size, index4, upperIndex, ignored)
	assert.Equal(t, index4, upperIndex)
	assert.Equal(t, 1, len(nodeMap))
	assert.Equal(t, endpoints4, nodeMap[index4])
	assert.Equal(t, 2, len(ignored))
	assert.Equal(t, endpoints6[0], ignored[0])
	assert.Equal(t, endpoints5[0], ignored[1])

	// increase size to 3
	ignored = []string{}
	size = 3
	upperIndex, ignored = addNodeToMap(nodeMap, endpoints6, size, index6, upperIndex, ignored)
	upperIndex, ignored = addNodeToMap(nodeMap, endpoints5, size, index5, upperIndex, ignored)
	assert.Equal(t, index6, upperIndex)
	assert.Equal(t, 3, len(nodeMap))
	assert.Equal(t, 0, len(ignored))
	assert.Equal(t, index6, upperIndex)

	// replace a node with a lower index
	index3 := uint64(3)
	endpoints3 := []string{"http://1.1.1.3:5000"}
	upperIndex, ignored = addNodeToMap(nodeMap, endpoints3, size, index3, upperIndex, ignored)
	assert.Equal(t, index5, upperIndex)
	assert.Equal(t, 3, len(nodeMap))
	assert.Equal(t, endpoints6[0], ignored[0])

	// replace another node
	index2 := uint64(2)
	endpoints2 := []string{"http://1.1.1.2:5000"}
	upperIndex, ignored = addNodeToMap(nodeMap, endpoints2, size, index2, upperIndex, ignored)
	assert.Equal(t, index4, upperIndex)
	assert.Equal(t, 3, len(nodeMap))
	assert.Equal(t, 2, len(ignored))
}
