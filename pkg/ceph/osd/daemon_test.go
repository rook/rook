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
	"io/ioutil"
	"strings"
	"testing"

	"os"

	"github.com/rook/rook/pkg/clusterd"
	exectest "github.com/rook/rook/pkg/util/exec/test"
	"github.com/rook/rook/pkg/util/kvstore"
	"github.com/stretchr/testify/assert"
)

func TestStoreOSDDirMap(t *testing.T) {
	configDir, _ := ioutil.TempDir("", "")
	defer os.RemoveAll(configDir)
	context := &clusterd.Context{ConfigDir: configDir}
	defer os.RemoveAll(context.ConfigDir)
	os.MkdirAll(context.ConfigDir, 0755)

	kv := kvstore.NewMockKeyValueStore()
	nodeName := "node6046"

	// user has specified devices to use, no dirs should be returned
	dirMap, err := getDataDirs(context, kv, "", true, nodeName)
	assert.Nil(t, err)
	assert.Equal(t, 0, len(dirMap))

	// user has no devices specified, should return default dir
	dirMap, err = getDataDirs(context, kv, "", false, nodeName)
	assert.Nil(t, err)
	assert.Equal(t, 1, len(dirMap))
	assert.Equal(t, unassignedOSDID, dirMap[context.ConfigDir])

	// user has no devices specified but does specify dirs, those should be returned
	dirMap, err = getDataDirs(context, kv, "/rook/dir1", false, nodeName)
	assert.Nil(t, err)
	assert.Equal(t, 1, len(dirMap))
	assert.Equal(t, unassignedOSDID, dirMap["/rook/dir1"])
	dirMap["/rook/dir1"] = 0 // simulate an OSD ID being assigned to the dir

	// save the directory config
	err = saveOSDDirMap(kv, nodeName, dirMap)
	assert.Nil(t, err)

	// user has specified devices to use, we should still return the saved dir
	dirMap, err = getDataDirs(context, kv, "", true, nodeName)
	assert.Nil(t, err)
	assert.Equal(t, 1, len(dirMap))
	assert.Equal(t, 0, dirMap["/rook/dir1"])

	// user has specified devices and also a directory to use.  it should be added to the dir map
	dirMap, err = getDataDirs(context, kv, "/tmp/mydir", true, nodeName)
	assert.Nil(t, err)
	assert.Equal(t, 2, len(dirMap))
	assert.Equal(t, 0, dirMap["/rook/dir1"])
	assert.Equal(t, unassignedOSDID, dirMap["/tmp/mydir"])

	// simulate that the user's dir got an OSD by assigning it an ID
	dirMap["/tmp/mydir"] = 23
	err = saveOSDDirMap(kv, nodeName, dirMap)
	assert.Nil(t, err)

	// user is still specifying the directory, we should get back it's ID now
	dirMap, err = getDataDirs(context, kv, "/tmp/mydir", true, nodeName)
	assert.Nil(t, err)
	assert.Equal(t, 2, len(dirMap))
	assert.Equal(t, 0, dirMap["/rook/dir1"])
	assert.Equal(t, 23, dirMap["/tmp/mydir"])
}

func TestAvailableDevices(t *testing.T) {
	executor := &exectest.MockExecutor{}
	// set up a mock function to return "rook owned" partitions on the device and it does not have a filesystem
	executor.MockExecuteCommandWithOutput = func(debug bool, name string, command string, args ...string) (string, error) {
		logger.Infof("OUTPUT for %s. %s %+v", name, command, args)

		if command == "lsblk" {
			if strings.Index(name, "sdb") != -1 {
				// /dev/sdb has a partition
				return `NAME="sdb" SIZE="65" TYPE="disk" PKNAME=""
NAME="sdb1" SIZE="30" TYPE="part" PKNAME="sdb"`, nil
			}
			return "", nil
		} else if command == "blkid" {
			if strings.Index(name, "sdb1") != -1 {
				// partition sdb1 has a label MY-PART
				return "MY-PART", nil
			}
		} else if command == "df" {
			if strings.Index(name, "sdc") != -1 {
				// /dev/sdc has a file system
				return "/dev/sdc ext4", nil
			}
			return "", nil
		}

		return "", fmt.Errorf("unknown command %s %+v", command, args)
	}

	context := &clusterd.Context{Executor: executor}
	context.Devices = []*clusterd.LocalDisk{
		{Name: "sda"},
		{Name: "sdb"},
		{Name: "sdc"},
		{Name: "sdd"},
		{Name: "nvme01"},
		{Name: "rda"},
		{Name: "rdb"},
	}

	// select all devices, including nvme01 for metadata
	mapping, err := getAvailableDevices(context, "all", "nvme01", true)
	assert.Nil(t, err)
	assert.Equal(t, 5, len(mapping.Entries))
	assert.Equal(t, -1, mapping.Entries["sda"].Data)
	assert.Equal(t, -1, mapping.Entries["sdd"].Data)
	assert.Equal(t, -1, mapping.Entries["rda"].Data)
	assert.Equal(t, -1, mapping.Entries["rdb"].Data)
	assert.Equal(t, -1, mapping.Entries["nvme01"].Data)
	assert.NotNil(t, mapping.Entries["nvme01"].Metadata)
	assert.Equal(t, 0, len(mapping.Entries["nvme01"].Metadata))

	// select no devices both using and not using a filter
	mapping, err = getAvailableDevices(context, "", "", false)
	assert.Nil(t, err)
	assert.Equal(t, 0, len(mapping.Entries))

	mapping, err = getAvailableDevices(context, "", "", true)
	assert.Nil(t, err)
	assert.Equal(t, 0, len(mapping.Entries))

	// select the sd* devices
	mapping, err = getAvailableDevices(context, "^sd.$", "", true)
	assert.Nil(t, err)
	assert.Equal(t, 2, len(mapping.Entries))
	assert.Equal(t, -1, mapping.Entries["sda"].Data)
	assert.Equal(t, -1, mapping.Entries["sdd"].Data)

	// select an exact device
	mapping, err = getAvailableDevices(context, "sdd", "", false)
	assert.Nil(t, err)
	assert.Equal(t, 1, len(mapping.Entries))
	assert.Equal(t, -1, mapping.Entries["sdd"].Data)

	// select all devices except those that have a prefix of "s"
	mapping, err = getAvailableDevices(context, "^[^s]", "", true)
	assert.Nil(t, err)
	assert.Equal(t, 3, len(mapping.Entries))
	assert.Equal(t, -1, mapping.Entries["rda"].Data)
	assert.Equal(t, -1, mapping.Entries["rdb"].Data)
	assert.Equal(t, -1, mapping.Entries["nvme01"].Data)
}
