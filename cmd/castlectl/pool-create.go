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
	newPoolName string
)

var poolCreateCmd = &cobra.Command{
	Use:   "create",
	Short: "Creates a new storage pool in the cluster",
}

func init() {
	poolCreateCmd.Flags().StringVar(&newPoolName, "pool-name", "", "Name of new storage pool to created (required)")

	poolCreateCmd.MarkFlagRequired("pool-name")
	poolCreateCmd.RunE = createPoolsEntry
}

func createPoolsEntry(cmd *cobra.Command, args []string) error {
	if err := flags.VerifyRequiredFlags(cmd, []string{"pool-name"}); err != nil {
		return err
	}

	c := client.NewCastleNetworkRestClient(client.GetRestURL(apiServerEndpoint), http.DefaultClient)
	out, err := createPool(newPoolName, c)
	if err != nil {
		return err
	}

	fmt.Println(out)
	return nil
}

func createPool(poolName string, c client.CastleRestClient) (string, error) {
	newPool := model.Pool{Name: poolName}
	resp, err := c.CreatePool(newPool)
	if err != nil {
		return "", fmt.Errorf("failed to create new pool '%+v': %+v", newPool, err)
	}

	return resp, nil
}
