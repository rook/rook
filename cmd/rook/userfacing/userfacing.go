/*
Copyright 2023 The Rook Authors. All rights reserved.

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

// contains user facing commands, separated to a subdir for convenience
package userfacing

import (
	"context"
	"os"
	"os/signal"
	"syscall"

	"github.com/rook/rook/cmd/rook/rook"
	"github.com/rook/rook/cmd/rook/userfacing/multus"
	"github.com/spf13/cobra"
)

var Commands = []*cobra.Command{
	multus.Cmd,
}

var stopSignalCapture context.CancelFunc

func init() {
	// ensure all user-facing commands have some basic assumptions covered
	for _, cmd := range Commands {
		// they should not be hidden
		cmd.Hidden = false

		// they should have a context that captures ctrl-c interrupts
		cmd.PersistentPreRunE = func(cmd *cobra.Command, args []string) error {
			ctx := context.Background()
			ctx, stopSignalCapture = signal.NotifyContext(ctx, os.Interrupt, syscall.SIGTERM)
			cmd.SetContext(ctx)

			rook.SetLogLevel()

			return cmd.ValidateArgs(args)
		}
		// and should stop capturing when exiting
		cmd.PersistentPostRun = func(cmd *cobra.Command, args []string) {
			if stopSignalCapture != nil {
				stopSignalCapture()
			}
		}

		// they should always provide help
		cmd.InitDefaultHelpCmd()
		cmd.InitDefaultHelpFlag()
	}
}
