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
	"fmt"
	"os"

	"github.com/rook/rook/cmd/rookctl/rook"
	"github.com/rook/rook/pkg/model"
	"github.com/rook/rook/pkg/rook/client"
	"github.com/rook/rook/pkg/util/flags"
	"github.com/spf13/cobra"
)

const (
	PoolTypeReplicated   = "replicated"
	PoolTypeErasureCoded = "erasure-coded"
)

var (
	newPoolName         string
	newPoolType         string
	newPoolReplicaCount uint
	newPoolDataChunks   uint
	newPoolCodingChunks uint
)

var createCmd = &cobra.Command{
	Use:   "create",
	Short: "Creates a new storage pool in the cluster",
}

func init() {
	createCmd.Flags().StringVarP(&newPoolName, "name", "n", "", "Name of new storage pool to create (required)")

	createCmd.Flags().StringVarP(&newPoolType, "type", "t", PoolTypeReplicated,
		fmt.Sprintf("Type of storage pool, '%s' or '%s' (required)", PoolTypeReplicated, PoolTypeErasureCoded))

	createCmd.Flags().UintVarP(&newPoolReplicaCount, "replica-count", "r", 0,
		fmt.Sprintf("Number of copies per object in a replicated storage pool, including the object itself (required for %s pool type)", PoolTypeReplicated))

	createCmd.Flags().UintVarP(&newPoolDataChunks, "ec-data-chunks", "d", 0,
		fmt.Sprintf("Number of data chunks per object in an erasure coded storage pool (required for %s pool type)", PoolTypeErasureCoded))

	createCmd.Flags().UintVarP(&newPoolCodingChunks, "ec-coding-chunks", "c", 0,
		fmt.Sprintf("Number of coding chunks per object in an erasure coded storage pool (required for %s pool type)", PoolTypeErasureCoded))

	createCmd.MarkFlagRequired("name")
	createCmd.MarkFlagRequired("type")
	createCmd.RunE = createPoolsEntry
}

func createPoolsEntry(cmd *cobra.Command, args []string) error {
	rook.SetupLogging()

	if err := flags.VerifyRequiredFlags(cmd, []string{"name", "type"}); err != nil {
		return err
	}

	c := rook.NewRookNetworkRestClient()
	out, err := createPool(newPoolName, newPoolType, newPoolReplicaCount, newPoolDataChunks, newPoolCodingChunks, c)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}

	fmt.Println(out)
	return nil
}

func createPool(poolName, poolType string, replicaCount, dataChunks, codingChunks uint, c client.RookRestClient) (string, error) {
	if poolType != PoolTypeReplicated && poolType != PoolTypeErasureCoded {
		return "", fmt.Errorf("invalid pool type '%s', allowed pool types are '%s' and '%s'",
			poolType, PoolTypeReplicated, PoolTypeErasureCoded)
	}

	newPool := model.Pool{Name: poolName}

	if poolType == PoolTypeReplicated {
		if dataChunks > 0 || codingChunks > 0 {
			return "", fmt.Errorf("both data chunks and coding chunks must be zero for pool type '%s'", PoolTypeReplicated)
		}

		// note that a replica count of 0 is okay, the pool will get the ceph default when it's created
		newPool.Type = model.Replicated
		newPool.ReplicationConfig.Size = replicaCount
	} else {
		if dataChunks == 0 || codingChunks == 0 {
			return "", fmt.Errorf("both data chunks and coding chunks must be greater than zero for pool type '%s'", PoolTypeErasureCoded)
		}
		newPool.Type = model.ErasureCoded
		newPool.ErasureCodedConfig.DataChunkCount = dataChunks
		newPool.ErasureCodedConfig.CodingChunkCount = codingChunks
	}

	resp, err := c.CreatePool(newPool)
	if err != nil {
		return "", fmt.Errorf("failed to create new pool '%s': %+v", newPool.Name, err)
	}

	return resp, nil
}
