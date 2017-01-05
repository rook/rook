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
	"github.com/rook/rook/pkg/util/flags"
	"github.com/spf13/cobra"
)

var monCmd = &cobra.Command{
	Use:    "mon",
	Short:  "Generates mon config and runs the mon daemon",
	Hidden: true,
}

func init() {
	monCmd.RunE = startMon
}

func startMon(cmd *cobra.Command, args []string) error {
	if err := flags.VerifyRequiredFlags(toolCmd, []string{""}); err != nil {
		return err
	}

	// mon.Start(config)
	return nil
}
