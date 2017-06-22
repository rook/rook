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

	"github.com/rook/rook/pkg/model"
	"github.com/rook/rook/pkg/rook/test"
	exectest "github.com/rook/rook/pkg/util/exec/test"
)

func TestMountFilesystem(t *testing.T) {
	c := &test.MockRookRestClient{
		MockGetClientAccessInfo: func() (model.ClientAccessInfo, error) {
			return model.ClientAccessInfo{
				MonAddresses: []string{"10.37.129.214:6790/0"},
				UserName:     "admin",
				SecretKey:    "AQBsCv1X5oD9GhAARHVU9N+kFRWDjyLA1dqzIg==",
			}, nil
		},
	}
	e := &exectest.MockExecutor{
		MockExecuteCommand: func(actionName string, command string, arg ...string) error {
			assert.Equal(t, "mount", command)
			expectedArgs := []string{"-t", "ceph", "-o", "name=admin,secret=AQBsCv1X5oD9GhAARHVU9N+kFRWDjyLA1dqzIg==",
				"10.37.129.214:6790:/", "/tmp/myfs1mount"}
			assert.Equal(t, expectedArgs, arg)
			return nil
		},
	}

	out, err := mountFilesystem("myfs1", "/tmp/myfs1mount", c, e)
	assert.Nil(t, err)
	assert.Equal(t, "succeeded mounting shared filesystem myfs1 at '/tmp/myfs1mount'", out)
}

func TestMountFilesystemError(t *testing.T) {
	c := &test.MockRookRestClient{
		MockGetClientAccessInfo: func() (model.ClientAccessInfo, error) {
			return model.ClientAccessInfo{}, fmt.Errorf("mock get client access info failed")
		},
	}
	e := &exectest.MockExecutor{}

	out, err := mountFilesystem("myfs1", "/tmp/myfs1mount", c, e)
	assert.NotNil(t, err)
	assert.Equal(t, "", out)
}
