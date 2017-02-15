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
package osd

import (
	"testing"

	"os"

	"github.com/coreos/pkg/capnslog"
	"github.com/rook/rook/pkg/clusterd"
	"github.com/stretchr/testify/assert"
)

func TestStoreOSDDirMap(t *testing.T) {
	context := clusterd.NewDaemonContext("/tmp/testdir", "", capnslog.INFO)
	defer os.RemoveAll(context.ConfigDir)
	os.MkdirAll(context.ConfigDir, 0755)

	dirMap, err := getDataDirs(context, false)
	assert.Nil(t, err)
	assert.Equal(t, 0, len(dirMap))

	dirMap, err = getDataDirs(context, true)
	assert.Nil(t, err)
	assert.Equal(t, 1, len(dirMap))
	assert.Equal(t, unassignedOSDID, dirMap[context.ConfigDir])
	dirMap[context.ConfigDir] = 0

	err = saveDirConfig(context, dirMap)
	assert.Nil(t, err)

	dirMap, err = getDataDirs(context, false)
	assert.Nil(t, err)
	assert.Equal(t, 1, len(dirMap))
	assert.Equal(t, 0, dirMap[context.ConfigDir])

	// add another directory to the map
	dirMap["/tmp/mydir"] = 23
	err = saveDirConfig(context, dirMap)
	assert.Nil(t, err)

	dirMap, err = getDataDirs(context, false)
	assert.Nil(t, err)
	assert.Equal(t, 2, len(dirMap))
	assert.Equal(t, 0, dirMap[context.ConfigDir])
	assert.Equal(t, 23, dirMap["/tmp/mydir"])
}
