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

func init() {
	osdCmd.RunE = startOSD
}

func startOSD(cmd *cobra.Command, args []string) error {
	if err := flags.VerifyRequiredFlags(osdCmd, []string{"cluster-name", "mon-endpoints", "mon-secret", "admin-secret"}); err != nil {
		return err
	}

	setLogLevel()

	devices := ""
	metadataDevice := ""
	forceFormat := false
	location := ""
	bluestoreConfig := partition.BluestoreConfig{DatabaseSizeMB: 512} // FIX
	clusterInfo.Monitors = mon.ParseMonEndpoints(cfg.monEndpoints)
	agent := osd.NewAgent(cephd.New(), devices, metadataDevice, forceFormat, location, bluestoreConfig, &clusterInfo)
	context := clusterd.NewDaemonContext(cfg.dataDir, cfg.cephConfigOverride, cfg.logLevel)

	return osd.Run(context, agent)
}
