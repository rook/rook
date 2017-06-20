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
	"strings"

	rc "github.com/rook/rook/cmd/rookctl/client"
	"github.com/rook/rook/cmd/rookctl/rook"
	"github.com/rook/rook/pkg/rook/client"
	"github.com/rook/rook/pkg/util/exec"
	"github.com/rook/rook/pkg/util/flags"
	"github.com/rook/rook/pkg/util/sys"
	"github.com/spf13/cobra"
)

var (
	mountFilesystemName string
	mountFilesystemPath string
)

var mountCmd = &cobra.Command{
	Use:   "mount",
	Short: "Mounts a shared filesystem from the cluster to the given local mount point",
}

func init() {
	mountCmd.Flags().StringVarP(&mountFilesystemName, "name", "n", "", "Name of filesystem to mount (required)")
	mountCmd.Flags().StringVarP(&mountFilesystemPath, "path", "p", "", "Path to mount shared filesystem (required)")

	mountCmd.MarkFlagRequired("name")
	mountCmd.MarkFlagRequired("path")
	mountCmd.RunE = mountFilesystemEntry
}

func mountFilesystemEntry(cmd *cobra.Command, args []string) error {
	rook.SetupLogging()

	if err := flags.VerifyRequiredFlags(cmd, []string{"name", "path"}); err != nil {
		return err
	}

	c := rook.NewRookNetworkRestClient()
	e := &exec.CommandExecutor{}
	out, err := mountFilesystem(mountFilesystemName, mountFilesystemPath, c, e)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}

	fmt.Println(out)
	return nil
}

func mountFilesystem(name, path string, c client.RookRestClient, executor exec.Executor) (string, error) {
	clientAccessInfo, err := c.GetClientAccessInfo()
	if err != nil {
		return "", err
	}

	if err := rc.VerifyClientAccessInfo(clientAccessInfo); err != nil {
		return "", err
	}

	monAddrs := rc.ProcessMonAddresses(clientAccessInfo)
	devicePath := strings.Join(monAddrs, ",") + ":/"
	options := fmt.Sprintf("name=%s,secret=%s", clientAccessInfo.UserName, clientAccessInfo.SecretKey)

	// runs a mount command in the general form of:
	// > mount -t ceph -o name=admin,secret=AQAWczxYpVlFGTAAb3UezRRtT3C+2hiiu/4fJA== 127.0.0.1:6790:/ /tmp/myfsmount
	if err := sys.MountDeviceWithOptions(devicePath, path, "ceph", options, executor); err != nil {
		return "", err
	}

	return fmt.Sprintf("succeeded mounting shared filesystem %s at '%s'", name, path), nil
}
