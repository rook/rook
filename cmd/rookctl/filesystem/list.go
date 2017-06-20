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
	"bytes"
	"fmt"
	"os"
	"strings"

	"github.com/rook/rook/cmd/rookctl/rook"
	"github.com/rook/rook/pkg/rook/client"
	"github.com/rook/rook/pkg/util/flags"
	"github.com/spf13/cobra"
)

var listCmd = &cobra.Command{
	Use:   "ls",
	Short: "Gets a listing with details of all shared file systems in the cluster",
}

func init() {
	listCmd.RunE = listFilesystemEntry
}

func listFilesystemEntry(cmd *cobra.Command, args []string) error {
	rook.SetupLogging()

	if err := flags.VerifyRequiredFlags(cmd, []string{}); err != nil {
		return err
	}

	c := rook.NewRookNetworkRestClient()
	out, err := listFilesystems(c)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}

	fmt.Print(out)
	return nil
}

func listFilesystems(c client.RookRestClient) (string, error) {
	filesystems, err := c.GetFilesystems()
	if err != nil {
		return "", fmt.Errorf("failed to list file systems: %+v", err)
	}

	if len(filesystems) == 0 {
		return "", nil
	}

	var buffer bytes.Buffer
	w := rook.NewTableWriter(&buffer)

	fmt.Fprintln(w, "NAME\tMETADATA POOL\tDATA POOLS")

	for _, fs := range filesystems {
		fmt.Fprintf(w, "%s\t%s\t%s\n", fs.Name, fs.MetadataPool, strings.Join(fs.DataPools, ", "))
	}

	w.Flush()
	return buffer.String(), nil
}
