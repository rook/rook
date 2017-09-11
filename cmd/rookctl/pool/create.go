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
	newPoolName string
	poolConfig  Config
)

var createCmd = &cobra.Command{
	Use:   "create",
	Short: "Creates a new storage pool in the cluster",
}

type Config struct {
	PoolType     string
	ReplicaCount uint
	DataChunks   uint
	CodingChunks uint
}

func init() {
	createCmd.Flags().StringVarP(&newPoolName, "name", "n", "", "Name of new storage pool to create (required)")
	AddPoolFlags(createCmd, "", &poolConfig)

	createCmd.RunE = createPoolsEntry
}

func AddPoolFlags(cmd *cobra.Command, prefix string, config *Config) {
	shorthand := ""
	if prefix == "" {
		shorthand = "t"
	}
	cmd.Flags().StringVarP(&config.PoolType, prefix+"type", shorthand, PoolTypeReplicated,
		fmt.Sprintf("Type of storage pool, '%s' or '%s'", PoolTypeReplicated, PoolTypeErasureCoded))

	shorthand = ""
	if prefix == "" {
		shorthand = "r"
	}
	cmd.Flags().UintVarP(&config.ReplicaCount, prefix+"replica-count", shorthand, 0,
		fmt.Sprintf("Number of copies per object in a replicated storage pool, including the object itself (required for %s pool type)", PoolTypeReplicated))

	shorthand = ""
	if prefix == "" {
		shorthand = "d"
	}
	cmd.Flags().UintVarP(&config.DataChunks, prefix+"ec-data-chunks", shorthand, 0,
		fmt.Sprintf("Number of data chunks per object in an erasure coded storage pool (required for %s pool type)", PoolTypeErasureCoded))

	shorthand = ""
	if prefix == "" {
		shorthand = "c"
	}
	cmd.Flags().UintVarP(&config.CodingChunks, prefix+"ec-coding-chunks", shorthand, 0,
		fmt.Sprintf("Number of coding chunks per object in an erasure coded storage pool (required for %s pool type)", PoolTypeErasureCoded))
}

func createPoolsEntry(cmd *cobra.Command, args []string) error {
	rook.SetupLogging()

	if err := flags.VerifyRequiredFlags(cmd, []string{"name", "type"}); err != nil {
		return err
	}

	newPool, err := ConfigToModel(poolConfig)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	newPool.Name = newPoolName

	c := rook.NewRookNetworkRestClient()
	out, err := createPool(*newPool, c)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}

	fmt.Println(out)
	return nil
}

func ConfigToModel(config Config) (*model.Pool, error) {

	if config.PoolType != PoolTypeReplicated && config.PoolType != PoolTypeErasureCoded {
		return nil, fmt.Errorf("invalid pool type '%s', allowed pool types are '%s' and '%s'",
			config.PoolType, PoolTypeReplicated, PoolTypeErasureCoded)
	}

	newPool := &model.Pool{}

	if config.PoolType == PoolTypeReplicated {
		if config.DataChunks > 0 || config.CodingChunks > 0 {
			return nil, fmt.Errorf("both data chunks and coding chunks must be zero for pool type '%s'", PoolTypeReplicated)
		}

		// note that a replica count of 0 is okay, the pool will get the ceph default when it's created
		newPool.Type = model.Replicated
		newPool.ReplicatedConfig.Size = config.ReplicaCount
	} else {
		if config.DataChunks == 0 || config.CodingChunks == 0 {
			return nil, fmt.Errorf("both data chunks and coding chunks must be greater than zero for pool type '%s'", PoolTypeErasureCoded)
		}
		newPool.Type = model.ErasureCoded
		newPool.ErasureCodedConfig.DataChunkCount = config.DataChunks
		newPool.ErasureCodedConfig.CodingChunkCount = config.CodingChunks
	}

	return newPool, nil
}

func createPool(newPool model.Pool, c client.RookRestClient) (string, error) {
	resp, err := c.CreatePool(newPool)
	if err != nil {
		return "", fmt.Errorf("failed to create new pool '%s': %+v", newPool.Name, err)
	}

	return resp, nil
}
