package cmd

import (
	"bytes"
	"fmt"
	"net/http"

	"github.com/quantum/castle/pkg/castlectl/client"
	"github.com/quantum/castle/pkg/util"
	"github.com/spf13/cobra"
)

var poolListCmd = &cobra.Command{
	Use:   "ls",
	Short: "Gets a listing with details of all storage pools in the cluster",
}

func init() {
	poolListCmd.RunE = listPoolsEntry
}

func listPoolsEntry(cmd *cobra.Command, args []string) error {
	if err := util.VerifyRequiredFlags(cmd, []string{}); err != nil {
		return err
	}

	c := client.NewCastleNetworkRestClient(client.GetRestURL(apiServerEndpoint), http.DefaultClient)
	out, err := listPools(c)
	if err != nil {
		return err
	}

	fmt.Println(out)
	return nil
}

func listPools(c client.CastleRestClient) (string, error) {
	pools, err := c.GetPools()
	if err != nil {
		return "", fmt.Errorf("failed to get pools: %+v", err)
	}

	// TODO: pretty print the pools listing

	var buffer bytes.Buffer

	for _, p := range pools {
		buffer.WriteString(fmt.Sprintf("%+v", p))
	}

	return buffer.String(), nil
}
