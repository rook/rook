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
package block

import (
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"

	exectest "github.com/rook/rook/pkg/util/exec/test"
)

func TestUnmountBlock(t *testing.T) {
	e := &exectest.MockExecutor{
		MockExecuteCommandWithOutput: func(actionName string, command string, args ...string) (string, error) {
			switch {
			case strings.HasPrefix(command, "modinfo"):
				return "single_major:Use a single major number for all rbd devices (default: false) (bool)", nil
			case strings.HasPrefix(actionName, "get device from mount point"):
				return "/dev/rbd4 on /tmp/mymount1 ", nil
			}
			return "", nil
		},
	}

	// set up a mock RBD sys bus file system for rbd4 with myimage1 and mypool1
	mockRBDSysBusPath, err := ioutil.TempDir("", "TestListBlockImages")
	if err != nil {
		t.Fatalf("failed to create temp rbd sys bus dir: %+v", err)
	}
	defer os.RemoveAll(mockRBDSysBusPath)
	os.Create(filepath.Join(mockRBDSysBusPath, rbdRemoveSingleMajorNode))
	os.Create(filepath.Join(mockRBDSysBusPath, rbdRemoveNode))
	createMockRBD(mockRBDSysBusPath, "4", "myimage1", "mypool1")

	// call unmountBlock and verify success and output
	out, err := unmapBlock("", "/tmp/mymount1", mockRBDSysBusPath, e)
	assert.Nil(t, err)
	assert.Equal(t, "succeeded removing rbd device /dev/rbd4 from '/tmp/mymount1'", out)

	// verify the correct rbd data was written to the remove file
	removeFileData, err := ioutil.ReadFile(filepath.Join(mockRBDSysBusPath, rbdRemoveSingleMajorNode))
	assert.Nil(t, err)
	assert.Equal(t, "4", string(removeFileData))
}

func TestUnmountBlockFailure(t *testing.T) {
	e := &exectest.MockExecutor{
		MockExecuteCommandWithOutput: func(actionName string, command string, args ...string) (string, error) {
			switch {
			case strings.HasPrefix(command, "modinfo"):
				return "single_major:Use a single major number for all rbd devices (default: false) (bool)", nil
			case strings.HasPrefix(actionName, "get device from mount point"):
				return "", fmt.Errorf("mock failure for get device from mount point")
			}
			return "", nil
		},
	}

	// expect unmountBlock to fail
	out, err := unmapBlock("", "/tmp/mymount1", "", e)
	assert.NotNil(t, err)
	assert.Equal(t, "", out)
}

func TestUnmountBlockRequiresDeviceOrPath(t *testing.T) {
	out, err := unmapBlock("", "", "", nil)
	assert.NotNil(t, err)
	assert.Equal(t, "", out)
}
