/*
Copyright 2017 The Rook Authors. All rights reserved.

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

package cmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

var (
	initCmd = &cobra.Command{
		Use:   "init",
		Short: "Initialize the volume plugin",
		RunE:  initPlugin,
	}
)

func init() {
	RootCmd.AddCommand(initCmd)
}

func initPlugin(cmd *cobra.Command, args []string) error {
	fmt.Println(`{"status":"Success", "capabilities": {"attach": false}}`)
	os.Exit(0)
	return nil
}
