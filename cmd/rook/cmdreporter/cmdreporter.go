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

package cmdreporter

import (
	"fmt"

	"github.com/rook/rook/cmd/rook/rook"
	cmdreporter "github.com/rook/rook/pkg/daemon/cmdreporter"
	"github.com/spf13/cobra"
)

// Cmd is the main command for operator and daemons.
var Cmd = &cobra.Command{
	Use:   "cmd-reporter",
	Short: "Utility to run a given command to completion and store the result in a ConfigMap.",
	Long:  "Utility to run a given command to completion and store the result in a ConfigMap.",
}

var runCmd = &cobra.Command{
	Use:   "run",
	Short: "Run a given command to completion, and store the result in a ConfigMap.",
	Long: `Run a given command to completion, and store the Stdout, Stderr, and return code
results of the command in a ConfigMap. If the ConfigMap already exists, the
Stdout, Stderr, and return code data which may be present in the ConfigMap
will be overwritten.

If cmd-reporter succeeds in running the command to completion, no error is
reported, even if the command's return code is nonzero (failure). Run will
return an error if the command could not be run for any reason or if there was
an error storing the command results into the ConfigMap. An application label
is applied to the ConfigMap, and if the label already exists and has a
different application's name name, this returns an error, as this may indicate
that it is not safe for cmd-reporter to edit the ConfigMap.`,
	Args: cobra.NoArgs,
	RunE: runJob,
}

var copyBinsCmd = &cobra.Command{
	Use:   "copy-binaries",
	Short: "Copy 'rook' and 'tini' binaries from a container to a given directory.",
	Long: `Copy 'rook' and 'tini' binaries from a container to a given directory.
'cmd-reporter run' may often need to be run from a container other than the
container containing the 'rook' binary. Use this command to copy the 'rook' and
required 'tini' binaries from the container containing the 'rook' binary to a
Kubernetes EmptyDir volume mounted at the given directory in a pod's init
container. From the pod's main container, mount the volume which now contains
the 'rook' and 'tini' binaries, and call 'rook cmd-reporter run' from 'tini'
in order to run the desired command from a non-Rook container.`,
	Args: cobra.NoArgs,
	RunE: copyJob,
}

var (
	// run sub-command
	commandString string
	configMapName string
	namespace     string

	// copy-binaries sub-command
	copyToDir string
)

func init() {
	// run sub-command
	runCmd.Flags().StringVar(&commandString, "command", "",
		"The command to run in JSON list syntax. e.g., '[\"command\", \"--flag\", \"value\", \"arg\"]'")
	runCmd.MarkFlagRequired("command")

	runCmd.Flags().StringVar(&configMapName, "config-map-name", "",
		"The name of the ConfigMap into which the result of the command will be stored.")
	runCmd.MarkFlagRequired("config-map-name")

	runCmd.Flags().StringVar(&namespace, "namespace", "", "The namespace in which to create the ConfigMap.")
	runCmd.MarkFlagRequired("namespace")

	// copy-binaries sub-command
	copyBinsCmd.Flags().StringVar(&copyToDir, "copy-to-dir", "",
		"The directory into which 'tini' and 'rook' binaries will be copied.")
	copyBinsCmd.MarkFlagRequired("copy-to-dir")

	// cmd-reporter main command
	Cmd.AddCommand(runCmd, copyBinsCmd)
}

func runJob(cCmd *cobra.Command, cArgs []string) error {
	cmd, args, err := cmdreporter.FlagArgumentToCommand(commandString)
	if err != nil {
		rook.TerminateFatal(fmt.Errorf("failed to parse '--command' argument [%s]. %+v", commandString, err))
	}

	k8s, _, _, err := rook.GetClientset()
	if err != nil {
		rook.TerminateFatal(fmt.Errorf("failed to start command-reporter. failed to init k8s client. %+v", err))
	}

	reporter, err := cmdreporter.New(k8s, cmd, args, configMapName, namespace)
	if err != nil {
		rook.TerminateFatal(fmt.Errorf("cannot start command-reporter. %+v", err))
	}
	return reporter.Run()
}

func copyJob(cCmd *cobra.Command, cArgs []string) error {
	if err := cmdreporter.CopyBinaries(copyToDir); err != nil {
		rook.TerminateFatal(fmt.Errorf("could not copy binaries to %s. %+v", copyToDir, err))
	}
	return nil
}
