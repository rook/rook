// +build linux,amd64 linux,arm64

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

	"github.com/rook/rook/pkg/cephmgr/cephd"
	"github.com/rook/rook/pkg/cephmgr/mon"
	"github.com/rook/rook/pkg/cephmgr/rgw"
	"github.com/rook/rook/pkg/clusterd"
	"github.com/rook/rook/pkg/util/exec"
	"github.com/rook/rook/pkg/util/flags"
	"github.com/spf13/cobra"
)

var rgwCmd = &cobra.Command{
	Use:    "rgw",
	Short:  "Generates rgw config and runs the rgw daemon",
	Hidden: true,
}

var (
	rgwCluster mon.ClusterInfo
	rgwKeyring string
	rgwHost    string
	rgwPort    int
)

func init() {
	rgwCmd.Flags().StringVar(&rgwKeyring, "rgw-keyring", "", "the rgw keyring")
	rgwCmd.Flags().StringVar(&osdCluster.MonitorSecret, "rgw-mon-secret", "", "the monitor secret")
	rgwCmd.Flags().StringVar(&osdCluster.AdminSecret, "rgw-admin-secret", "", "the admin secret")
	rgwCmd.Flags().StringVar(&rgwHost, "rgw-host", "", "dns host name")
	rgwCmd.Flags().IntVar(&rgwPort, "rgw-port", 0, "rgw port number")

	rgwCmd.RunE = startRGW
}

func startRGW(cmd *cobra.Command, args []string) error {
	if err := flags.VerifyRequiredFlags(rgwCmd, []string{"mon-endpoints", "cluster-name", "rgw-host", "rgw-mon-secret", "rgw-admin-secret", "rgw-keyring"}); err != nil {
		return err
	}

	if rgwPort == 0 {
		return fmt.Errorf("port is required")
	}

	setLogLevel()

	rgwCluster.Monitors = mon.ParseMonEndpoints(cfg.monEndpoints)
	rgwCluster.Name = cfg.clusterName
	config := &rgw.Config{
		ClusterInfo:  &rgwCluster,
		CephLauncher: cephd.New(),
		Keyring:      rgwKeyring,
		Host:         rgwHost,
		Port:         rgwPort,
		InProc:       true,
	}

	executor := &exec.CommandExecutor{}
	context := &clusterd.DaemonContext{
		Executor:           executor,
		ConfigDir:          cfg.dataDir,
		ConfigFileOverride: cfg.cephConfigOverride,
		LogLevel:           cfg.logLevel,
	}

	return rgw.Run(context, config)
}
