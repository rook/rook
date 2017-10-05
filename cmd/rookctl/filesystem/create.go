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

	"github.com/rook/rook/cmd/rookctl/pool"
	"github.com/rook/rook/cmd/rookctl/rook"
	"github.com/rook/rook/pkg/model"
	"github.com/rook/rook/pkg/rook/client"
	"github.com/rook/rook/pkg/util/flags"
	"github.com/spf13/cobra"
)

var (
	fileRequest    model.FilesystemRequest
	dataConfig     pool.Config
	metadataConfig pool.Config
)

var createCmd = &cobra.Command{
	Use:   "create",
	Short: "Creates a new shared filesystem in the cluster",
}

func init() {
	createCmd.Flags().StringVarP(&fileRequest.Name, "name", "n", "", "Name of new filesystem to create (required)")
	createCmd.Flags().Int32VarP(&fileRequest.MetadataServer.ActiveCount, "active-mds", "a", 1, "Number of active MDS servers")
	pool.AddPoolFlags(createCmd, "data-", &dataConfig)
	pool.AddPoolFlags(createCmd, "metadata-", &metadataConfig)

	createCmd.RunE = createFilesystemEntry
}

func createFilesystemEntry(cmd *cobra.Command, args []string) error {
	rook.SetupLogging()

	if err := flags.VerifyRequiredFlags(cmd, []string{"name", "data-type", "metadata-type"}); err != nil {
		return err
	}

	dataPool, err := pool.ConfigToModel(dataConfig)
	if err != nil {
		fmt.Fprintln(os.Stderr, "failed to read data settings", err)
		os.Exit(1)
	}
	fileRequest.DataPools = append(fileRequest.DataPools, *dataPool)
	metadataPool, err := pool.ConfigToModel(metadataConfig)
	if err != nil {
		fmt.Fprintln(os.Stderr, "failed to read metadata settings", err)
		os.Exit(1)
	}
	fileRequest.MetadataPool = *metadataPool

	c := rook.NewRookNetworkRestClient()
	out, err := createFilesystem(fileRequest, c)
	if err != nil {
		fmt.Fprintln(os.Stderr, fmt.Errorf("failed to create new file system %s: %+v", fileRequest.Name, err))
		os.Exit(1)
	}
	fmt.Fprintln(os.Stdout, out)
	return nil
}

func createFilesystem(request model.FilesystemRequest, c client.RookRestClient) (string, error) {
	_, err := c.CreateFilesystem(request)
	if err != nil {
		return "", fmt.Errorf("failed to create new file system %s: %+v", request.Name, err)
	}

	return fmt.Sprintf("succeeded starting creation of shared filesystem %s", request.Name), nil
}
