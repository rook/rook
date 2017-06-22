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
package filesystem

import (
	"fmt"
	"os"

	"github.com/rook/rook/cmd/rookctl/rook"
	"github.com/rook/rook/pkg/util/exec"
	"github.com/rook/rook/pkg/util/flags"
	"github.com/rook/rook/pkg/util/sys"
	"github.com/spf13/cobra"
)

var (
	unmountFilesystemPath string
)

var unmountCmd = &cobra.Command{
	Use:     "unmount",
	Aliases: []string{"umount"},
	Short:   "Unmounts a shared filesystem from its local mount point (data is still persisted in the cluster)",
}

func init() {
	unmountCmd.Flags().StringVarP(&unmountFilesystemPath, "path", "p", "", "Path to unmount shared filesystem from (required)")

	unmountCmd.MarkFlagRequired("path")
	unmountCmd.RunE = unmountFilesystemEntry
}

func unmountFilesystemEntry(cmd *cobra.Command, args []string) error {
	rook.SetupLogging()

	if err := flags.VerifyRequiredFlags(cmd, []string{"path"}); err != nil {
		return err
	}

	e := &exec.CommandExecutor{}
	out, err := unmountFilesystem(unmountFilesystemPath, e)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}

	fmt.Println(out)
	return nil
}

func unmountFilesystem(path string, executor exec.Executor) (string, error) {
	if err := sys.UnmountDevice(path, executor); err != nil {
		return "", err
	}

	return fmt.Sprintf("succeeded unmounting shared filesystem from '%s'", path), nil
}
