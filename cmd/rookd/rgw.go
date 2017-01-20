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
	"strings"

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
	keyring string
	host    string
	port    int
)

func init() {
	rgwCmd.Flags().StringVar(&keyring, "keyring", "", "keyring for connecting rgw to the cluster")
	rgwCmd.Flags().StringVar(&host, "host", "", "dns host name")
	rgwCmd.Flags().IntVar(&port, "port", 0, "rgw port number")

	rgwCmd.RunE = startRGW
}

func startRGW(cmd *cobra.Command, args []string) error {
	if err := flags.VerifyRequiredFlags(rgwCmd, []string{"mons", "keyring", "host"}); err != nil {
		return err
	}

	if port == 0 {
		return fmt.Errorf("port is required")
	}

	setLogLevel()

	config := &rgw.Config{
		ClusterInfo:  &mon.ClusterInfo{Name: cfg.clusterName, Monitors: parseMonitors(cfg.monEndpoints)},
		CephLauncher: cephd.New(),
		Keyring:      keyring,
		Host:         host,
		Port:         port,
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

func parseMonitors(mons string) map[string]*mon.CephMonitorConfig {
	result := map[string]*mon.CephMonitorConfig{}
	endpoints := strings.Split(mons, ",")
	for _, endpoint := range endpoints {
		parts := strings.Split(endpoint, "-")
		if len(parts) != 2 {
			logger.Warningf("invalid monitor format: %s", endpoint)
			continue
		}

		name := parts[0]
		endpoint := parts[1]
		result[name] = &mon.CephMonitorConfig{Name: name, Endpoint: endpoint}
	}
	return result
}
