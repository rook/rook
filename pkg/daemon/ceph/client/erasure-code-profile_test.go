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

	"github.com/pkg/errors"
	"github.com/rook/rook/pkg/daemon/ceph/model"
	exectest "github.com/rook/rook/pkg/util/exec/test"
	"github.com/stretchr/testify/assert"

	"github.com/rook/rook/pkg/clusterd"
)

func TestCreateProfile(t *testing.T) {
	testCreateProfile(t, "", "myroot", "")
}

func TestCreateProfileWithFailureDomain(t *testing.T) {
	testCreateProfile(t, "osd", "", "")
}

func TestCreateProfileWithDeviceClass(t *testing.T) {
	testCreateProfile(t, "osd", "", "hdd")
}

func testCreateProfile(t *testing.T, failureDomain, crushRoot, deviceClass string) {
	cfg := model.ErasureCodedPoolConfig{DataChunkCount: 2, CodingChunkCount: 3, Algorithm: "myalg"}

	executor := &exectest.MockExecutor{}
	context := &clusterd.Context{Executor: executor}
	executor.MockExecuteCommandWithOutputFile = func(debug bool, actionName, command, outputFile string, args ...string) (string, error) {
		logger.Infof("Command: %s %v", command, args)
		if args[1] == "erasure-code-profile" {
			if args[2] == "get" {
				assert.Equal(t, "default", args[3])
				return `{"plugin":"myplugin","technique":"t"}`, nil
			}
			if args[2] == "set" {
				assert.Equal(t, "myapp", args[3])
				assert.Equal(t, fmt.Sprintf("k=%d", cfg.DataChunkCount), args[4])
				assert.Equal(t, fmt.Sprintf("m=%d", cfg.CodingChunkCount), args[5])
				assert.Equal(t, "plugin=myplugin", args[6])
				assert.Equal(t, "technique=t", args[7])
				nextArg := 8
				if failureDomain != "" {
					assert.Equal(t, fmt.Sprintf("crush-failure-domain=%s", failureDomain), args[nextArg])
					nextArg++
				}
				if crushRoot != "" {
					assert.Equal(t, fmt.Sprintf("crush-root=%s", crushRoot), args[nextArg])
					nextArg++
				}
				if deviceClass != "" {
					assert.Equal(t, fmt.Sprintf("crush-device-class=%s", deviceClass), args[nextArg])
					nextArg++
				}
				return "", nil
			}
		}
		return "", errors.Errorf("unexpected ceph command %q", args)
	}

	err := CreateErasureCodeProfile(context, "myns", cfg, "myapp", failureDomain, crushRoot, deviceClass)
	assert.Nil(t, err)
}
