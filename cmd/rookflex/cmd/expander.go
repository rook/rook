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

package cmd

import (
	"encoding/json"
	"fmt"
	"github.com/rook/rook/pkg/daemon/ceph/agent/flexvolume"
	"github.com/spf13/cobra"
	"os/exec"
	"strconv"
)

var (
	expandFSCmd = &cobra.Command{
		Use:   "expandfs",
		Short: "Expands the size of pod filesystem",
		RunE:  handleExpandFs,
	}
)

func init() {
	RootCmd.AddCommand(expandFSCmd)
}

func handleExpandFs(cmd *cobra.Command, args []string) error {
	var opts = &flexvolume.ExpandOptions{}

	err := json.Unmarshal([]byte(args[0]), opts)
	if err != nil {
		return fmt.Errorf("could not parse options for expand %s, got %+v", args[1], err)
	}
	client, err := getRPCClient()
	if err != nil {
		return fmt.Errorf("error getting RPC client: %+v", err)
	}

	size, err := strconv.ParseUint(args[3], 10, 64)
	if err != nil {
		return fmt.Errorf("error while decoding RBD size: %+v", err)
	}

	err = client.Call("Controller.Expand", flexvolume.ExpandArgs{ExpandOptions: opts, Size: size}, nil)
	if err != nil {
		return fmt.Errorf("error while resizing RBD: %+v", err)
	}

	var command *exec.Cmd
	switch opts.FsType {
	case "ext3", "ext4":
		command = exec.Command("resize2fs", args[2])
	case "xfs":
		command = exec.Command("xfs_growfs", "-d", args[2])
	default:
		log(client, fmt.Sprintf("resize is not supported for fs: %s", opts.FsType), false)
		return nil
	}
	err = command.Run()
	if err != nil {
		return fmt.Errorf("error resizing FS: %+v", err)
	}
	return nil
}
