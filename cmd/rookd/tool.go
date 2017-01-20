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
	toolCmd.Flags().StringVar(&toolType, "type", "", "type of tool [rgw-admin|mon --mkfs|osd --mkfs]")
	toolCmd.MarkFlagRequired("type")

	toolCmd.RunE = runTool
}

func runTool(cmd *cobra.Command, args []string) error {
	if err := flags.VerifyRequiredFlags(toolCmd, []string{"type"}); err != nil {
		return err
	}

	setLogLevel()

	// allow rgw admin commands as well as mon and osd mkfs
	if toolType != "rgw-admin" && toolType != "mon" && toolType != "osd" {
		return fmt.Errorf("unknown tool type: %s", toolType)
	}

	if (toolType == "mon" || toolType == "osd") && (len(args) == 0 || args[0] != "--mkfs") {
		return fmt.Errorf("--mkfs expected for mon and osd commands")
	}

	runCephCommand(toolType, args)
	return nil
}
