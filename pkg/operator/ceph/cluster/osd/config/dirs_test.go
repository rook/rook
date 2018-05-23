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

// Package config for OSD config managed by the operator
package config

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"k8s.io/apimachinery/pkg/api/errors"
)

func TestLoadOSDDirMap(t *testing.T) {
	kv := mockKVStore()
	nodeName := "node418"

	// no dir map exists yet, load should return not found
	dirMap, err := LoadOSDDirMap(kv, nodeName)
	assert.True(t, errors.IsNotFound(err))
	assert.Equal(t, 0, len(dirMap))

	// add some items to the dir map and save it
	dirMap = map[string]int{
		"/foo/bar": 3,
		"/baz/biz": 55,
	}
	err = SaveOSDDirMap(kv, nodeName, dirMap)
	assert.Nil(t, err)

	// load the dir map, it should be equal to what we saved
	loadedDirMap, err := LoadOSDDirMap(kv, nodeName)
	assert.Nil(t, err)
	assert.Equal(t, dirMap, loadedDirMap)
}
