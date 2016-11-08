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
package rook

import (
	"bytes"
	"fmt"
	"net/http"
	"os"

	"github.com/rook/rook/pkg/model"
	"github.com/rook/rook/pkg/rook/client"
	"github.com/rook/rook/pkg/util/display"
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

	c := client.NewRookNetworkRestClient(client.GetRestURL(apiServerEndpoint), http.DefaultClient)
	out, err := listNodes(c)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}

	fmt.Print(out)
	return nil
}

func listNodes(c client.RookRestClient) (string, error) {
	nodes, err := c.GetNodes()
	if err != nil {
		return "", fmt.Errorf("failed to get nodes: %+v", err)
	}

	var buffer bytes.Buffer
	w := NewTableWriter(&buffer)

	// write header columns
	fmt.Fprintln(w, "PUBLIC\tPRIVATE\tSTATE\tCLUSTER\tSIZE\tLOCATION\tUPDATED\t")

	// print a row for each node
	for _, n := range nodes {
		fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\t%s\t%s ago\t\n", n.PublicIP, n.PrivateIP, model.NodeStateToString(n.State), n.ClusterName,
			display.BytesToString(n.Storage), n.Location, n.LastUpdated.String())
	}

	w.Flush()
	return buffer.String(), nil
}
