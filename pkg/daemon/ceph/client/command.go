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
	"path"
	"time"

	"github.com/rook/rook/pkg/clusterd"
)

// When running the e2e tests, all ceph commands need to be run in the toolbox.
// Everywhere else, the ceph tools are assumed to be in the container where we can shell out.
var RunAllCephCommandsInToolbox = false

const (
	AdminUsername     = "client.admin"
	CephTool          = "ceph"
	RBDTool           = "rbd"
	Kubectl           = "kubectl"
	CrushTool         = "crushtool"
	cmdExecuteTimeout = 1 * time.Minute
)

func FinalizeCephCommandArgs(command string, args []string, configDir, clusterName string) (string, []string) {
	// If the command should be run inside the toolbox pod, include the kubectl args to call the toolbox
	if RunAllCephCommandsInToolbox {
		toolArgs := []string{"-it", "exec", "rook-ceph-tools", "-n", clusterName, "--", command}
		return Kubectl, append(toolArgs, args...)
	}

	// No need to append the args if it's the default ceph cluster
	if clusterName == "ceph" && configDir == "/etc" {
		return command, args
	}

	// Append the args to find the config and keyring
	confFile := fmt.Sprintf("%s.config", clusterName)
	keyringFile := fmt.Sprintf("%s.keyring", AdminUsername)
	configArgs := []string{
		fmt.Sprintf("--cluster=%s", clusterName),
		fmt.Sprintf("--conf=%s", path.Join(configDir, clusterName, confFile)),
		fmt.Sprintf("--keyring=%s", path.Join(configDir, clusterName, keyringFile)),
	}
	return command, append(args, configArgs...)
}

func ExecuteCephCommandDebugLog(context *clusterd.Context, clusterName string, args []string) ([]byte, error) {
	return executeCephCommandWithOutputFile(context, clusterName, true, args)
}

func ExecuteCephCommand(context *clusterd.Context, clusterName string, args []string) ([]byte, error) {
	return executeCephCommandWithOutputFile(context, clusterName, false, args)
}

func ExecuteCephCommandPlain(context *clusterd.Context, clusterName string, args []string) ([]byte, error) {
	command, args := FinalizeCephCommandArgs(CephTool, args, context.ConfigDir, clusterName)
	args = append(args, "--format", "plain")
	return executeCommandWithOutputFile(context, false, command, args)
}

func ExecuteCephCommandPlainNoOutputFile(context *clusterd.Context, clusterName string, args []string) ([]byte, error) {
	command, args := FinalizeCephCommandArgs(CephTool, args, context.ConfigDir, clusterName)
	args = append(args, "--format", "plain")
	return executeCommand(context, command, args)
}

func executeCephCommandWithOutputFile(context *clusterd.Context, clusterName string, debug bool, args []string) ([]byte, error) {
	command, args := FinalizeCephCommandArgs(CephTool, args, context.ConfigDir, clusterName)
	args = append(args, "--format", "json")
	return executeCommandWithOutputFile(context, debug, command, args)
}

func ExecuteRBDCommand(context *clusterd.Context, clusterName string, args []string) ([]byte, error) {
	command, args := FinalizeCephCommandArgs(RBDTool, args, context.ConfigDir, clusterName)
	args = append(args, "--format", "json")
	return executeCommand(context, command, args)
}

func ExecuteRBDCommandNoFormat(context *clusterd.Context, clusterName string, args []string) ([]byte, error) {
	command, args := FinalizeCephCommandArgs(RBDTool, args, context.ConfigDir, clusterName)
	return executeCommand(context, command, args)
}

func ExecuteRBDCommandWithTimeout(context *clusterd.Context, clusterName string, args []string) (string, error) {
	output, err := context.Executor.ExecuteCommandWithTimeout(false, cmdExecuteTimeout, "", RBDTool, args...)
	return output, err
}

func executeCommand(context *clusterd.Context, command string, args []string) ([]byte, error) {
	output, err := context.Executor.ExecuteCommandWithOutput(false, "", command, args...)
	return []byte(output), err
}

func executeCommandWithOutputFile(context *clusterd.Context, debug bool, command string, args []string) ([]byte, error) {
	if command == Kubectl {
		// Kubectl commands targeting the toolbox container generate a temp file in the wrong place, so we will instead capture the output from stdout for the tests
		return executeCommand(context, command, args)
	}
	output, err := context.Executor.ExecuteCommandWithOutputFile(debug, "", command, "--out-file", args...)
	return []byte(output), err
}
