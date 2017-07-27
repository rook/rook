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
package main

import (
	"fmt"
	"os"

	"github.com/rook/rook/pkg/ceph/mon"
	"github.com/rook/rook/pkg/ceph/rgw"
	"github.com/rook/rook/pkg/util/flags"
	"github.com/spf13/cobra"
)

var rgwCmd = &cobra.Command{
	Use:    "rgw",
	Short:  "Generates rgw config and runs the rgw daemon",
	Hidden: true,
}

var (
	rgwKeyring string
	rgwHost    string
	rgwPort    int
)

func init() {
	rgwCmd.Flags().StringVar(&rgwKeyring, "rgw-keyring", "", "the rgw keyring")
	rgwCmd.Flags().StringVar(&rgwHost, "rgw-host", "", "dns host name")
	rgwCmd.Flags().IntVar(&rgwPort, "rgw-port", 0, "rgw port number")
	addCephFlags(rgwCmd)

	flags.SetFlagsFromEnv(rgwCmd.Flags(), "ROOKD")

	rgwCmd.RunE = startRGW
}

func startRGW(cmd *cobra.Command, args []string) error {
	if err := flags.VerifyRequiredFlags(rgwCmd, []string{"mon-endpoints", "cluster-name", "mon-secret", "admin-secret", "rgw-host", "rgw-keyring"}); err != nil {
		return err
	}

	if rgwPort == 0 {
		return fmt.Errorf("port is required")
	}

	setLogLevel()

	clusterInfo.Monitors = mon.ParseMonEndpoints(cfg.monEndpoints)
	config := &rgw.Config{
		ClusterInfo: &clusterInfo,
		Keyring:     rgwKeyring,
		Host:        rgwHost,
		Port:        rgwPort,
		InProc:      true,
	}

	err := rgw.Run(createContext(), config)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}

	return nil
}
