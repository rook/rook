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
	"fmt"
	"os"

	"github.com/rook/rook/cmd/rookctl/rook"
	"github.com/rook/rook/pkg/model"
	"github.com/rook/rook/pkg/rook/client"
	"github.com/rook/rook/pkg/util/flags"
	"github.com/spf13/cobra"
)

var (
	newImageName     string
	newImagePoolName string
	newImageSize     uint64
)

var createCmd = &cobra.Command{
	Use:   "create",
	Short: "Creates a new block image in the cluster",
}

func init() {
	createCmd.Flags().StringVar(&newImageName, "name", "", "Name of new block image to create (required)")
	createCmd.Flags().StringVar(&newImagePoolName, "pool-name", "rbd", "Name of storage pool to create new block image in")
	createCmd.Flags().Uint64Var(&newImageSize, "size", 0, "Size in bytes of the new block image to create (required)")

	createCmd.MarkFlagRequired("name")
	createCmd.MarkFlagRequired("size")
	createCmd.RunE = createBlockImageEntry
}

func createBlockImageEntry(cmd *cobra.Command, args []string) error {
	rook.SetupLogging()

	if err := flags.VerifyRequiredFlags(cmd, []string{"name"}); err != nil {
		return err
	}

	if err := flags.VerifyRequiredUint64Flags(cmd, []string{"size"}); err != nil {
		return err
	}

	c := rook.NewRookNetworkRestClient()
	out, err := createBlockImage(newImageName, newImagePoolName, newImageSize, c)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}

	fmt.Println(out)
	return nil
}

func createBlockImage(imageName, poolName string, size uint64, c client.RookRestClient) (string, error) {
	newImage := model.BlockImage{Name: imageName, PoolName: poolName, Size: size}
	resp, err := c.CreateBlockImage(newImage)
	if err != nil {
		return "", fmt.Errorf("failed to create new block image '%+v': %+v", newImage, err)
	}

	return resp, nil
}
