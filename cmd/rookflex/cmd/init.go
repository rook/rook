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
	"os/exec"

	"github.com/rook/rook/pkg/util/sys"
	"github.com/spf13/cobra"
)

const (
	moduleName = "rbd"
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

	hasSingleMajor := false
	// check to see if the rbd kernel module has single_major support
	out, err := exec.Command("modinfo", "-F", "parm", moduleName).Output()
	if err == nil {
		hasSingleMajor = sys.Grep(string(out), "^single_major") != ""
	}

	opts := []string{moduleName}
	if hasSingleMajor {
		opts = append(opts, "single_major=Y")
	}

	// load the rbd kernel module with options
	// TODO: should this fail if modprobe fails?
	exec.Command("modprobe", opts...).Run()

	fmt.Println(`{"status":"Success", "capabilities": {"attach": false}}`)
	os.Exit(0)
	return nil
}
