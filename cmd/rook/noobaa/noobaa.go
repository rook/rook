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

// Package noobaa defines rook-noobaa commands, mainly - the operator.
package noobaa

//
//

import (
	"github.com/coreos/pkg/capnslog"
	"github.com/rook/rook/cmd/rook/rook"
	"github.com/rook/rook/pkg/operator/noobaa"
	"github.com/rook/rook/pkg/util/flags"
	"github.com/spf13/cobra"
)

var logger = capnslog.NewPackageLogger("github.com/rook/rook", "cmd/rook/noobaa")

// Cmd is the parent command for noobaa operator and tools
var Cmd = &cobra.Command{
	Use:   "noobaa",
	Short: "Main command for noobaa operator.",
	Long:  "See https://github.com/noobaa/noobaa-core and https://github.com/rook/rook",
}

// ASCIILogo is noobaa's logo ascii art
const ASCIILogo = `
 /~~\\__~__//~~\
|               |
 \~\\_     _//~/
     \\   //
      |   |
      \~~~/
`

func init() {

	initSubCommand(&cobra.Command{
		Use:   "operator",
		Short: "Run noobaa operator",
		Run: func(cmd *cobra.Command, args []string) {
			logger.Print("\n" + ASCIILogo + "\n")
			logger.Info("Running the noobaa operator ...")
			rook.SetLogLevel()
			rook.LogStartupInfo(cmd.Flags())
			noobaa.NewOperator(rook.NewContext()).Run()
		},
	})

	initSubCommand(&cobra.Command{
		Hidden: true,
		Use:    "status",
		Short:  "Read system status",
		Run: func(cmd *cobra.Command, args []string) {
			logger.Infof("TODO: %s ...", cmd.Use)
		},
	})

	initSubCommand(&cobra.Command{
		Hidden: true,
		Use:    "install",
		Short:  "Install noobaa operator",
		Run: func(cmd *cobra.Command, args []string) {
			logger.Infof("TODO: %s ...", cmd.Use)
		},
	})

	initSubCommand(&cobra.Command{
		Hidden: true,
		Use:    "create",
		Short:  "Create a new system",
		Run: func(cmd *cobra.Command, args []string) {
			logger.Infof("TODO: %s ...", cmd.Use)
		},
	})

	initSubCommand(&cobra.Command{
		Hidden: true,
		Use:    "delete",
		Short:  "Delete system",
		Run: func(cmd *cobra.Command, args []string) {
			logger.Infof("TODO: %s ...", cmd.Use)
		},
	})
}

func initSubCommand(sub *cobra.Command) {
	flags.SetFlagsFromEnv(sub.Flags(), rook.RookEnvVarPrefix)
	flags.SetLoggingFlags(sub.Flags())
	Cmd.AddCommand(sub)
}
