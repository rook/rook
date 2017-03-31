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
	"github.com/rook/rook/pkg/util/flags"
	"github.com/spf13/cobra"
)

var osdCmd = &cobra.Command{
	Use:    "osd",
	Short:  "Generates osd config and runs the osd daemon",
	Hidden: true,
}
var (
	osdCluster mon.ClusterInfo
)

func addOSDFlags(command *cobra.Command) {
	command.Flags().StringVar(&cfg.location, "location", "", "location of this node for CRUSH placement")
	command.Flags().StringVar(&cfg.devices, "data-devices", "", "comma separated list of devices to use for storage, or \"all\"")
	command.Flags().StringVar(&cfg.metadataDevice, "metadata-device", "", "device to use for metadata (e.g. a high performance SSD/NVMe device)")
	command.Flags().BoolVar(&cfg.forceFormat, "force-format", false,
		"true to force the format of any specified devices, even if they already have a filesystem.  BE CAREFUL!")
	// bluestore config flags
	command.Flags().IntVar(&cfg.bluestoreConfig.WalSizeMB, "osd-wal-size", partition.WalDefaultSizeMB, "default size (MB) for OSD write ahead log (WAL)")
	command.Flags().IntVar(&cfg.bluestoreConfig.DatabaseSizeMB, "osd-database-size", partition.DBDefaultSizeMB, "default size (MB) for OSD database")
}

func init() {
	addOSDFlags(osdCmd)
	addCephFlags(osdCmd)
	flags.SetFlagsFromEnv(monCmd.Flags(), "ROOKD")

	osdCmd.RunE = startOSD
}

func startOSD(cmd *cobra.Command, args []string) error {
	if err := flags.VerifyRequiredFlags(osdCmd, []string{"cluster-name", "mon-endpoints", "mon-secret", "admin-secret"}); err != nil {
		return err
	}

	setLogLevel()

	metadataDevice := ""
	forceFormat := false
	location := ""
	clusterInfo.Monitors = mon.ParseMonEndpoints(cfg.monEndpoints)
	agent := osd.NewAgent(cephd.New(), cfg.devices, metadataDevice, forceFormat, location, cfg.bluestoreConfig, &clusterInfo)
	context := clusterd.NewDaemonContext(cfg.dataDir, cfg.cephConfigOverride, cfg.logLevel)

	return osd.Run(context, agent)
}
