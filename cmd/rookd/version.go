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

	etcdversion "github.com/coreos/etcd/version"
	"github.com/rook/rook/pkg/version"
	"github.com/spf13/cobra"
)

var versionCmd = &cobra.Command{
	Use:   "version",
	Short: "Print the version number of rookd",
	RunE: func(cmd *cobra.Command, args []string) error {
		fmt.Printf("rookd: %s\n", version.Version)
		fmt.Printf(" etcd: %s\n", etcdversion.Version)
		fmt.Printf("cephd: %v\n", cephVersion())
		return nil
	},
}

// get the version of the Ceph tools in the container
func cephVersion() string {
	return "notimplemented"
}
