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

Some of the code below came from https://github.com/digitalocean/ceph_exporter
which has the same license.
*/
package client

import (
	"fmt"
	"testing"

	"github.com/rook/rook/pkg/model"
	exectest "github.com/rook/rook/pkg/util/exec/test"
	"github.com/stretchr/testify/assert"

	"github.com/rook/rook/pkg/clusterd"
)

func TestCreateProfile(t *testing.T) {
	testCreateProfile(t, "")
}

func TestCreateProfileWithFailureDomain(t *testing.T) {
	testCreateProfile(t, "osd")
}

func testCreateProfile(t *testing.T, failureDomain string) {
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
				if failureDomain != "" {
					assert.Equal(t, fmt.Sprintf("crush-failure-domain=%s", failureDomain), args[8])
				}
				return "", nil
			}
		}
		return "", fmt.Errorf("unexpected ceph command '%v'", args)
	}

	err := CreateErasureCodeProfile(context, "myns", cfg, "myapp", failureDomain)
	assert.Nil(t, err)
}
