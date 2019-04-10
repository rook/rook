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

// RunAllCephCommandsInToolbox - when running the e2e tests, all ceph commands need to be run in the toolbox.
// Everywhere else, the ceph tools are assumed to be in the container where we can shell out.
var RunAllCephCommandsInToolbox = false

const (
	// AdminUsername is the name of the admin user
	AdminUsername = "client.admin"
	// CephTool is the name of the CLI tool for 'ceph'
	CephTool = "ceph"
	// RBDTool is the name of the CLI tool for 'rbd'
	RBDTool = "rbd"
	// Kubectl is the name of the CLI tool for 'kubectl'
	Kubectl = "kubectl"
	// CrushTool is the name of the CLI tool for 'crushtool'
	CrushTool             = "crushtool"
	cmdExecuteTimeout     = 1 * time.Minute
	cephConnectionTimeout = "15" // in seconds
)

// FinalizeCephCommandArgs builds the command line to be called
func FinalizeCephCommandArgs(command string, args []string, configDir, clusterName string) (string, []string) {
	// the rbd client tool does not support the '--connect-timeout' option
	// so we only use it for the 'ceph' command
	// Also, there is no point of adding that option to 'crushtool' since that CLI does not connect to anything
	// 'crushtool' is a utility that lets you create, compile, decompile and test CRUSH map files.

	// we could use a slice and iterate over it but since we have only 3 elements
	// I don't think this is worth a loop
	if command != "rbd" && command != "crushtool" && command != "radosgw-admin" {
		args = append(args, "--connect-timeout="+cephConnectionTimeout)
	}

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

// ExecuteCephCommandDebugLog executes the 'ceph' command with 'debug' logs instead of 'info' logs
func ExecuteCephCommandDebugLog(context *clusterd.Context, clusterName string, args []string) ([]byte, error) {
	return executeCephCommandWithOutputFile(context, clusterName, true, args)
}

// ExecuteCephCommand executes the 'ceph' command
func ExecuteCephCommand(context *clusterd.Context, clusterName string, args []string) ([]byte, error) {
	return executeCephCommandWithOutputFile(context, clusterName, false, args)
}

// ExecuteCephCommandDebug executes the 'ceph' command with debug output
func ExecuteCephCommandDebug(context *clusterd.Context, clusterName string, debug bool, args []string) ([]byte, error) {
	return executeCephCommandWithOutputFile(context, clusterName, debug, args)
}

// ExecuteCephCommandPlain executes the 'ceph' command and returns stdout in PLAIN format instead of JSON
func ExecuteCephCommandPlain(context *clusterd.Context, clusterName string, args []string) ([]byte, error) {
	command, args := FinalizeCephCommandArgs(CephTool, args, context.ConfigDir, clusterName)
	args = append(args, "--format", "plain")
	return executeCommandWithOutputFile(context, false, command, args)
}

// ExecuteCephCommandPlainNoOutputFile executes the 'ceph' command and returns stdout in PLAIN format instead of JSON
// with no output file, suppresses '--out-file' option
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

// ExecuteRBDCommand executes the 'rbd' command
func ExecuteRBDCommand(context *clusterd.Context, clusterName string, args []string) ([]byte, error) {
	command, args := FinalizeCephCommandArgs(RBDTool, args, context.ConfigDir, clusterName)
	args = append(args, "--format", "json")
	return executeCommand(context, command, args)
}

// ExecuteRBDCommandNoFormat executes the 'rbd' command and returns stdout in PLAIN format
func ExecuteRBDCommandNoFormat(context *clusterd.Context, clusterName string, args []string) ([]byte, error) {
	command, args := FinalizeCephCommandArgs(RBDTool, args, context.ConfigDir, clusterName)
	return executeCommand(context, command, args)
}

// ExecuteRBDCommandWithTimeout executes the 'rbd' command with a timeout of 1 minute
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

func ExecuteCephCommandWithRetry(
	cmd func() ([]byte, error),
	getExitCode func(err error) (int, bool),
	retries int,
	retryOnExitCode int,
	waitTime time.Duration,
) ([]byte, error) {
	for i := 0; i < retries; i++ {
		data, err := cmd()
		if err != nil {
			exitCode, parsed := getExitCode(err)
			if parsed {
				if exitCode == retryOnExitCode {
					logger.Infof("command failed. trying again...")
					time.Sleep(waitTime)
					continue
				}
			}
			return nil, fmt.Errorf("failed to complete command %+v", err)
		}
		if i > 0 {
			logger.Infof("command succeeded on attempt %d", i)
		}
		return data, nil
	}
	return nil, fmt.Errorf("max command retries exceeded")
}
