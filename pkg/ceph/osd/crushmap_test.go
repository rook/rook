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
	"fmt"
	"strings"
	"testing"

	"github.com/rook/rook/pkg/clusterd"
	"github.com/rook/rook/pkg/util"
	exectest "github.com/rook/rook/pkg/util/exec/test"
	"github.com/stretchr/testify/assert"
)

func TestCrushMap(t *testing.T) {
	testCrushMapHelper(t, &StoreConfig{StoreType: Filestore})
	testCrushMapHelper(t, &StoreConfig{StoreType: Bluestore})
}

func testCrushMapHelper(t *testing.T, storeConfig *StoreConfig) {
	etcdClient := util.NewMockEtcdClient()
	executor := &exectest.MockExecutor{}
	executor.MockExecuteCommandWithOutput = func(name string, command string, args ...string) (string, error) {
		if strings.HasPrefix(name, "lsblk /dev/disk/by-partuuid") {
			// this is a call to get device properties so we figure out CRUSH weight, which should only be done for Bluestore
			// (Filestore uses Statfs since it has a mounted filesystem)
			assert.Equal(t, Bluestore, storeConfig.StoreType)
			return `SIZE="1234567890" TYPE="part"`, nil
		}

		assert.Equal(t, "osd", args[0])
		assert.Equal(t, "crush", args[1])
		assert.Equal(t, "create-or-move", args[2])
		assert.Equal(t, "23", args[3])
		assert.Equal(t, "1.1", args[4]) // weight
		assert.Equal(t, 7, len(args))

		// verify the contents of the CRUSH location args
		/*argsSet := util.CreateSet(request.Args)
		assert.True(t, argsSet.Contains("root=default"))
		assert.True(t, argsSet.Contains("dc=datacenter1"))
		assert.True(t, argsSet.Contains("host=node1"))*/
		assert.Fail(t, fmt.Sprintf("Test crush args from: %+v", args))
		return "", nil
	}
	context := &clusterd.Context{DirectContext: clusterd.DirectContext{EtcdClient: etcdClient, NodeID: "node1"}, Executor: executor}

	location := "root=default,dc=datacenter1,host=node1"

	config := &osdConfig{id: 23, rootPath: "/"}
	if storeConfig.StoreType == Bluestore {
		// if we're using bluestore, give some extra partition config info, the addOSDToCrushMap call will need it
		config.partitionScheme = NewPerfSchemeEntry(storeConfig.StoreType)
		PopulateCollocatedPerfSchemeEntry(config.partitionScheme, "sda", *storeConfig)
	}

	err := addOSDToCrushMap(context, config, "rook", location)
	assert.Nil(t, err)

	// location should have been stored in etcd as well
	assert.Equal(t, location, etcdClient.GetValue("/rook/nodes/config/node1/location"))
}

func TestGetCrushMap(t *testing.T) {
	executor := &exectest.MockExecutor{}
	response, err := GetCrushMap(&clusterd.Context{Executor: executor}, "rook")

	assert.Nil(t, err)
	assert.Equal(t, "", response)
}

func TestCrushLocation(t *testing.T) {
	loc := "dc=datacenter1"

	// test that root will get filled in with default/runtime values
	res, err := formatLocation(loc)
	assert.Nil(t, err)
	assert.Equal(t, 2, len(res))
	locSet := util.CreateSet(res)
	assert.True(t, locSet.Contains("root=default"))
	assert.True(t, locSet.Contains("dc=datacenter1"))

	// test that if host name and root are already set they will be honored
	loc = "root=otherRoot,dc=datacenter2,host=node123"
	res, err = formatLocation(loc)
	assert.Nil(t, err)
	assert.Equal(t, 3, len(res))
	locSet = util.CreateSet(res)
	assert.True(t, locSet.Contains("root=otherRoot"))
	assert.True(t, locSet.Contains("dc=datacenter2"))
	assert.True(t, locSet.Contains("host=node123"))

	// test an invalid CRUSH location format
	loc = "root=default,prop:value"
	_, err = formatLocation(loc)
	assert.NotNil(t, err)
	assert.Contains(t, err.Error(), "is not in a valid format")
}
