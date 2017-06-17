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
package client

import (
	"fmt"
	"testing"

	"strings"

	"github.com/rook/rook/pkg/clusterd"
	exectest "github.com/rook/rook/pkg/util/exec/test"
	"github.com/stretchr/testify/assert"
)

func TestCreateImage(t *testing.T) {
	executor := &exectest.MockExecutor{}
	context := &clusterd.Context{Executor: executor}

	// mock an error during the create image call.  rbd tool returns error information to the output stream,
	// separate from the error object, so verify that information also makes it back to us (because it is useful).
	executor.MockExecuteCommandWithOutput = func(actionName string, command string, args ...string) (string, error) {
		switch {
		case command == "rbd" && args[0] == "create":
			return "mocked detailed ceph error output stream", fmt.Errorf("some mocked error")
		}
		return "", fmt.Errorf("unexpected ceph command '%v'", args)
	}
	image, err := CreateImage(context, "foocluster", "image1", "pool1", uint64(1048576)) // 1MB
	assert.NotNil(t, err)
	assert.True(t, strings.Contains(err.Error(), "mocked detailed ceph error output stream"))

	// rbd tool interprets sizes as MB, so anything smaller than that should get rounded up to the minimum
	// (except for 0, that's OK)
	createCalled := false
	expectedSizeArg := ""
	executor.MockExecuteCommandWithOutput = func(actionName string, command string, args ...string) (string, error) {
		switch {
		case command == "rbd" && args[0] == "create":
			createCalled = true
			assert.Equal(t, expectedSizeArg, args[3])
			return "", nil
		case command == "rbd" && args[0] == "ls" && args[1] == "-l":
			return `[{"image":"image1","size":1048576,"format":2}]`, nil
		}
		return "", fmt.Errorf("unexpected ceph command '%v'", args)
	}

	// 0 byte --> 0 MB
	expectedSizeArg = "0"
	image, err = CreateImage(context, "foocluster", "image1", "pool1", uint64(0))
	assert.Nil(t, err)
	assert.NotNil(t, image)
	assert.True(t, createCalled)
	createCalled = false

	// 1 byte --> 1 MB
	expectedSizeArg = "1"
	image, err = CreateImage(context, "foocluster", "image1", "pool1", uint64(1))
	assert.Nil(t, err)
	assert.NotNil(t, image)
	assert.True(t, createCalled)
	createCalled = false

	// (1 MB - 1 byte) --> 1 MB
	expectedSizeArg = "1"
	image, err = CreateImage(context, "foocluster", "image1", "pool1", uint64(1048575))
	assert.Nil(t, err)
	assert.NotNil(t, image)
	assert.True(t, createCalled)
	createCalled = false

	// 1 MB
	expectedSizeArg = "1"
	image, err = CreateImage(context, "foocluster", "image1", "pool1", uint64(1048576))
	assert.Nil(t, err)
	assert.NotNil(t, image)
	assert.True(t, createCalled)
	createCalled = false
}
