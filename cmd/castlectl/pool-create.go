package main

import (
	"fmt"
	"net/http"

	"github.com/quantum/castle/pkg/castlectl/client"
	"github.com/quantum/castle/pkg/model"
	"github.com/quantum/castle/pkg/util/flags"
	"github.com/spf13/cobra"
)

var (
	newPoolName         string
	newPoolSize         uint
	newPoolDataChunks   uint
	newPoolCodingChunks uint
)

var poolCreateCmd = &cobra.Command{
	Use:   "create",
	Short: "Creates a new storage pool in the cluster",
}

func init() {
	poolCreateCmd.Flags().StringVar(&newPoolName, "name", "", "Name of new storage pool to create (required)")
	poolCreateCmd.Flags().UintVar(&newPoolSize, "size", 0,
		"Number of copies of objects in the storage pool (including object itself).  Implies a replicated pool.")
	poolCreateCmd.Flags().UintVar(&newPoolDataChunks, "data-chunks", 0,
		"Number of data chunks for objects in the storage pool.  Implies an erasure coded pool.")
	poolCreateCmd.Flags().UintVar(&newPoolCodingChunks, "coding-chunks", 0,
		"Number of coding chunks for objects in the storage pool.  Implies an erasure coded pool.")

	poolCreateCmd.MarkFlagRequired("name")
	poolCreateCmd.RunE = createPoolsEntry
}

func createPoolsEntry(cmd *cobra.Command, args []string) error {
	if err := flags.VerifyRequiredFlags(cmd, []string{"name"}); err != nil {
		return err
	}

	c := client.NewCastleNetworkRestClient(client.GetRestURL(apiServerEndpoint), http.DefaultClient)
	out, err := createPool(newPoolName, newPoolSize, newPoolDataChunks, newPoolCodingChunks, c)
	if err != nil {
		return err
	}

	fmt.Println(out)
	return nil
}

func createPool(poolName string, size, dataChunks, codingChunks uint, c client.CastleRestClient) (string, error) {
	if size > 0 && (dataChunks > 0 || codingChunks > 0) {
		return "", fmt.Errorf("Pool cannot be both replicated and erasure coded.")
	}

	newPool := model.Pool{Name: poolName}

	if size > 0 || dataChunks > 0 || codingChunks > 0 {
		if size > 0 {
			newPool.Type = model.Replicated
			newPool.ReplicationConfig.Size = size
		} else {
			newPool.Type = model.ErasureCoded
			newPool.ErasureCodedConfig.DataChunkCount = dataChunks
			newPool.ErasureCodedConfig.CodingChunkCount = codingChunks
		}
	}

	resp, err := c.CreatePool(newPool)
	if err != nil {
		return "", fmt.Errorf("failed to create new pool '%s': %+v", newPool.Name, err)
	}

	return resp, nil
}
