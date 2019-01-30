/*
Copyright 2018 The Rook Authors. All rights reserved.

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

	"github.com/stretchr/testify/assert"
)

func TestCephArgs(t *testing.T) {
	// cluster a under /etc
	args := []string{}
	command, args := FinalizeCephCommandArgs(CephTool, args, "/etc", "a")
	assert.Equal(t, CephTool, command)
	assert.Equal(t, 4, len(args))
	assert.Equal(t, 4, len(args))
	assert.Equal(t, "--connect-timeout=15", args[0])
	assert.Equal(t, "--cluster=a", args[1])
	assert.Equal(t, "--conf=/etc/a/a.config", args[2])
	assert.Equal(t, "--keyring=/etc/a/client.admin.keyring", args[3])

	RunAllCephCommandsInToolbox = true
	args = []string{}
	command, args = FinalizeCephCommandArgs(CephTool, args, "/etc", "a")
	assert.Equal(t, Kubectl, command)
	assert.Equal(t, 8, len(args), fmt.Sprintf("%+v", args))
	assert.Equal(t, "-it", args[0])
	assert.Equal(t, "exec", args[1])
	assert.Equal(t, "rook-ceph-tools", args[2])
	assert.Equal(t, "-n", args[3])
	assert.Equal(t, "a", args[4])
	assert.Equal(t, "--", args[5])
	assert.Equal(t, CephTool, args[6])
	assert.Equal(t, "--connect-timeout=15", args[7])
	RunAllCephCommandsInToolbox = false

	// cluster under /var/lib/rook
	args = []string{"myarg"}
	command, args = FinalizeCephCommandArgs(RBDTool, args, "/var/lib/rook", "rook")
	assert.Equal(t, RBDTool, command)
	assert.Equal(t, 4, len(args))
	assert.Equal(t, "myarg", args[0])
	assert.Equal(t, "--cluster=rook", args[1])
	assert.Equal(t, "--conf=/var/lib/rook/rook/rook.config", args[2])
	assert.Equal(t, "--keyring=/var/lib/rook/rook/client.admin.keyring", args[3])

	// the default ceph cluster will not need the config args
	args = []string{"myarg"}
	command, args = FinalizeCephCommandArgs(CephTool, args, "/etc", "ceph")
	assert.Equal(t, CephTool, command)
	assert.Equal(t, 2, len(args))
	assert.Equal(t, "myarg", args[0])
}
