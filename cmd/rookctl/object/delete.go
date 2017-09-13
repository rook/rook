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
package object

import (
	"fmt"
	"os"

	"github.com/rook/rook/cmd/rookctl/rook"
	"github.com/rook/rook/pkg/util/flags"
	"github.com/spf13/cobra"
)

var (
	deleteObjectStoreName string
)

var deleteCmd = &cobra.Command{
	Use:   "delete",
	Short: "Deletes an object store from the cluster",
}

func init() {
	deleteCmd.Flags().StringVar(&deleteObjectStoreName, "name", "", "Name of object store to delete (required)")

	deleteCmd.RunE = deleteBlockImageEntry
}

func deleteBlockImageEntry(cmd *cobra.Command, args []string) error {
	rook.SetupLogging()

	if err := flags.VerifyRequiredFlags(cmd, []string{"name"}); err != nil {
		return err
	}

	c := rook.NewRookNetworkRestClient()
	err := c.DeleteObjectStore(deleteObjectStoreName)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}

	return nil
}
