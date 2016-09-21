package cmd

import (
	"bytes"
	"fmt"
	"net/http"

	"github.com/quantum/castle/pkg/castlectl/client"
	"github.com/quantum/castle/pkg/util"
	"github.com/spf13/cobra"
)

var nodeListCmd = &cobra.Command{
	Use:   "ls",
	Short: "Gets a listing with details of all nodes in the cluster",
}

func init() {
	nodeListCmd.RunE = listNodesEntry
}

func listNodesEntry(cmd *cobra.Command, args []string) error {
	if err := util.VerifyRequiredFlags(cmd, []string{}); err != nil {
		return err
	}

	c := client.NewCastleNetworkRestClient(client.GetRestURL(apiServerEndpoint), http.DefaultClient)
	out, err := listNodes(c)
	if err != nil {
		return err
	}

	fmt.Println(out)
	return nil
}

func listNodes(c client.CastleRestClient) (string, error) {
	nodes, err := c.GetNodes()
	if err != nil {
		return "", fmt.Errorf("failed to get nodes: %+v", err)
	}

	// TODO: pretty print the node listing

	var buffer bytes.Buffer

	for _, n := range nodes {
		buffer.WriteString(fmt.Sprintf("%+v", n))
	}

	return buffer.String(), nil
}
