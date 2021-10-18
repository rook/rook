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
	"strconv"
	"time"

	"github.com/pkg/errors"
	"github.com/rook/rook/pkg/clusterd"
	"github.com/rook/rook/pkg/util/exec"
)

// RunAllCephCommandsInToolboxPod - when running the e2e tests, all ceph commands need to be run in the toolbox.
// Everywhere else, the ceph tools are assumed to be in the container where we can shell out.
// This is the name of the pod.
var RunAllCephCommandsInToolboxPod string

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
	CrushTool = "crushtool"
	// DefaultPGCount will cause Ceph to use the internal default PG count
	DefaultPGCount = "0"
	// CommandProxyInitContainerName is the name of the init container for proxying ceph command when multus is used
	CommandProxyInitContainerName = "cmd-proxy"
	// ProxyAppLabel is the label used to identify the proxy container
	ProxyAppLabel = "rook-ceph-mgr"
)

// CephConfFilePath returns the location to the cluster's config file in the operator container.
func CephConfFilePath(configDir, clusterName string) string {
	confFile := fmt.Sprintf("%s.config", clusterName)
	return path.Join(configDir, clusterName, confFile)
}

// FinalizeCephCommandArgs builds the command line to be called
func FinalizeCephCommandArgs(command string, clusterInfo *ClusterInfo, args []string, configDir string) (string, []string) {
	// the rbd client tool does not support the '--connect-timeout' option
	// so we only use it for the 'ceph' command
	// Also, there is no point of adding that option to 'crushtool' since that CLI does not connect to anything
	// 'crushtool' is a utility that lets you create, compile, decompile and test CRUSH map files.

	// we could use a slice and iterate over it but since we have only 3 elements
	// I don't think this is worth a loop
	timeout := strconv.Itoa(int(exec.CephCommandsTimeout.Seconds()))
	if command != "rbd" && command != "crushtool" && command != "radosgw-admin" {
		args = append(args, "--connect-timeout="+timeout)
	}

	// If the command should be run inside the toolbox pod, include the kubectl args to call the toolbox
	if RunAllCephCommandsInToolboxPod != "" {
		toolArgs := []string{"exec", "-i", RunAllCephCommandsInToolboxPod, "-n", clusterInfo.Namespace,
			"--", "timeout", timeout, command}
		return Kubectl, append(toolArgs, args...)
	}

	// Append the args to find the config and keyring
	keyringFile := fmt.Sprintf("%s.keyring", clusterInfo.CephCred.Username)
	configArgs := []string{
		fmt.Sprintf("--cluster=%s", clusterInfo.Namespace),
		fmt.Sprintf("--conf=%s", CephConfFilePath(configDir, clusterInfo.Namespace)),
		fmt.Sprintf("--name=%s", clusterInfo.CephCred.Username),
		fmt.Sprintf("--keyring=%s", path.Join(configDir, clusterInfo.Namespace, keyringFile)),
	}
	return command, append(args, configArgs...)
}

type CephToolCommand struct {
	context         *clusterd.Context
	clusterInfo     *ClusterInfo
	tool            string
	args            []string
	timeout         time.Duration
	JsonOutput      bool
	RemoteExecution bool
}

func newCephToolCommand(tool string, context *clusterd.Context, clusterInfo *ClusterInfo, args []string) *CephToolCommand {
	return &CephToolCommand{
		context:     context,
		tool:        tool,
		clusterInfo: clusterInfo,
		args:        args,
		JsonOutput:  true,
	}
}

func NewCephCommand(context *clusterd.Context, clusterInfo *ClusterInfo, args []string) *CephToolCommand {
	return newCephToolCommand(CephTool, context, clusterInfo, args)
}

func NewRBDCommand(context *clusterd.Context, clusterInfo *ClusterInfo, args []string) *CephToolCommand {
	cmd := newCephToolCommand(RBDTool, context, clusterInfo, args)
	cmd.JsonOutput = false

	// When Multus is enabled, the RBD tool should run inside the proxy container
	if clusterInfo.NetworkSpec.IsMultus() {
		cmd.RemoteExecution = true
	}

	return cmd
}

func (c *CephToolCommand) run() ([]byte, error) {
	// Initialize the command and args
	command := c.tool
	args := c.args

	// If this is a remote execution, we don't want to build the full set of args. For instance all
	// these args are not needed since those paths don't exist inside the cmd-proxy container:
	//      --cluster=openshift-storage
	//		--conf=/var/lib/rook/openshift-storage/openshift-storage.config
	//		--name=client.admin
	//		--keyring=/var/lib/rook/openshift-storage/client.admin.keyring
	//
	// The cmd-proxy container will take care of the rest with the help of the env CEPH_ARGS
	if !c.RemoteExecution {
		command, args = FinalizeCephCommandArgs(c.tool, c.clusterInfo, c.args, c.context.ConfigDir)
	}

	if c.JsonOutput {
		args = append(args, "--format", "json")
	} else {
		// the `rbd` tool doesn't use special flag for plain format
		if c.tool != RBDTool {
			args = append(args, "--format", "plain")
		}
	}

	var output, stderr string
	var err error

	// NewRBDCommand does not use the --out-file option so we only check for remote execution here
	// Still forcing the check for the command if the behavior changes in the future
	if command == RBDTool {
		if c.RemoteExecution {
			output, stderr, err = c.context.RemoteExecutor.ExecCommandInContainerWithFullOutputWithTimeout(ProxyAppLabel, CommandProxyInitContainerName, c.clusterInfo.Namespace, append([]string{command}, args...)...)
			if stderr != "" || err != nil {
				err = errors.Errorf("%s. %s", err.Error(), stderr)
			}
		} else if c.timeout == 0 {
			output, err = c.context.Executor.ExecuteCommandWithOutput(command, args...)
		} else {
			output, err = c.context.Executor.ExecuteCommandWithTimeout(c.timeout, command, args...)
		}
	} else if c.timeout == 0 {
		output, err = c.context.Executor.ExecuteCommandWithOutput(command, args...)
	} else {
		output, err = c.context.Executor.ExecuteCommandWithTimeout(c.timeout, command, args...)
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
func ExecuteRBDCommandWithTimeout(context *clusterd.Context, args []string) (string, error) {
	output, err := context.Executor.ExecuteCommandWithTimeout(exec.CephCommandsTimeout, RBDTool, args...)
	return output, err
}

func ExecuteCephCommandWithRetry(
	cmd func() (string, []byte, error),
	getExitCode func(err error) (int, bool),
	retries int,
	retryOnExitCode int,
	waitTime time.Duration,
) ([]byte, error) {
	for i := 0; i < retries; i++ {
		action, data, err := cmd()
		if err != nil {
			exitCode, parsed := getExitCode(err)
			if parsed {
				if exitCode == retryOnExitCode {
					logger.Infof("command failed for %s. trying again...", action)
					time.Sleep(waitTime)
					continue
				}
			}
			return nil, errors.Wrapf(err, "failed to complete command for %s", action)
		}
		if i > 0 {
			logger.Infof("action %s succeeded on attempt %d", action, i)
		}
		return data, nil
	}
	return nil, errors.New("max command retries exceeded")
}
