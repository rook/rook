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
	"github.com/rook/rook/pkg/util/flags"
	"github.com/spf13/cobra"
)

var monCmd = &cobra.Command{
	Use:    "mon",
	Short:  "Generates mon config and runs the mon daemon",
	Hidden: true,
}

var (
	monName string
	monPort int
)

func addCephFlags(command *cobra.Command) {
	command.Flags().StringVar(&cfg.networkInfo.PublicAddrIPv4, "public-ipv4", "127.0.0.1", "public IPv4 address for this machine")
	command.Flags().StringVar(&cfg.networkInfo.ClusterAddrIPv4, "private-ipv4", "127.0.0.1", "private IPv4 address for this machine")
	command.Flags().StringVar(&clusterInfo.Name, "cluster-name", "rookcluster", "ceph cluster name")
	command.Flags().StringVar(&clusterInfo.FSID, "fsid", "", "the cluster uuid")
	command.Flags().StringVar(&clusterInfo.MonitorSecret, "mon-secret", "", "the cephx keyring for monitors")
	command.Flags().StringVar(&clusterInfo.AdminSecret, "admin-secret", "", "secret for the admin user (random if not specified)")
	command.Flags().StringVar(&cfg.monEndpoints, "mon-endpoints", "", "ceph mon endpoints")
	command.Flags().StringVar(&cfg.dataDir, "config-dir", "/var/lib/rook", "directory for storing configuration")
	command.Flags().StringVar(&cfg.cephConfigOverride, "ceph-config-override", "", "optional path to a ceph config file that will be appended to the config files that rook generates")
}

func init() {
	monCmd.Flags().StringVar(&monName, "name", "", "name of the monitor")
	monCmd.Flags().IntVar(&monPort, "port", 0, "port of the monitor")
	addCephFlags(monCmd)

	flags.SetFlagsFromEnv(monCmd.Flags(), "ROOKD")

	monCmd.RunE = startMon
}

func startMon(cmd *cobra.Command, args []string) error {
	if err := flags.VerifyRequiredFlags(monCmd, []string{"name", "fsid", "mon-secret", "admin-secret", "config-dir", "cluster-name", "private-ipv4"}); err != nil {
		return err
	}

	setLogLevel()

	if monPort == 0 {
		return fmt.Errorf("missing mon port")
	}

	// at first start the local monitor needs to be added to the list of mons
	clusterInfo.Monitors = mon.ParseMonEndpoints(cfg.monEndpoints)
	clusterInfo.Monitors[monName] = mon.ToCephMon(monName, cfg.networkInfo.ClusterAddrIPv4)

	monCfg := &mon.Config{Name: monName, Cluster: &clusterInfo}
	err := mon.Run(createDaemonContext(), monCfg)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}

	return nil
}
