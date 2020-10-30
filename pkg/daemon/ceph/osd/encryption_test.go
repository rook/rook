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
	"testing"

	"github.com/pkg/errors"
	"github.com/rook/rook/pkg/clusterd"
	exectest "github.com/rook/rook/pkg/util/exec/test"
	"github.com/stretchr/testify/assert"
)

func TestCloseEncryptedDevice(t *testing.T) {
	executor := &exectest.MockExecutor{}
	executor.MockExecuteCommandWithCombinedOutput = func(command string, args ...string) (string, error) {
		logger.Infof("%s %v", command, args)
		if command == "cryptsetup" && args[0] == "--verbose" && args[1] == "luksClose" {
			return "success", nil
		}

		return "", errors.Errorf("unknown command %s %s", command, args)
	}

	context := &clusterd.Context{Executor: executor}
	err := closeEncryptedDevice(context, "/dev/mapper/ceph-43e9efed-0676-4731-b75a-a4c42ece1bb1-xvdbr-block-dmcrypt")
	assert.NoError(t, err)
}

func TestDmsetupVersion(t *testing.T) {
	dmsetupOutput := `
Library version:   1.02.154 (2018-12-07)
Driver version:    4.40.0
`
	executor := &exectest.MockExecutor{}
	executor.MockExecuteCommandWithCombinedOutput = func(command string, args ...string) (string, error) {
		logger.Infof("%s %v", command, args)
		if command == "dmsetup" && args[0] == "version" {
			return dmsetupOutput, nil
		}

		return "", errors.Errorf("unknown command %s %s", command, args)
	}

	context := &clusterd.Context{Executor: executor}
	err := dmsetupVersion(context)
	assert.NoError(t, err)
}
