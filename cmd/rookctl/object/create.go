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
	"fmt"
	"io/ioutil"
	"os"
	"time"

	"github.com/rook/rook/cmd/rookctl/pool"
	"github.com/rook/rook/cmd/rookctl/rook"
	"github.com/rook/rook/pkg/model"
	"github.com/rook/rook/pkg/rook/client"
	"github.com/rook/rook/pkg/util/flags"
	"github.com/spf13/cobra"
)

const (
	initObjectStoreTimeout = 60
)

var (
	store           model.ObjectStore
	dataConfig      pool.Config
	metadataConfig  pool.Config
	certificateFile string
	createCmd       = &cobra.Command{
		Use:   "create",
		Short: "Creates a new object storage instance in the cluster",
	}
)

func init() {
	createCmd.Flags().StringVarP(&store.Name, "name", "n", "default", "The name of the object store instance")
	createCmd.Flags().Int32VarP(&store.Gateway.Port, "port", "p", model.RGWPort, "The port on which to expose the object store (http)")
	createCmd.Flags().Int32VarP(&store.Gateway.SecurePort, "secure-port", "s", 0, "The port on which to expose the object store (https)")
	createCmd.Flags().StringVarP(&store.Gateway.DnsName, "dns-name", "d", store.Name, "The DNS name that will be used to access the objecstore (defaults to the objectstore service name)")
	createCmd.Flags().Int32VarP(&store.Gateway.Instances, "rgw-instances", "i", 1, "The number of RGW pods for load balancing (ignored if all nodes is set)")
	createCmd.Flags().BoolVarP(&store.Gateway.AllNodes, "rgw-all-nodes", "a", false, "Whether RGW pods should be started on all nodes")
	createCmd.Flags().StringVarP(&certificateFile, "certificate", "c", "", "Path to the ssl cert file (pem format)")
	pool.AddPoolFlags(createCmd, "data-", &dataConfig)
	pool.AddPoolFlags(createCmd, "metadata-", &metadataConfig)

	createCmd.RunE = createObjectStoreEntry
}

func createObjectStoreEntry(cmd *cobra.Command, args []string) error {
	rook.SetupLogging()
	if err := flags.VerifyRequiredFlags(cmd, []string{"name", "data-type", "metadata-type"}); err != nil {
		return err
	}

	if certificateFile != "" {
		cert, err := ioutil.ReadFile(certificateFile)
		if err != nil {
			fmt.Fprintln(os.Stderr, "failed to read certificate", err)
			os.Exit(1)
		}
		store.Gateway.Certificate = string(cert)
	}
	if store.Gateway.Certificate == "" && store.Gateway.SecurePort != 0 {
		fmt.Fprintln(os.Stderr, "must specify a certificate to set the secure port")
		os.Exit(1)
	}

	dataPool, err := pool.ConfigToModel(dataConfig)
	if err != nil {
		fmt.Fprintln(os.Stderr, "failed to read data settings", err)
		os.Exit(1)
	}
	store.DataConfig = *dataPool
	metadataPool, err := pool.ConfigToModel(metadataConfig)
	if err != nil {
		fmt.Fprintln(os.Stderr, "failed to read metadata settings", err)
		os.Exit(1)
	}
	store.MetadataConfig = *metadataPool

	c := rook.NewRookNetworkRestClientWithTimeout(initObjectStoreTimeout * time.Second)
	err = createObjectStore(c)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}

	return nil
}

func createObjectStore(c client.RookRestClient) error {

	_, err := c.CreateObjectStore(store)
	if err != nil {
		return fmt.Errorf("failed to create new object store: %+v", err)
	}

	fmt.Println("succeeded creation of object store")
	return nil
}
