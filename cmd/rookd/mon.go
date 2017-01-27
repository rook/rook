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
	"os"

	"github.com/rook/rook/pkg/cephmgr/cephd"
	"github.com/rook/rook/pkg/cephmgr/mon"
	"github.com/rook/rook/pkg/clusterd"
	"github.com/rook/rook/pkg/util/exec"
	"github.com/rook/rook/pkg/util/flags"
	"github.com/rook/rook/pkg/util/proc"
	"github.com/spf13/cobra"
)

var monCmd = &cobra.Command{
	Use:    "mon",
	Short:  "Generates mon config and runs the mon daemon",
	Hidden: true,
}

var (
	cluster mon.ClusterInfo
	monName string
	monPort int
)

func init() {
	monCmd.Flags().StringVar(&monName, "name", "", "name of the monitor")
	monCmd.Flags().StringVar(&cluster.FSID, "fsid", "", "keyring for secure monitors")
	monCmd.Flags().StringVar(&cluster.MonitorSecret, "mon-secret", "", "keyring for secure monitors")
	monCmd.Flags().StringVar(&cluster.AdminSecret, "admin-secret", "", "keyring for secure monitors")
	monCmd.Flags().IntVar(&monPort, "port", 0, "port of the monitor")

	monCmd.RunE = startMon
}

func startMon(cmd *cobra.Command, args []string) error {
	if err := flags.VerifyRequiredFlags(monCmd, []string{"name", "fsid", "mon-secret", "admin-secret", "data-dir", "cluster-name"}); err != nil {
		return err
	}

	setLogLevel()

	ipaddress := os.Getenv(mon.IPAddressEnvVar)
	if ipaddress == "" {
		return fmt.Errorf("missing pod ip address for the monitor")
	}

	if monPort == 0 {
		return fmt.Errorf("missing mon port")
	}

	cluster.Name = cfg.clusterName

	// at first start the local monitor needs to be added to the list of mons
	cluster.Monitors = mon.ParseMonEndpoints(cfg.monEndpoints)
	cluster.Monitors[monName] = mon.ToCephMon(monName, ipaddress)

	monCfg := &mon.Config{Name: monName, Cluster: &cluster, CephLauncher: cephd.New()}

	executor := &exec.CommandExecutor{}
	context := &clusterd.DaemonContext{
		ProcMan:            proc.New(executor),
		Executor:           executor,
		ConfigDir:          cfg.dataDir,
		ConfigFileOverride: cfg.cephConfigOverride,
		LogLevel:           cfg.logLevel,
	}

	return mon.Run(context, monCfg)
}
