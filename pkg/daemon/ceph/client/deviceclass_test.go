/*
Copyright 2021 The Rook Authors. All rights reserved.

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

	"github.com/pkg/errors"
	"github.com/rook/rook/pkg/clusterd"
	exectest "github.com/rook/rook/pkg/util/exec/test"
	"github.com/stretchr/testify/assert"
)

func TestGetDeviceClassOSDs(t *testing.T) {
	executor := &exectest.MockExecutor{}
	executor.MockExecuteCommandWithOutput = func(command string, args ...string) (string, error) {
		logger.Infof("Command: %s %v", command, args)
		if args[1] == "crush" && args[2] == "class" && args[3] == "ls-osd" && args[4] == "ssd" {
			// Mock executor for `ceph osd crush class ls-osd ssd`
			return "[0, 1, 2]", nil
		} else if args[1] == "crush" && args[2] == "class" && args[3] == "ls-osd" && args[4] == "hdd" {
			// Mock executor for `ceph osd crush class ls-osd hdd`
			return "[]", nil
		}
		return "", errors.Errorf("unexpected ceph command '%v'", args)
	}
	osds, err := GetDeviceClassOSDs(&clusterd.Context{Executor: executor}, AdminClusterInfo("mycluster"), "ssd")
	assert.Nil(t, err)
	assert.Equal(t, []int{0, 1, 2}, osds)

	osds, err = GetDeviceClassOSDs(&clusterd.Context{Executor: executor}, AdminClusterInfo("mycluster"), "hdd")
	assert.Nil(t, err)
	assert.Equal(t, []int{}, osds)
}
