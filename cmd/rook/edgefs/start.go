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

package edgefs

import (
	"github.com/rook/rook/cmd/rook/rook"
	"github.com/rook/rook/pkg/util/exec"
	"github.com/rook/rook/pkg/util/flags"
	"github.com/spf13/cobra"
)

const (
	edgefStartCmdPath = "/opt/nedge/sbin/edgefs-start.sh"
	logSectionName    = "edgefs-internal"
)

var startCmd = &cobra.Command{
	Use:   "start",
	Short: "Starts edgefs-start.sh on target node",
	Long:  `Starts edgefs-start.sh on target node`,
}

func init() {
	flags.SetFlagsFromEnv(startCmd.Flags(), rook.RookEnvVarPrefix)

	startCmd.RunE = start
}

func start(cmd *cobra.Command, args []string) error {
	rook.SetLogLevel()
	rook.LogStartupInfo(operatorCmd.Flags())
	executor := &exec.CommandExecutor{}
	err := executor.ExecuteCommand(true, logSectionName, edgefStartCmdPath, args...)

	logger.Infof("Start script %s exited: %+v", edgefStartCmdPath, err)
	return nil
}
