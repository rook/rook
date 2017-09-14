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

	"github.com/rook/rook/cmd/rookctl/rook"
	"github.com/rook/rook/pkg/rook/client"
	"github.com/spf13/cobra"
)

var (
	purge bool
)

var bucketCmd = &cobra.Command{
	Use:   "bucket",
	Short: "Performs commands and operations on object store buckets",
}

func init() {
	bucketCmd.AddCommand(bucketListCmd)
	bucketListCmd.RunE = listBucketsEntry
	bucketCmd.AddCommand(bucketGetCmd)
	bucketGetCmd.RunE = getBucketEntry
	bucketCmd.AddCommand(bucketDeleteCmd)
	bucketDeleteCmd.RunE = deleteBucketEntry

	bucketDeleteCmd.Flags().BoolVarP(&purge, "purge", "p", false, "delete bucket contents")
}

var bucketListCmd = &cobra.Command{
	Use:   "list [ObjectStore]",
	Short: "Gets a listing with details of all buckets in the object store",
}

func listBucketsEntry(cmd *cobra.Command, args []string) error {
	rook.SetupLogging()

	if err := checkObjectArgs(args, []string{}); err != nil {
		return err
	}

	c := rook.NewRookNetworkRestClient()
	out, err := listBuckets(args[0], c)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}

	fmt.Print(out)
	return nil
}

func listBuckets(storeName string, c client.RookRestClient) (string, error) {
	buckets, err := c.ListBuckets(storeName)
	if err != nil {
		return "", fmt.Errorf("failed to list buckets: %+v", err)
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

var bucketGetCmd = &cobra.Command{
	Use:   "get [ObjectStore] [BucketName]",
	Short: "Gets the details of a bucket in the object store",
}

func getBucketEntry(cmd *cobra.Command, args []string) error {
	rook.SetupLogging()

	if err := checkObjectArgs(args, []string{"[BucketName]"}); err != nil {
		return err
	}

	c := rook.NewRookNetworkRestClient()
	out, err := getBucket(c, args[0], args[1])
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}

	fmt.Print(out)
	return nil
}

func getBucket(c client.RookRestClient, storeName, bucketName string) (string, error) {
	bucket, err := c.GetBucket(storeName, bucketName)
	if err != nil {
		return "", fmt.Errorf("failed to get bucket: %+v", err)
	}

	var buffer bytes.Buffer
	w := rook.NewTableWriter(&buffer)

	fmt.Fprintf(w, "Name:\t%s\n", bucket.Name)
	fmt.Fprintf(w, "Owner:\t%s\n", bucket.Owner)
	fmt.Fprintf(w, "Creation time:\t%s\n", bucket.CreatedAt)
	fmt.Fprintf(w, "Size:\t%d\n", bucket.Size)
	fmt.Fprintf(w, "Number of Objects:\t%d\n", bucket.NumberOfObjects)

	w.Flush()
	return buffer.String(), nil
}

var bucketDeleteCmd = &cobra.Command{
	Use:   "delete [ObjectStore] [BucketName]",
	Short: "Deletes the bucket from the object store",
}

func deleteBucketEntry(cmd *cobra.Command, args []string) error {
	rook.SetupLogging()

	if err := checkObjectArgs(args, []string{"[BucketName]"}); err != nil {
		return err
	}

	c := rook.NewRookNetworkRestClient()
	out, err := deleteBucket(c, args[0], args[1])
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}

	fmt.Print(out)
	return nil
}

func deleteBucket(c client.RookRestClient, storeName, bucketName string) (string, error) {
	err := c.DeleteBucket(storeName, bucketName, purge)

	if err != nil {
		if client.IsHttpNotFound(err) {
			return "", fmt.Errorf("Unable to find bucket %s", bucketName)
		}
		return "", fmt.Errorf("failed to delete bucket: %+v", err)
	}
	return "Bucket deleted\n", nil
}
