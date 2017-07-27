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

	"github.com/rook/rook/cmd/rookctl/block"
	"github.com/rook/rook/cmd/rookctl/filesystem"
	"github.com/rook/rook/cmd/rookctl/node"
	"github.com/rook/rook/cmd/rookctl/object"
	"github.com/rook/rook/cmd/rookctl/pool"
	"github.com/rook/rook/cmd/rookctl/rook"
	"github.com/rook/rook/cmd/rookctl/status"
	"github.com/rook/rook/cmd/rookctl/version"
)

func main() {
	addCommands()
	if err := rook.RootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func addCommands() {
	rook.RootCmd.AddCommand(node.Cmd)
	rook.RootCmd.AddCommand(pool.Cmd)
	rook.RootCmd.AddCommand(block.Cmd)
	rook.RootCmd.AddCommand(filesystem.Cmd)
	rook.RootCmd.AddCommand(object.Cmd)
	rook.RootCmd.AddCommand(status.Cmd)
	rook.RootCmd.AddCommand(version.Cmd)
}
