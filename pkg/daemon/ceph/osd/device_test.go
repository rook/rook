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
package osd

import (
	"io/ioutil"
	"path"
	"strings"
	"testing"

	"github.com/rook/rook/pkg/clusterd"
	"github.com/rook/rook/pkg/daemon/ceph/client"
	exectest "github.com/rook/rook/pkg/util/exec/test"
	"github.com/stretchr/testify/assert"
)

func TestOSDBootstrap(t *testing.T) {
	configDir := t.TempDir()

	executor := &exectest.MockExecutor{
		MockExecuteCommandWithOutput: func(command string, args ...string) (string, error) {
			return "{\"key\":\"mysecurekey\"}", nil
		},
	}

	context := &clusterd.Context{Executor: executor, ConfigDir: configDir}
	err := createOSDBootstrapKeyring(context, client.AdminTestClusterInfo("mycluster"), configDir)
	assert.Nil(t, err)

	targetPath := path.Join(configDir, bootstrapOsdKeyring)
	contents, err := ioutil.ReadFile(targetPath)
	assert.Nil(t, err)
	assert.NotEqual(t, -1, strings.Index(string(contents), "[client.bootstrap-osd]"))
	assert.NotEqual(t, -1, strings.Index(string(contents), "key = mysecurekey"))
	assert.NotEqual(t, -1, strings.Index(string(contents), "caps mon = \"allow profile bootstrap-osd\""))
}
