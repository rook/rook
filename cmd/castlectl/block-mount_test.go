package main

import (
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/quantum/castle/pkg/castlectl/test"
	"github.com/quantum/castle/pkg/model"
	exectest "github.com/quantum/castle/pkg/util/exec/test"
)

func TestMountBlock(t *testing.T) {
	c := &test.MockCastleRestClient{
		MockGetBlockImageMapInfo: func() (model.BlockImageMapInfo, error) {
			return model.BlockImageMapInfo{
				MonAddresses: []string{"10.37.129.214:6790/0"},
				UserName:     "admin",
				SecretKey:    "AQBsCv1X5oD9GhAARHVU9N+kFRWDjyLA1dqzIg==",
			}, nil
		},
	}
	e := &exectest.MockExecutor{
		MockExecuteCommandPipeline: func(actionName string, command string) (string, error) {
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
	out, err := mountBlock("myimage1", "mypool1", "/tmp/mymount1", mockRBDSysBusPath, c, e)
	assert.Nil(t, err)
	assert.Equal(t, "succeeded mapping image myimage1 on device /dev/rbd3 at '/tmp/mymount1'", out)

	// verify the correct rbd data was written to the add file
	addFileData, err := ioutil.ReadFile(filepath.Join(mockRBDSysBusPath, rbdAddSingleMajorNode))
	assert.Nil(t, err)
	assert.Equal(t, "10.37.129.214:6790 name=admin,secret=AQBsCv1X5oD9GhAARHVU9N+kFRWDjyLA1dqzIg== mypool1 myimage1", string(addFileData))
}

func TestMountBlockFailure(t *testing.T) {
	c := &test.MockCastleRestClient{
		MockGetBlockImageMapInfo: func() (model.BlockImageMapInfo, error) {
			return model.BlockImageMapInfo{}, fmt.Errorf("mock failure for GetBlockImageMapInfo")
		},
	}
	e := &exectest.MockExecutor{}

	// expect mountBlock to fail
	out, err := mountBlock("myimage1", "mypool1", "/tmp/mymount1", "", c, e)
	assert.NotNil(t, err)
	assert.Equal(t, "", out)
}
