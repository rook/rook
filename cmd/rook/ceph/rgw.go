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
package ceph

import (
	"fmt"
	"os"

	"github.com/rook/rook/cmd/rook/rook"
	"github.com/rook/rook/pkg/daemon/ceph/mon"
	"github.com/rook/rook/pkg/daemon/ceph/rgw"
	"github.com/rook/rook/pkg/util/flags"
	"github.com/spf13/cobra"
)

var rgwCmd = &cobra.Command{
	Use:    "rgw",
	Short:  "Generates rgw config and runs the rgw daemon",
	Hidden: true,
}

var (
	rgwName       string
	rgwKeyring    string
	rgwHost       string
	rgwCert       string
	rgwPort       int
	rgwSecurePort int
)

func init() {
	rgwCmd.Flags().StringVar(&rgwName, "rgw-name", "", "name of the object store")
	rgwCmd.Flags().StringVar(&rgwKeyring, "rgw-keyring", "", "the rgw keyring")
	rgwCmd.Flags().StringVar(&rgwHost, "rgw-host", os.Getenv("HOSTNAME"), "RGW host name. Becomes the only accepted hostname if the rgw dns name property is unset. Defaults to the pod hostname")
	rgwCmd.Flags().StringVar(&rgwCert, "rgw-cert", "", "path to the ssl certificate in pem format")
	rgwCmd.Flags().IntVar(&rgwPort, "rgw-port", 0, "rgw port (http)")
	rgwCmd.Flags().IntVar(&rgwSecurePort, "rgw-secure-port", 0, "rgw secure port number (https)")
	addCephFlags(rgwCmd)

	flags.SetFlagsFromEnv(rgwCmd.Flags(), rook.RookEnvVarPrefix)

	rgwCmd.RunE = startRGW
}

func startRGW(cmd *cobra.Command, args []string) error {
	required := []string{"mon-endpoints", "cluster-name", "rgw-name", "rgw-keyring", "public-ipv4", "private-ipv4"}
	if err := flags.VerifyRequiredFlags(rgwCmd, required); err != nil {
		return err
	}

	if rgwPort == 0 && rgwSecurePort == 0 {
		return fmt.Errorf("port or secure port are required")
	}

	rook.SetLogLevel()

	rook.LogStartupInfo(rgwCmd.Flags())

	clusterInfo.Monitors = mon.ParseMonEndpoints(cfg.monEndpoints)
	config := &rgw.Config{
		ClusterInfo:     &clusterInfo,
		Name:            rgwName,
		Keyring:         rgwKeyring,
		Host:            rgwHost,
		Port:            rgwPort,
		SecurePort:      rgwSecurePort,
		CertificatePath: rgwCert,
	}

	err := rgw.Run(createContext(), config)
	if err != nil {
		rook.TerminateFatal(err)
	}

	return nil
}
