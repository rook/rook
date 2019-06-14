/*
Copyright 2019 The Rook Authors. All rights reserved.

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

	"github.com/rook/rook/pkg/clusterd"
	exectest "github.com/rook/rook/pkg/util/exec/test"
	"github.com/stretchr/testify/assert"
)

func TestGetCephMonVersionString(t *testing.T) {
	executor := &exectest.MockExecutor{}
	executor.MockExecuteCommandWithOutput = func(debug bool, name string, command string, args ...string) (string, error) {
		assert.Equal(t, "version", args[0])
		assert.Equal(t, 1, len(args))
		return "", nil
	}
	context := &clusterd.Context{Executor: executor}

	_, err := getCephMonVersionString(context)
	assert.Nil(t, err)
}

func TestGetCephMonVersionsString(t *testing.T) {
	executor := &exectest.MockExecutor{}
	executor.MockExecuteCommandWithOutput = func(debug bool, name string, command string, args ...string) (string, error) {
		assert.Equal(t, "versions", args[0])
		assert.Equal(t, 1, len(args))
		return "", nil
	}
	context := &clusterd.Context{Executor: executor}

	_, err := getCephVersionsString(context)
	assert.Nil(t, err)
}

func TestEnableMessenger2(t *testing.T) {
	executor := &exectest.MockExecutor{}
	executor.MockExecuteCommandWithOutput = func(debug bool, name string, command string, args ...string) (string, error) {
		assert.Equal(t, "mon", args[0])
		assert.Equal(t, "enable-msgr2", args[1])
		assert.Equal(t, 2, len(args))
		return "", nil
	}
	context := &clusterd.Context{Executor: executor}

	err := EnableMessenger2(context)
	assert.Nil(t, err)
}

func TestEnableNautilusOSD(t *testing.T) {
	executor := &exectest.MockExecutor{}
	executor.MockExecuteCommandWithOutput = func(debug bool, name string, command string, args ...string) (string, error) {
		assert.Equal(t, "osd", args[0])
		assert.Equal(t, "require-osd-release", args[1])
		assert.Equal(t, "nautilus", args[2])
		assert.Equal(t, 3, len(args))
		return "", nil
	}
	context := &clusterd.Context{Executor: executor}

	err := EnableNautilusOSD(context)
	assert.Nil(t, err)
}
