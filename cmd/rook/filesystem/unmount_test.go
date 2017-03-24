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
package filesystem

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"

	exectest "github.com/rook/rook/pkg/util/exec/test"
)

func TestUnmountFilesystem(t *testing.T) {
	e := &exectest.MockExecutor{
		MockExecuteCommand: func(actionName string, command string, arg ...string) error {
			assert.Equal(t, "umount", command)
			expectedArgs := []string{"/tmp/myfs1mount"}
			assert.Equal(t, expectedArgs, arg)
			return nil
		},
	}

	out, err := unmountFilesystem("/tmp/myfs1mount", e)
	assert.Nil(t, err)
	assert.Equal(t, "succeeded unmounting shared filesystem from '/tmp/myfs1mount'", out)
}

func TestUnmountFilesystemError(t *testing.T) {
	e := &exectest.MockExecutor{
		MockExecuteCommand: func(actionName string, command string, arg ...string) error {
			return fmt.Errorf("mock unmount failure")
		},
	}

	out, err := unmountFilesystem("/tmp/myfs1mount", e)
	assert.NotNil(t, err)
	assert.Equal(t, "", out)
}
