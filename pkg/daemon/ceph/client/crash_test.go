/*
Copyright 2021 The Rook Authors. All rights reserved.

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

	"github.com/pkg/errors"
	"github.com/rook/rook/pkg/clusterd"
	exectest "github.com/rook/rook/pkg/util/exec/test"
	"github.com/stretchr/testify/assert"
)

var fakecrash = `[
    		{
				"crash_id": "2020-11-09_13:58:08.230130Z_ca918f58-c078-444d-a91a-bd972c14c155",
				"timestamp": "2020-11-09 13:58:08.230130Z",
				"process_name": "ceph-osd",
				"entity_name": "osd.0"
		    }
	    ]`

func TestCephCrash(t *testing.T) {
	executor := &exectest.MockExecutor{}
	context := &clusterd.Context{Executor: executor}
	executor.MockExecuteCommandWithOutput = func(command string, args ...string) (string, error) {
		logger.Infof("ExecuteCommandWithOutputFile: %s %v", command, args)
		if args[0] == "crash" && args[1] == "ls" {
			return fakecrash, nil
		}
		return "", errors.Errorf("unexpected ceph command %q", args)
	}
	crash, err := GetCrashList(context, AdminTestClusterInfo("mycluster"))
	assert.NoError(t, err)
	assert.Equal(t, 1, len(crash))
}
