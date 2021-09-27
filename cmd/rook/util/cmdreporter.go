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

package util

import (
	"context"
	"fmt"
	"os/signal"

	"github.com/rook/rook/pkg/daemon/util"
	operator "github.com/rook/rook/pkg/operator/ceph"

	"github.com/rook/rook/cmd/rook/rook"
	"github.com/spf13/cobra"
)

// CmdReporterCmd defines a top-level utility command which runs a given command and stores the
// results in a ConfigMap. Operators are advised to use operator/k8sutil.CmdReporter, which wraps
// this functionality neatly rather than calling this with a custom setup.
var CmdReporterCmd = &cobra.Command{
	Use:   "cmd-reporter",
	Short: "Run a given command to completion, and store the result in a ConfigMap.",
	Long: `Run a given command to completion, and store the Stdout, Stderr, and return code
results of the command in a ConfigMap. If the ConfigMap already exists, the
Stdout, Stderr, and return code data which may be present in the ConfigMap
will be overwritten.

If cmd-reporter succeeds in running the command to completion, no error is
reported, even if the command's return code is nonzero (failure). Run will
terminate if the command could not be run for any reason or if there was an
error storing the command results into the ConfigMap. An application label
is applied to the ConfigMap. Run will also terminate if the label already
exists and has a different application's name name; this may indicate that
it is not safe for cmd-reporter to edit the ConfigMap.`,
	Args: cobra.NoArgs,
	Run:  runCmdReporter,
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
	// cmd-reporter
	CmdReporterCmd.Flags().StringVar(&commandString, "command", "",
		"The command to run in JSON list syntax. e.g., '[\"command\", \"--flag\", \"value\", \"arg\"]'")
	if err := CmdReporterCmd.MarkFlagRequired("command"); err != nil {
		panic(err)
	}

	CmdReporterCmd.Flags().StringVar(&configMapName, "config-map-name", "",
		"The name of the ConfigMap into which the result of the command will be stored.")
	if err := CmdReporterCmd.MarkFlagRequired("config-map-name"); err != nil {
		panic(err)
	}

	CmdReporterCmd.Flags().StringVar(&namespace, "namespace", "", "The namespace in which to create the ConfigMap.")
	if err := CmdReporterCmd.MarkFlagRequired("namespace"); err != nil {
		panic(err)
	}
}

func runCmdReporter(cCmd *cobra.Command, cArgs []string) {
	// Initialize the context
	ctx, cancel := signal.NotifyContext(context.Background(), operator.ShutdownSignals...)
	defer cancel()

	cmd, args, err := util.CmdReporterFlagArgumentToCommand(commandString)
	if err != nil {
		rook.TerminateFatal(fmt.Errorf("failed to parse '--command' argument [%s]. %+v", commandString, err))
	}

	context := rook.NewContext()
	reporter, err := util.NewCmdReporter(ctx, context.Clientset, cmd, args, configMapName, namespace)
	if err != nil {
		rook.TerminateFatal(fmt.Errorf("cannot start command-reporter. %+v", err))
	}
	err = reporter.Run()
	if err != nil {
		rook.TerminateFatal(err)
	}
}
