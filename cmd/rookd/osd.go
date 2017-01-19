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
	"github.com/rook/rook/pkg/cephmgr/cephd"
	"github.com/rook/rook/pkg/cephmgr/mon"
	"github.com/rook/rook/pkg/cephmgr/osd"
	"github.com/rook/rook/pkg/cephmgr/osd/partition"
	"github.com/rook/rook/pkg/clusterd"
	"github.com/rook/rook/pkg/util/exec"
	"github.com/rook/rook/pkg/util/flags"
	"github.com/rook/rook/pkg/util/proc"
	"github.com/spf13/cobra"
)

var osdCmd = &cobra.Command{
	Use:    "osd",
	Short:  "Generates osd config and runs the osd daemon",
	Hidden: true,
}
var (
	osdCluster   mon.ClusterInfo
	monEndpoints string
)

func init() {
	osdCmd.Flags().StringVar(&monEndpoints, "osd-mon-endpoints", "", "monitor endpoints for the osds to connect")
	osdCmd.Flags().StringVar(&osdCluster.MonitorSecret, "osd-mon-secret", "", "keyring for secure monitors")
	osdCmd.Flags().StringVar(&osdCluster.AdminSecret, "osd-admin-secret", "", "keyring for secure monitors")

	osdCmd.RunE = startOSD
}

func startOSD(cmd *cobra.Command, args []string) error {
	if err := flags.VerifyRequiredFlags(osdCmd, []string{"cluster-name", "osd-mon-endpoints", "osd-mon-secret", "osd-admin-secret"}); err != nil {
		return err
	}

	setLogLevel()

	devices := ""
	metadataDevice := ""
	forceFormat := false
	location := ""
	bluestoreConfig := partition.BluestoreConfig{DatabaseSizeMB: 512} //FIX
	osdCluster.Monitors = mon.ParseMonEndpoints(monEndpoints)
	osdCluster.Name = cfg.clusterName
	agent := osd.NewAgent(cephd.New(), devices, metadataDevice, forceFormat, location, bluestoreConfig, &osdCluster)

	executor := &exec.CommandExecutor{}
	context := &clusterd.DaemonContext{
		ProcMan:            proc.New(executor),
		Executor:           executor,
		ConfigDir:          cfg.dataDir,
		ConfigFileOverride: cfg.cephConfigOverride,
		LogLevel:           cfg.logLevel,
	}

	return osd.Run(context, agent)
}
