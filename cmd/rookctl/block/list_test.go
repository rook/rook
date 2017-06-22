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
	"runtime"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/rook/rook/pkg/model"
	"github.com/rook/rook/pkg/rook/test"
	exectest "github.com/rook/rook/pkg/util/exec/test"
)

func TestListBlockImages(t *testing.T) {
	c := &test.MockRookRestClient{
		MockGetBlockImages: func() ([]model.BlockImage, error) {
			return []model.BlockImage{
				{Name: "myimage1", PoolName: "mypool1", Size: 1024},
			}, nil
		},
	}
	e := &exectest.MockExecutor{
		MockExecuteCommandWithOutput: func(actionName string, command string, args ...string) (string, error) {
			if strings.Contains(command, "mount") {
				return "/dev/rbd5 on /tmp/mymount1", nil
			}
			return "", nil
		},
	}

	// set up a mock RBD sys bus file system that has rbd5 for myimage1 and mypool1
	mockRBDSysBusPath, err := ioutil.TempDir("", "TestListBlockImages")
	if err != nil {
		t.Fatalf("failed to create temp rbd sys bus dir: %+v", err)
	}
	defer os.RemoveAll(mockRBDSysBusPath)
	createMockRBD(mockRBDSysBusPath, "5", "myimage1", "mypool1")

	out, err := listBlocks(mockRBDSysBusPath, c, e)
	assert.Nil(t, err)

	expectedOut := "NAME       POOL      SIZE       DEVICE    MOUNT\n"
	if runtime.GOOS == "linux" {
		expectedOut += "myimage1   mypool1   1.00 KiB   rbd5      /tmp/mymount1\n"
	} else {
		expectedOut += "myimage1   mypool1   1.00 KiB             \n"
	}
	assert.Equal(t, expectedOut, out)
}

func TestListBlockImagesFailure(t *testing.T) {
	c := &test.MockRookRestClient{
		MockGetBlockImages: func() ([]model.BlockImage, error) {
			return nil, fmt.Errorf("mock failure to get block images")
		},
	}
	e := &exectest.MockExecutor{}

	out, err := listBlocks("", c, e)
	assert.NotNil(t, err)
	assert.Equal(t, "", out)
}

func TestListBlockImagesZeroImages(t *testing.T) {
	c := &test.MockRookRestClient{
		MockGetBlockImages: func() ([]model.BlockImage, error) {
			return []model.BlockImage{}, nil
		},
	}
	e := &exectest.MockExecutor{}

	out, err := listBlocks("", c, e)
	assert.Nil(t, err)
	assert.Equal(t, "", out)
}

func createMockRBD(mockRBDSysBusPath, deviceID, imageName, poolName string) {
	dev0Path := filepath.Join(mockRBDSysBusPath, "devices", deviceID)
	os.MkdirAll(dev0Path, 0777)
	ioutil.WriteFile(filepath.Join(dev0Path, "name"), []byte(imageName), 0777)
	ioutil.WriteFile(filepath.Join(dev0Path, "pool"), []byte(poolName), 0777)
}
