/*
Copyright 2020 The Rook Authors. All rights reserved.

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
	"path/filepath"
	"testing"

	"github.com/pkg/errors"
	"github.com/rook/rook/pkg/clusterd"
	exectest "github.com/rook/rook/pkg/util/exec/test"
	"github.com/stretchr/testify/assert"
)

func TestBuildNsEnterCLI(t *testing.T) {
	ne := NewNsenter(&clusterd.Context{}, lvmCommandToCheck, []string{"help"})
	args := ne.buildNsEnterCLI(filepath.Join("/sbin/", ne.binary))
	expectedCLI := []string{"--mount=/rootfs/proc/1/ns/mnt", "--", "/sbin/lvm", "help"}

	assert.Equal(t, expectedCLI, args)
}

func TestCheckIfBinaryExistsOnHost(t *testing.T) {
	executor := &exectest.MockExecutor{}
	executor.MockExecuteCommandWithCombinedOutput = func(command string, args ...string) (string, error) {
		logger.Infof("%s %v", command, args)
		if command == "nsenter" && args[0] == "--mount=/rootfs/proc/1/ns/mnt" && args[1] == "--" && args[3] == "help" {
			if args[2] == "/usr/sbin/lvm" || args[2] == "/sbin/lvm" {
				return "success", nil
			}
		}

		return "", errors.Errorf("unknown command %s %s", command, args)
	}

	context := &clusterd.Context{Executor: executor}
	ne := NewNsenter(context, lvmCommandToCheck, []string{"help"})
	err := ne.checkIfBinaryExistsOnHost()
	assert.NoError(t, err)
}
