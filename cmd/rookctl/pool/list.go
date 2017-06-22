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
package pool

import (
	"bytes"
	"fmt"
	"os"

	"github.com/rook/rook/cmd/rookctl/rook"
	"github.com/rook/rook/pkg/model"
	"github.com/rook/rook/pkg/rook/client"
	"github.com/rook/rook/pkg/util/display"
	"github.com/rook/rook/pkg/util/flags"
	"github.com/spf13/cobra"
)

var listCmd = &cobra.Command{
	Use:   "ls",
	Short: "Gets a listing with details of all storage pools in the cluster",
}

func init() {
	listCmd.RunE = listPoolsEntry
}

func listPoolsEntry(cmd *cobra.Command, args []string) error {
	rook.SetupLogging()

	if err := flags.VerifyRequiredFlags(cmd, []string{}); err != nil {
		return err
	}

	c := rook.NewRookNetworkRestClient()
	out, err := listPools(c)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}

	fmt.Print(out)
	return nil
}

func listPools(c client.RookRestClient) (string, error) {
	pools, err := c.GetPools()
	if err != nil {
		return "", fmt.Errorf("failed to get pools: %+v", err)
	}

	if len(pools) == 0 {
		return "", nil
	}

	var buffer bytes.Buffer
	w := rook.NewTableWriter(&buffer)

	fmt.Fprintln(w, "NAME\tNUMBER\tTYPE\tSIZE\tDATA\tCODING\tALGORITHM")

	for _, p := range pools {
		fmt.Fprintf(w, "%s\t%d\t%s\t%s\t%s\t%s\t%s\n", p.Name, p.Number, model.PoolTypeToString(p.Type),
			display.NumToStrOmitEmpty(p.ReplicationConfig.Size),
			display.NumToStrOmitEmpty(p.ErasureCodedConfig.DataChunkCount),
			display.NumToStrOmitEmpty(p.ErasureCodedConfig.CodingChunkCount),
			p.ErasureCodedConfig.Algorithm)
	}

	w.Flush()
	return buffer.String(), nil
}
