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
	deleteImageName     string
	deleteImagePoolName string
)

var deleteCmd = &cobra.Command{
	Use:   "delete",
	Short: "Deletes a block image from the cluster",
}

func init() {
	deleteCmd.Flags().StringVar(&deleteImageName, "name", "", "Name of block image to delete (required)")
	deleteCmd.Flags().StringVar(&deleteImagePoolName, "pool-name", "rbd", "Name of storage pool to delete block image from")

	deleteCmd.MarkFlagRequired("name")
	deleteCmd.RunE = deleteBlockImageEntry
}

func deleteBlockImageEntry(cmd *cobra.Command, args []string) error {
	rook.SetupLogging()

	if err := flags.VerifyRequiredFlags(cmd, []string{"name"}); err != nil {
		return err
	}

	c := rook.NewRookNetworkRestClient()
	out, err := deleteBlockImage(deleteImageName, deleteImagePoolName, c)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}

	fmt.Println(out)
	return nil
}

func deleteBlockImage(imageName, poolName string, c client.RookRestClient) (string, error) {
	i := model.BlockImage{Name: imageName, PoolName: poolName}
	resp, err := c.DeleteBlockImage(i)
	if err != nil {
		return "", fmt.Errorf("failed to delete block image '%+v': %+v", i, err)
	}

	return resp, nil
}
