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
	"path"

	"github.com/go-ini/ini"
	"github.com/rook/rook/cmd/rook/rook"
	"github.com/rook/rook/pkg/daemon/ceph/mon"
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
	monPort int32
)

func init() {
	monCmd.Flags().StringVar(&monName, "name", "", "name of the monitor")
	monCmd.Flags().Int32Var(&monPort, "port", 0, "port of the monitor")
	addCephFlags(monCmd)

	flags.SetFlagsFromEnv(monCmd.Flags(), rook.RookEnvVarPrefix)

	monCmd.RunE = startMon
}

func startMon(cmd *cobra.Command, args []string) error {
	required := []string{"name", "fsid", "mon-secret", "admin-secret", "config-dir", "cluster-name", "public-ipv4", "private-ipv4"}
	if err := flags.VerifyRequiredFlags(monCmd, required); err != nil {
		return err
	}

	rook.SetLogLevel()

	rook.LogStartupInfo(monCmd.Flags())

	if monPort == 0 {
		return fmt.Errorf("missing mon port")
	}

	if err := compareMonSecret(clusterInfo.MonitorSecret, path.Join(cfg.dataDir, monName)); err != nil {
		rook.TerminateFatal(err)
	}

	// at first start the local monitor needs to be added to the list of mons
	clusterInfo.Monitors = mon.ParseMonEndpoints(cfg.monEndpoints)
	clusterInfo.Monitors[monName] = mon.ToCephMon(monName, cfg.networkInfo.PublicAddrIPv4, monPort)

	monCfg := &mon.Config{
		Name:    monName,
		Cluster: &clusterInfo,
		Port:    monPort,
	}
	err := mon.Run(createContext(), monCfg)
	if err != nil {
		rook.TerminateFatal(err)
	}

	return nil
}

// Compare the expected mon keyring secret with the cached keyring from a previous run of the monitor.
// If these don't match we will not want to launch the monitor.
func compareMonSecret(secret, configDir string) error {
	cachedKeyringFile := path.Join(configDir, "data", "keyring")
	if _, err := os.Stat(cachedKeyringFile); os.IsNotExist(err) {
		// the mon is starting for the first time
		logger.Infof("mon keyring is not yet cached at %s", cachedKeyringFile)
		return nil
	}

	contents, err := ini.Load(cachedKeyringFile)
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to read config file %s. %+v. Skipping check for cached keyring.\n", cachedKeyringFile, err)
		return nil
	}
	section, err := contents.GetSection("mon.")
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to find mon section in the cached keyring. %+v. Skipping check for cached keyring.\n", err)
		return nil
	}
	cachedKeyring, err := section.GetKey("key")
	if err != nil || cachedKeyring == nil {
		fmt.Fprintf(os.Stderr, "failed to find mon keyring in the cached file. %+v. Skipping check for cached keyring.\n", err)
		return nil
	}
	if cachedKeyring.Value() != secret {
		return fmt.Errorf("The keyring does not match the existing keyring in %s. You may need to delete the contents of dataDirHostPath on the host from a previous deployment.", cachedKeyringFile)
	}
	logger.Infof("cached mon secret matches the expected keyring")
	return nil
}
