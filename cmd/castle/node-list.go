package castle

import (
	"bytes"
	"fmt"
	"net/http"
	"os"

	"github.com/quantum/castle/pkg/castle/client"
	"github.com/quantum/castle/pkg/model"
	"github.com/quantum/castle/pkg/util/display"
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
	// verify required flags. currently there are none

	c := client.NewCastleNetworkRestClient(client.GetRestURL(apiServerEndpoint), http.DefaultClient)
	out, err := listNodes(c)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}

	fmt.Print(out)
	return nil
}

func listNodes(c client.CastleRestClient) (string, error) {
	nodes, err := c.GetNodes()
	if err != nil {
		return "", fmt.Errorf("failed to get nodes: %+v", err)
	}

	var buffer bytes.Buffer
	w := NewTableWriter(&buffer)

	// write header columns
	fmt.Fprintln(w, "ADDRESS\tSTATE\tCLUSTER\tSIZE\tLOCATION\tUPDATED\t")

	// print a row for each node
	for _, n := range nodes {
		fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\t%s ago\t\n", n.IPAddress, model.NodeStateToString(n.State), n.ClusterName,
			display.BytesToString(n.Storage), n.Location, n.LastUpdated.String())
	}

	w.Flush()
	return buffer.String(), nil
}
