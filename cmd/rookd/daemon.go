// +build linux,amd64 linux,arm64

/*
Copyright 2016 The Rook Authors. All rights reserved.

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
package main

import (
	"fmt"
	"os"

	"github.com/rook/rook/pkg/cephmgr/cephd"
	"github.com/rook/rook/pkg/util/flags"
	"github.com/spf13/cobra"
)

var (
	daemonType string
)

var daemonCmd = &cobra.Command{
	Use:    "daemon",
	Short:  "Runs a rookd daemon",
	Hidden: true,
}

func init() {
	daemonCmd.Flags().StringVar(&daemonType, "type", "", "type of daemon [mon|osd|mds|rgw]")
	daemonCmd.MarkFlagRequired("type")

	daemonCmd.RunE = runDaemon
}

func runDaemon(cmd *cobra.Command, args []string) error {
	if err := flags.VerifyRequiredFlags(daemonCmd, []string{"type"}); err != nil {
		return err
	}
	if daemonType != "mon" && daemonType != "mds" && daemonType != "rgw-admin" {
		return fmt.Errorf("unknown daemon type: %s", daemonType)
	}

	runCephCommand(daemonType, args)
	return nil
}

// run a command in libcephd
func runCephCommand(command string, args []string) {

	// The command passes through args to the child process.  Look for the
	// terminator arg, and pass through all args after that (without a terminator arg,
	// FlagSet.Parse prints errors for args it doesn't recognize)
	passthruIndex := 3
	for i := range os.Args {
		if os.Args[i] == "--" {
			passthruIndex = i + 1
			break
		}
	}

	// run the specified command
	if err := cephd.New().Run(command, os.Args[passthruIndex:]...); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}

}
