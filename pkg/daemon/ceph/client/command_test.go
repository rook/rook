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

	"github.com/stretchr/testify/assert"
)

func TestFinalizeCephCommandArgs(t *testing.T) {
	RunAllCephCommandsInToolbox = false
	clusterName := "rook"
	configDir := "/var/lib/rook/rook-ceph"
	expectedCommand := "ceph"
	args := []string{"quorum_status"}
	expectedArgs := []string{
		"quorum_status",
		"--connect-timeout=15",
		"--cluster=rook",
		"--conf=/var/lib/rook/rook-ceph/rook/rook.config",
		"--keyring=/var/lib/rook/rook-ceph/rook/client.admin.keyring",
	}

	cmd, args := FinalizeCephCommandArgs(expectedCommand, args, configDir, clusterName)
	assert.Exactly(t, expectedCommand, cmd)
	assert.Exactly(t, expectedArgs, args)
}

func TestFinalizeRadosGWAdminCommandArgs(t *testing.T) {
	RunAllCephCommandsInToolbox = false
	clusterName := "rook"
	configDir := "/var/lib/rook/rook-ceph"
	expectedCommand := "radosgw-admin"
	args := []string{
		"realm",
		"create",
		"--default",
		"--rgw-realm=default-rook",
		"--rgw-zonegroup=default-rook",
	}

	expectedArgs := []string{
		"realm",
		"create",
		"--default",
		"--rgw-realm=default-rook",
		"--rgw-zonegroup=default-rook",
		"--cluster=rook",
		"--conf=/var/lib/rook/rook-ceph/rook/rook.config",
		"--keyring=/var/lib/rook/rook-ceph/rook/client.admin.keyring",
	}

	cmd, args := FinalizeCephCommandArgs(expectedCommand, args, configDir, clusterName)
	assert.Exactly(t, expectedCommand, cmd)
	assert.Exactly(t, expectedArgs, args)
}

func TestFinalizeCephCommandArgsToolBox(t *testing.T) {
	RunAllCephCommandsInToolbox = true
	clusterName := "rook"
	configDir := "/var/lib/rook/rook-ceph"
	expectedCommand := "ceph"
	args := []string{"health"}
	expectedArgs := []string{
		"-it",
		"exec",
		"rook-ceph-tools",
		"-n",
		"rook",
		"--",
		"ceph",
		"health",
		"--connect-timeout=15",
	}

	cmd, args := FinalizeCephCommandArgs(expectedCommand, args, configDir, clusterName)
	assert.Exactly(t, "kubectl", cmd)
	assert.Exactly(t, expectedArgs, args)
}

func TestFinalizeCephCommandArgsClusterDefaultName(t *testing.T) {
	RunAllCephCommandsInToolbox = false
	clusterName := "ceph"
	configDir := "/etc"
	expectedCommand := "ceph"
	args := []string{"quorum_status"}
	expectedArgs := []string{
		"quorum_status",
		"--connect-timeout=15",
	}

	cmd, args := FinalizeCephCommandArgs(expectedCommand, args, configDir, clusterName)
	assert.Exactly(t, expectedCommand, cmd)
	assert.Exactly(t, expectedArgs, args)
}
