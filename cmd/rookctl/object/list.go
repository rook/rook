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
package object

import (
	"bytes"
	"fmt"
	"os"
	"strconv"
	"strings"

	"k8s.io/api/core/v1"

	"github.com/rook/rook/cmd/rookctl/rook"
	"github.com/rook/rook/pkg/rook/client"
	"github.com/rook/rook/pkg/util/flags"
	"github.com/spf13/cobra"
)

const noneEntry = "<none>"

var listCmd = &cobra.Command{
	Use:   "ls",
	Short: "Gets a listing of object stores in the cluster",
}

func init() {
	listCmd.RunE = listObjectStoresEntry
}

func listObjectStoresEntry(cmd *cobra.Command, args []string) error {
	rook.SetupLogging()

	if err := flags.VerifyRequiredFlags(cmd, []string{}); err != nil {
		return err
	}

	c := rook.NewRookNetworkRestClient()
	out, err := listObjectStores(c)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}

	fmt.Print(out)
	return nil
}

func listObjectStores(c client.RookRestClient) (string, error) {
	stores, err := c.GetObjectStores()
	if err != nil {
		return "", fmt.Errorf("failed to get object stores. %+v", err)
	}

	if len(stores) == 0 {
		return "", nil
	}

	var buffer bytes.Buffer
	w := rook.NewTableWriter(&buffer)

	fmt.Fprintln(w, "NAME\tCLUSTER-IP\tEXTERNAL-IP(s)\tPORT(s)")
	for _, s := range stores {
		fmt.Fprintf(w, "%s\t%s\t%s\t%s\n", s.Name, s.ClusterIP, externalIPsToString(s.ExternalIPs), portsToString(s.Ports))
	}

	w.Flush()
	return buffer.String(), nil
}

func portsToString(ports []v1.ServicePort) string {
	if len(ports) == 0 {
		return noneEntry
	}
	var result []string
	for _, port := range ports {
		result = append(result, strconv.Itoa(int(port.Port)))
	}
	return strings.Join(result, ",")
}

func externalIPsToString(ips []string) string {
	if len(ips) == 0 {
		return noneEntry
	}
	return strings.Join(ips, ",")
}
