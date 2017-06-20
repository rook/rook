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
	"github.com/rook/rook/pkg/model"
	"github.com/rook/rook/pkg/rook/client"
	"github.com/rook/rook/pkg/util/flags"
	"github.com/spf13/cobra"
)

var (
	newFilesystemName string
)

var createCmd = &cobra.Command{
	Use:   "create",
	Short: "Creates a new shared filesystem in the cluster",
}

func init() {
	createCmd.Flags().StringVarP(&newFilesystemName, "name", "n", "", "Name of new filesystem to create (required)")

	createCmd.MarkFlagRequired("name")
	createCmd.RunE = createFilesystemEntry
}

func createFilesystemEntry(cmd *cobra.Command, args []string) error {
	rook.SetupLogging()

	if err := flags.VerifyRequiredFlags(cmd, []string{"name"}); err != nil {
		return err
	}

	c := rook.NewRookNetworkRestClient()
	out, err := createFilesystem(newFilesystemName, c)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}

	fmt.Println(out)
	return nil
}

func createFilesystem(filesystemName string, c client.RookRestClient) (string, error) {
	newFilesystem := model.FilesystemRequest{Name: filesystemName, PoolName: filesystemName}
	_, err := c.CreateFilesystem(newFilesystem)

	// HTTP 202 Accepted is expected
	if err != nil && !client.IsHttpAccepted(err) {
		return "", fmt.Errorf("failed to create new file system '%+v': %+v", newFilesystem, err)
	}

	return fmt.Sprintf("succeeded starting creation of shared filesystem %s", filesystemName), nil
}
