package main

import (
	"bytes"
	"fmt"
	"net/http"

	"github.com/quantum/castle/pkg/castlectl/client"
	"github.com/quantum/castle/pkg/util/flags"
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
	if err := flags.VerifyRequiredFlags(cmd, []string{}); err != nil {
		return err
	}

	c := client.NewCastleNetworkRestClient(client.GetRestURL(apiServerEndpoint), http.DefaultClient)
	out, err := listPools(c)
	if err != nil {
		return err
	}

	fmt.Print(out)
	return nil
}

func listPools(c client.CastleRestClient) (string, error) {
	pools, err := c.GetPools()
	if err != nil {
		return "", fmt.Errorf("failed to get pools: %+v", err)
	}

	var buffer bytes.Buffer
	w := NewTableWriter(&buffer)

	fmt.Fprintln(w, "NAME\tNUMBER\t")

	for _, p := range pools {
		fmt.Fprintf(w, "%s\t%d\t\n", p.Name, p.Number)
	}

	w.Flush()
	return buffer.String(), nil
}
