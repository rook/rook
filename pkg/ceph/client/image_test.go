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

	// rbd tool interprets sizes as MB, so we should reject anything smaller than 1MB
	assertInvalidSize(t, context, uint64(0))

	// call with a valid size, but some other error occurs.  rbd tool returns error information to the output stream,
	// separate from the error object, so verify that information also makes it back to us (because it sure is useful).
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

	// now we'll successfully call with a valid size (1 MB), so set up a mock handler
	createCalled := false
	executor.MockExecuteCommandWithOutput = func(actionName string, command string, args ...string) (string, error) {
		switch {
		case command == "rbd" && args[0] == "create":
			createCalled = true
			assert.Equal(t, "1", args[3])
			return "", nil
		case command == "rbd" && args[0] == "ls" && args[1] == "-l":
			return `[{"image":"image1","size":1048576,"format":2}]`, nil
		}
		return "", fmt.Errorf("unexpected ceph command '%v'", args)
	}

	// create a 1MB image and verify the result
	image, err = CreateImage(context, "foocluster", "image1", "pool1", uint64(1000)) // anything less than 1MB will be rounded up to 1MB
	assert.Nil(t, err)
	assert.NotNil(t, image)
	assert.Equal(t, "image1", image.Name)
	assert.Equal(t, uint64(1048576), image.Size)
	assert.True(t, createCalled)
}

func assertInvalidSize(t *testing.T, context *clusterd.Context, size uint64) {
	image, err := CreateImage(context, "foocluster", "image1", "pool1", size)
	assert.Nil(t, image)
	assert.NotNil(t, err)
	assert.Equal(t, fmt.Sprintf("invalid size: %d", size), err.Error())
}
