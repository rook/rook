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
	"github.com/spf13/cobra"
)

var deleteCmd = &cobra.Command{
	Use:   "delete [ObjectStore]",
	Short: "Deletes an object store from the cluster",
}

func init() {
	deleteCmd.RunE = deleteBlockImageEntry
}

func deleteBlockImageEntry(cmd *cobra.Command, args []string) error {
	rook.SetupLogging()

	if err := checkObjectArgs(args, []string{}); err != nil {
		return err
	}

	c := rook.NewRookNetworkRestClient()
	err := c.DeleteObjectStore(args[0])
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}

	return nil
}
