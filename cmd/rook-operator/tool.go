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
	toolType string
)

var toolCmd = &cobra.Command{
	Use:    "tool",
	Short:  "Runs a rookd tool",
	Hidden: true,
}

func init() {
	toolCmd.Flags().StringVar(&toolType, "type", "", "type of tool [rgw-admin]")
	toolCmd.MarkFlagRequired("type")

	flags.SetFlagsFromEnv(toolCmd.Flags(), "ROOK-OPERATOR")

	toolCmd.RunE = runTool
}

func runTool(cmd *cobra.Command, args []string) error {
	if err := flags.VerifyRequiredFlags(toolCmd, []string{"type"}); err != nil {
		return err
	}

	setLogLevel()

	// allow rgw admin commands as well as mon and osd mkfs
	if toolType != "rgw-admin" {
		return fmt.Errorf("unknown tool type: %s", toolType)
	}

	if err := cephd.RunCommand(toolType, args); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}

	return nil
}
