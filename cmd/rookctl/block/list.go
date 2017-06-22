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
package block

import (
	"bytes"
	"fmt"
	"os"
	"runtime"
	"strings"

	"github.com/rook/rook/cmd/rookctl/rook"
	"github.com/rook/rook/pkg/rook/client"
	"github.com/rook/rook/pkg/util/display"
	"github.com/rook/rook/pkg/util/exec"
	"github.com/rook/rook/pkg/util/flags"
	"github.com/rook/rook/pkg/util/sys"
	"github.com/spf13/cobra"
)

var listCmd = &cobra.Command{
	Use:   "ls",
	Short: "Gets a listing with details of all block images in the cluster and their locally mapped devices",
}

func init() {
	listCmd.RunE = listBlocksEntry
}

func listBlocksEntry(cmd *cobra.Command, args []string) error {
	rook.SetupLogging()

	if err := flags.VerifyRequiredFlags(cmd, []string{}); err != nil {
		return err
	}

	c := rook.NewRookNetworkRestClient()
	e := &exec.CommandExecutor{}
	out, err := listBlocks(rbdSysBusPathDefault, c, e)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}

	fmt.Print(out)
	return nil
}

func listBlocks(rbdSysBusPath string, c client.RookRestClient, executor exec.Executor) (string, error) {
	images, err := c.GetBlockImages()
	if err != nil {
		return "", fmt.Errorf("failed to get block images: %+v", err)
	}

	if len(images) == 0 {
		return "", nil
	}

	if runtime.GOOS == "linux" {
		// for each image returned from the client API call, look up local details
		for i := range images {
			image := &(images[i])

			// look up local device and mount point, ignoring errors
			devPath, _ := findDevicePath(image.Name, image.PoolName, rbdSysBusPath)
			dev := strings.TrimPrefix(devPath, devicePathPrefix)
			var mountPoint string
			if dev != "" {
				mountPoint, _ = sys.GetDeviceMountPoint(dev, executor)
			}

			image.Device = dev
			image.MountPoint = mountPoint
		}
	}

	var buffer bytes.Buffer
	w := rook.NewTableWriter(&buffer)

	fmt.Fprintln(w, "NAME\tPOOL\tSIZE\tDEVICE\tMOUNT")

	for _, i := range images {
		fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\n", i.Name, i.PoolName, display.BytesToString(i.Size), i.Device, i.MountPoint)
	}

	w.Flush()
	return buffer.String(), nil
}
