/*
Copyright 2017 The Rook Authors. All rights reserved.

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
	"bytes"
	"fmt"
	"os"

	"github.com/rook/rook/cmd/rook/rook"
	"github.com/rook/rook/pkg/rook/client"
	"github.com/rook/rook/pkg/util/flags"
	"github.com/spf13/cobra"
)

var bucketCmd = &cobra.Command{
	Use:   "bucket",
	Short: "Performs commands and operations on object store buckets in the cluster",
}

func init() {
	bucketCmd.AddCommand(bucketListCmd)
	bucketListCmd.RunE = listBucketsEntry
}

var bucketListCmd = &cobra.Command{
	Use:   "list",
	Short: "Gets a listing with details of all buckets in the cluster",
}

func listBucketsEntry(cmd *cobra.Command, args []string) error {
	rook.SetupLogging()

	if err := flags.VerifyRequiredFlags(cmd, []string{}); err != nil {
		return err
	}

	c := rook.NewRookNetworkRestClient()
	out, err := listBuckets(c)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}

	fmt.Print(out)
	return nil
}

func listBuckets(c client.RookRestClient) (string, error) {
	buckets, err := c.ListBuckets()
	if err != nil {
		return "", fmt.Errorf("failed to get pools: %+v", err)
	}

	if len(buckets) == 0 {
		return "", nil
	}

	var buffer bytes.Buffer
	w := rook.NewTableWriter(&buffer)

	fmt.Fprintln(w, "NAME\tOWNER\tCREATED AT\tSIZE\tNUMBER OF OBJECTS")

	for _, b := range buckets {
		fmt.Fprintf(w, "%s\t%s\t%s\t%d\t%d\n", b.Name, b.Owner, b.CreatedAt, b.Size, b.NumberOfObjects)
	}

	w.Flush()
	return buffer.String(), nil
}
