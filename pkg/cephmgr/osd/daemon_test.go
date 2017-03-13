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

	"os"

	"github.com/coreos/pkg/capnslog"
	"github.com/rook/rook/pkg/clusterd"
	"github.com/rook/rook/pkg/clusterd/inventory"
	exectest "github.com/rook/rook/pkg/util/exec/test"
	"github.com/rook/rook/pkg/util/proc"
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

func TestAvailableDevices(t *testing.T) {
	executor := &exectest.MockExecutor{}
	// set up a mock function to return "rook owned" partitions on the device and it does not have a filesystem
	executor.MockExecuteCommandWithOutput = func(name string, command string, args ...string) (string, error) {
		logger.Infof("OUTPUT for %s. %s %+v", name, command, args)

		if command == "lsblk" {
			if strings.Index(name, "sdb") != -1 {
				// /dev/sdb has a partition
				return `NAME="sdb" SIZE="65" TYPE="disk" PKNAME="" PARTLABEL=""
NAME="sdb1" SIZE="30" TYPE="part" PKNAME="sdb" PARTLABEL="MY-PART"`, nil
			}
			return "", nil
		} else if command == "df" {
			if strings.Index(name, "sdc") != -1 {
				// /dev/sdc has a file system
				return "/dev/sdc ext4", nil
			}
			return "", nil
		}

		return "", fmt.Errorf("unknown command %s", command)
	}

	context := &clusterd.DaemonContext{ProcMan: proc.New(executor), Executor: executor}
	devices := []*inventory.LocalDisk{
		&inventory.LocalDisk{Name: "sda"},
		&inventory.LocalDisk{Name: "sdb"},
		&inventory.LocalDisk{Name: "sdc"},
		&inventory.LocalDisk{Name: "sdd"},
		&inventory.LocalDisk{Name: "rda"},
		&inventory.LocalDisk{Name: "rdb"},
	}
	// select all devices
	mapping, err := getAvailableDevices(context, devices, "all")
	assert.Nil(t, err)
	assert.Equal(t, 4, len(mapping.Entries))
	assert.Equal(t, -1, mapping.Entries["sda"].Data)
	assert.Equal(t, -1, mapping.Entries["sdd"].Data)
	assert.Equal(t, -1, mapping.Entries["rda"].Data)
	assert.Equal(t, -1, mapping.Entries["rdb"].Data)

	// select no devices
	mapping, err = getAvailableDevices(context, devices, "")
	assert.Nil(t, err)
	assert.Equal(t, 0, len(mapping.Entries))

	// select the sda devices
	mapping, err = getAvailableDevices(context, devices, "^sd.$")
	assert.Nil(t, err)
	assert.Equal(t, 2, len(mapping.Entries))
	assert.Equal(t, -1, mapping.Entries["sda"].Data)
	assert.Equal(t, -1, mapping.Entries["sdd"].Data)

	// select an exact device
	mapping, err = getAvailableDevices(context, devices, "sdd")
	assert.Nil(t, err)
	assert.Equal(t, 1, len(mapping.Entries))
	assert.Equal(t, -1, mapping.Entries["sdd"].Data)

	// select all devices except those that have a prefix of "s"
	mapping, err = getAvailableDevices(context, devices, "^[^s]")
	assert.Nil(t, err)
	assert.Equal(t, 2, len(mapping.Entries))
	assert.Equal(t, -1, mapping.Entries["rda"].Data)
	assert.Equal(t, -1, mapping.Entries["rdb"].Data)
}
