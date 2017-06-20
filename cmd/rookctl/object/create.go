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
	"github.com/rook/rook/pkg/rook/client"
	"github.com/spf13/cobra"
)

var createCmd = &cobra.Command{
	Use:   "create",
	Short: "Creates a new object storage instance in the cluster",
}

func init() {
	createCmd.RunE = createObjectStoreEntry
}

func createObjectStoreEntry(cmd *cobra.Command, args []string) error {
	rook.SetupLogging()

	c := rook.NewRookNetworkRestClient()
	out, err := createObjectStore(c)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}

	fmt.Println(out)
	return nil
}

func createObjectStore(c client.RookRestClient) (string, error) {
	_, err := c.CreateObjectStore()

	// HTTP 202 Accepted is expected
	if err != nil && !client.IsHttpAccepted(err) {
		return "", fmt.Errorf("failed to create new object store: %+v", err)
	}

	return fmt.Sprintf("succeeded starting creation of object store"), nil
}
