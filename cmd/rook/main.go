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

	"github.com/rook/rook/cmd/rook/ceph"
	"github.com/rook/rook/cmd/rook/cockroachdb"
	"github.com/rook/rook/cmd/rook/minio"
	rook "github.com/rook/rook/cmd/rook/rook"
	"github.com/rook/rook/cmd/rook/version"
)

func main() {
	addCommands()
	if err := rook.RootCmd.Execute(); err != nil {
		fmt.Printf("rook error: %+v\n", err)
	}
}

func addCommands() {
	rook.RootCmd.AddCommand(version.VersionCmd)
	rook.RootCmd.AddCommand(discoverCmd)
	rook.RootCmd.AddCommand(ceph.Cmd)
	rook.RootCmd.AddCommand(cockroachdb.Cmd)
	rook.RootCmd.AddCommand(minio.Cmd)

	// add the ceph legacy commands to the main command for backwards compatibility
	// TODO: remove these Ceph legacy commands in the future
	ceph.AddCommands(rook.RootCmd)
}
