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

	"github.com/rook/rook/pkg/model"
	"github.com/rook/rook/pkg/rook/test"
	exectest "github.com/rook/rook/pkg/util/exec/test"
)

func TestMountBlock(t *testing.T) {
	c := &test.MockRookRestClient{
		MockGetClientAccessInfo: func() (model.ClientAccessInfo, error) {
			return model.ClientAccessInfo{
				MonAddresses: []string{"10.37.129.214:6790/0"},
				UserName:     "admin",
				SecretKey:    "AQBsCv1X5oD9GhAARHVU9N+kFRWDjyLA1dqzIg==",
			}, nil
		},
	}
	e := &exectest.MockExecutor{
		MockExecuteCommandWithOutput: func(actionName string, command string, args ...string) (string, error) {
			switch {
			case strings.HasPrefix(command, "modinfo"):
				return "single_major:Use a single major number for all rbd devices (default: false) (bool)", nil
			}
			return "", nil
		},
	}

	// set up a mock RBD sys bus file system
	mockRBDSysBusPath, err := ioutil.TempDir("", "TestListBlockImages")
	if err != nil {
		t.Fatalf("failed to create temp rbd sys bus dir: %+v", err)
	}
	defer os.RemoveAll(mockRBDSysBusPath)
	os.Create(filepath.Join(mockRBDSysBusPath, rbdAddSingleMajorNode))
	os.Create(filepath.Join(mockRBDSysBusPath, rbdAddNode))
	createMockRBD(mockRBDSysBusPath, "3", "myimage1", "mypool1")

	// call mountBlock and verify success and output
	out, err := mapBlock("myimage1", "mypool1", "/tmp/mymount1", mockRBDSysBusPath, true, c, e)
	assert.Nil(t, err)
	assert.Equal(t, "succeeded mapping image myimage1 on device /dev/rbd3, formatted, and mounted at /tmp/mymount1", out)

	// verify the correct rbd data was written to the add file
	addFileData, err := ioutil.ReadFile(filepath.Join(mockRBDSysBusPath, rbdAddSingleMajorNode))
	assert.Nil(t, err)
	assert.Equal(t, "10.37.129.214:6790 name=admin,secret=AQBsCv1X5oD9GhAARHVU9N+kFRWDjyLA1dqzIg== mypool1 myimage1", string(addFileData))
}

func TestMountBlockFailure(t *testing.T) {
	c := &test.MockRookRestClient{
		MockGetClientAccessInfo: func() (model.ClientAccessInfo, error) {
			return model.ClientAccessInfo{}, fmt.Errorf("mock failure for GetClientAccessInfo")
		},
	}
	e := &exectest.MockExecutor{}

	// expect mountBlock to fail
	out, err := mapBlock("myimage1", "mypool1", "/tmp/mymount1", "", true, c, e)
	assert.NotNil(t, err)
	assert.Equal(t, "", out)
}
