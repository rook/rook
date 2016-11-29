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
package sys

import (
	"strings"
	"testing"

	exectest "github.com/rook/rook/pkg/util/exec/test"
	"github.com/stretchr/testify/assert"
)

func TestFindUUID(t *testing.T) {
	output := `Disk /dev/sdb: 10485760 sectors, 5.0 GiB
Logical sector size: 512 bytes
Disk identifier (GUID): 31273B25-7B2E-4D31-BAC9-EE77E62EAC71
Partition table holds up to 128 entries
First usable sector is 34, last usable sector is 10485726
Partitions will be aligned on 2048-sector boundaries
Total free space is 20971453 sectors (10.0 GiB)
`
	uuid, err := parseUUID("sdb", output)
	assert.Nil(t, err)
	assert.Equal(t, "31273b25-7b2e-4d31-bac9-ee77e62eac71", uuid)
}

func TestParseFileSystem(t *testing.T) {
	output := `Filesystem     Type
devtmpfs       devtmpfs
/dev/sda9      ext4
/dev/sda3      ext4
/dev/sda1      vfat
tmpfs          tmpfs
tmpfs          tmpfs
/dev/sda6      ext4
sdc            tmpfs`

	result := parseDFOutput("sda", output)
	assert.Equal(t, "ext4,ext4,vfat,ext4", result)

	result = parseDFOutput("sdb", output)
	assert.Equal(t, "", result)

	result = parseDFOutput("sdc", output)
	assert.Equal(t, "", result)
}

func TestGetDeviceFromMountPoint(t *testing.T) {
	const device = "/dev/rbd3"
	e := &exectest.MockExecutor{
		MockExecuteCommandPipeline: func(actionName string, command string) (string, error) {
			switch {
			case strings.HasPrefix(actionName, "get device from mount point"):
				// verify that the mount path being searched for has been cleaned
				assert.Contains(t, command, " /tmp/mymountpath ")
				return device, nil
			}
			return "", nil
		},
	}

	// no trailing slash should work OK
	d, err := GetDeviceFromMountPoint("/tmp/mymountpath", e)
	assert.Nil(t, err)
	assert.Equal(t, device, d)

	// a trailing slash should be cleaned and work OK
	d, err = GetDeviceFromMountPoint("/tmp/mymountpath/", e)
	assert.Nil(t, err)
	assert.Equal(t, device, d)

	// a parent directory '..' in the middle of the path should work OK
	d, err = GetDeviceFromMountPoint("/tmp/somedir/../mymountpath/", e)
	assert.Nil(t, err)
	assert.Equal(t, device, d)
}

func TestMountDeviceWithOptions(t *testing.T) {
	testCount := 0
	e := &exectest.MockExecutor{
		MockExecuteCommand: func(actionName string, command string, arg ...string) error {
			switch testCount {
			case 0:
				assert.Equal(t, []string{"mount", "/dev/abc1", "/tmp/mount1"}, arg)
			case 1:
				assert.Equal(t, []string{"mount", "-o", "foo=bar,baz=biz", "/dev/abc1", "/tmp/mount1"}, arg)
			case 2:
				assert.Equal(t, []string{"mount", "-t", "myfstype", "/dev/abc1", "/tmp/mount1"}, arg)
			case 3:
				assert.Equal(t, []string{"mount", "-t", "myfstype", "-o", "foo=bar,baz=biz", "/dev/abc1", "/tmp/mount1"}, arg)
			}

			testCount++
			return nil
		},
	}

	// no fstype or options
	MountDeviceWithOptions("/dev/abc1", "/tmp/mount1", "", "", e)

	// options specified
	MountDeviceWithOptions("/dev/abc1", "/tmp/mount1", "", "foo=bar,baz=biz", e)

	// fstype specified
	MountDeviceWithOptions("/dev/abc1", "/tmp/mount1", "myfstype", "", e)

	// both fstype and options specified
	MountDeviceWithOptions("/dev/abc1", "/tmp/mount1", "myfstype", "foo=bar,baz=biz", e)
}
