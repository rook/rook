/*
Copyright 2019 The Rook Authors. All rights reserved.

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

package util

import (
	"fmt"

	"github.com/rook/rook/cmd/rook/rook"
	"github.com/rook/rook/pkg/daemon/util"
	"github.com/spf13/cobra"
)

// CopyBinsCmd defines a top-level utility command which copies rook binary from the Rook
// container image to a directory.
var CopyBinsCmd = &cobra.Command{
	Use:   "copy-binaries",
	Short: "Copy 'rook' binary from a container to a given directory.",
	Long: `Copy 'rook' binary from a container to a given directory.
As an example, 'cmd-reporter run' may often need to be run from a container
other than the container containing the 'rook' binary. Use this command to copy
the 'rook' binary from the container containing the 'rook'
binary to a Kubernetes EmptyDir volume mounted at the given directory in a pod's
init container. From the pod's main container, mount the volume which now
contains the 'rook' binary, and call 'rook cmd-reporter run' from
'rook' in order to run the desired command from a non-Rook container.`,
	Args: cobra.NoArgs,
	RunE: runCopyBins,
}

func init() {
	// copy-binaries
	CopyBinsCmd.Flags().StringVar(&copyToDir, "copy-to-dir", "",
		"The directory into which 'rook' binary will be copied.")
	if err := CopyBinsCmd.MarkFlagRequired("copy-to-dir"); err != nil {
		panic(err)
	}
}

func runCopyBins(cCmd *cobra.Command, cArgs []string) error {
	if err := util.CopyBinaries(copyToDir); err != nil {
		rook.TerminateFatal(fmt.Errorf("could not copy binary to %s. %+v", copyToDir, err))
	}
	return nil
}
