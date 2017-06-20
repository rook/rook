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
package client

import (
	"testing"

	"github.com/rook/rook/pkg/clusterd"
	"github.com/rook/rook/pkg/util"
	exectest "github.com/rook/rook/pkg/util/exec/test"
	"github.com/stretchr/testify/assert"
)

func TestGetCrushMap(t *testing.T) {
	executor := &exectest.MockExecutor{}
	response, err := GetCrushMap(&clusterd.Context{Executor: executor}, "rook")

	assert.Nil(t, err)
	assert.Equal(t, "", response)
}

func TestCrushLocation(t *testing.T) {
	loc := "dc=datacenter1"

	// test that root will get filled in with default/runtime values
	res, err := FormatLocation(loc)
	assert.Nil(t, err)
	assert.Equal(t, 2, len(res))
	locSet := util.CreateSet(res)
	assert.True(t, locSet.Contains("root=default"))
	assert.True(t, locSet.Contains("dc=datacenter1"))

	// test that if host name and root are already set they will be honored
	loc = "root=otherRoot,dc=datacenter2,host=node123"
	res, err = FormatLocation(loc)
	assert.Nil(t, err)
	assert.Equal(t, 3, len(res))
	locSet = util.CreateSet(res)
	assert.True(t, locSet.Contains("root=otherRoot"))
	assert.True(t, locSet.Contains("dc=datacenter2"))
	assert.True(t, locSet.Contains("host=node123"))

	// test an invalid CRUSH location format
	loc = "root=default,prop:value"
	_, err = FormatLocation(loc)
	assert.NotNil(t, err)
	assert.Contains(t, err.Error(), "is not in a valid format")
}
