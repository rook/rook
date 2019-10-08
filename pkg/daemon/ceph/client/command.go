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

	"github.com/pkg/errors"
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
	CmdExecuteTimeout     = 1 * time.Minute
	cephConnectionTimeout = "15" // in seconds
)

// CephConfFilePath returns the location to the cluster's config file in the operator container.
func CephConfFilePath(configDir, clusterName string) string {
	confFile := fmt.Sprintf("%s.config", clusterName)
	return path.Join(configDir, clusterName, confFile)
}

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
	keyringFile := fmt.Sprintf("%s.keyring", AdminUsername)
	configArgs := []string{
		fmt.Sprintf("--cluster=%s", clusterName),
		fmt.Sprintf("--conf=%s", CephConfFilePath(configDir, clusterName)),
		fmt.Sprintf("--keyring=%s", path.Join(configDir, clusterName, keyringFile)),
	}
	return command, append(args, configArgs...)
}

type CephToolCommand struct {
	context     *clusterd.Context
	tool        string
	clusterName string
	args        []string
	timeout     time.Duration
	Debug       bool
	JsonOutput  bool
	OutputFile  bool
}

func newCephToolCommand(tool string, context *clusterd.Context, clusterName string, args []string, debug bool) *CephToolCommand {
	return &CephToolCommand{
		context:     context,
		tool:        tool,
		clusterName: clusterName,
		args:        args,
		Debug:       debug,
		JsonOutput:  true,
		OutputFile:  true,
	}
}

func NewCephCommand(context *clusterd.Context, clusterName string, args []string) *CephToolCommand {
	return newCephToolCommand(CephTool, context, clusterName, args, false)
}

func NewRBDCommand(context *clusterd.Context, clusterName string, args []string) *CephToolCommand {
	cmd := newCephToolCommand(RBDTool, context, clusterName, args, false)
	cmd.JsonOutput = false
	cmd.OutputFile = false
	return cmd
}

func (c *CephToolCommand) run() ([]byte, error) {
	command, args := FinalizeCephCommandArgs(c.tool, c.args, c.context.ConfigDir, c.clusterName)
	if c.JsonOutput {
		args = append(args, "--format", "json")
	} else {
		// the `rbd` tool doesn't use special flag for plain format
		if c.tool != RBDTool {
			args = append(args, "--format", "plain")
		}
	}

	var output string
	var err error

	if c.OutputFile {
		if command == Kubectl {
			// Kubectl commands targeting the toolbox container generate a temp
			// file in the wrong place, so we will instead capture the output
			// from stdout for the tests
			if c.timeout == 0 {
				output, err = c.context.Executor.ExecuteCommandWithOutput(c.Debug, "", command, args...)
			} else {
				output, err = c.context.Executor.ExecuteCommandWithTimeout(c.Debug, c.timeout, "", command, args...)
			}
		} else {
			if c.timeout == 0 {
				output, err = c.context.Executor.ExecuteCommandWithOutputFile(c.Debug, "", command, "--out-file", args...)
			} else {
				output, err = c.context.Executor.ExecuteCommandWithOutputFileTimeout(c.Debug, c.timeout, "", command, "--out-file", args...)
			}
		}
	} else {
		if c.timeout == 0 {
			output, err = c.context.Executor.ExecuteCommandWithOutput(c.Debug, "", command, args...)
		} else {
			output, err = c.context.Executor.ExecuteCommandWithTimeout(c.Debug, c.timeout, "", command, args...)
		}
	}

	return []byte(output), err
}

func (c *CephToolCommand) Run() ([]byte, error) {
	c.timeout = 0
	return c.run()
}

func (c *CephToolCommand) RunWithTimeout(timeout time.Duration) ([]byte, error) {
	c.timeout = timeout
	return c.run()
}

// ExecuteRBDCommandWithTimeout executes the 'rbd' command with a timeout of 1
// minute. This method is left as a special case in which the caller has fully
// configured its arguments. It is future work to integrate this case into the
// generalization.
func ExecuteRBDCommandWithTimeout(context *clusterd.Context, clusterName string, args []string) (string, error) {
	output, err := context.Executor.ExecuteCommandWithTimeout(false, CmdExecuteTimeout, "", RBDTool, args...)
	return output, err
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
			return nil, errors.Wrapf(err, "failed to complete command")
		}
		if i > 0 {
			logger.Infof("command succeeded on attempt %d", i)
		}
		return data, nil
	}
	return nil, errors.New("max command retries exceeded")
}
