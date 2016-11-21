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
package rook

import (
	"runtime"

	"github.com/spf13/cobra"
)

var blockCmd = &cobra.Command{
	Use:   "block",
	Short: "Performs commands and operations on block devices and images in the cluster",
}

func init() {
	blockCmd.AddCommand(blockListCmd)
	blockCmd.AddCommand(blockCreateCmd)

	if runtime.GOOS == "linux" {
		blockCmd.AddCommand(blockMountCmd)
		blockCmd.AddCommand(blockUnmountCmd)
	}
}
