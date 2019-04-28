/*
Copyright 2018 The Rook Authors. All rights reserved.

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

	"github.com/rook/rook/cmd/rook/rook"
	cephconfig "github.com/rook/rook/pkg/daemon/ceph/config"
	"github.com/rook/rook/pkg/operator/ceph/cluster/mon"
	"github.com/rook/rook/pkg/util"
	"github.com/rook/rook/pkg/util/flags"
	"github.com/spf13/cobra"
)

const (
	configKeyringTemplate = `
[%s]
key = %s
`
)

var configCmd = &cobra.Command{
	Use:   "config-init",
	Short: "Generates basic ceph config",
}

var (
	configKeyring  string
	configUsername string
)

func init() {
	configCmd.Flags().StringVar(&configKeyring, "keyring", "", "the daemon keyring")
	configCmd.Flags().StringVar(&configUsername, "username", "", "the daemon username")
	addCephFlags(configCmd)

	flags.SetFlagsFromEnv(configCmd.Flags(), rook.RookEnvVarPrefix)

	configCmd.RunE = initConfig
}

func initConfig(cmd *cobra.Command, args []string) error {
	required := []string{
		"username", "keyring", "mon-endpoints"}
	if err := flags.VerifyRequiredFlags(configCmd, required); err != nil {
		return err
	}

	if err := verifyRenamedFlags(configCmd); err != nil {
		return err
	}

	rook.SetLogLevel()

	rook.LogStartupInfo(configCmd.Flags())

	clusterInfo.Monitors = mon.ParseMonEndpoints(cfg.monEndpoints)
	clusterInfo.Name = "ceph"
	context := createContext()

	keyringPath := "/etc/ceph/keyring"
	_, err := cephconfig.GenerateConfigFile(context, &clusterInfo, "/etc/ceph", configUsername, keyringPath, nil, nil)
	if err != nil {
		return fmt.Errorf("failed to create config file: %+v", err)
	}

	util.WriteFileToLog(logger, cephconfig.DefaultConfigFilePath())

	keyringEval := func(key string) string {
		r := fmt.Sprintf(configKeyringTemplate, configUsername, key)
		return r
	}

	err = cephconfig.WriteKeyring(keyringPath, configKeyring, keyringEval)
	if err != nil {
		return fmt.Errorf("failed to create keyring: %+v\n", err)
	}
	if err != nil {
		rook.TerminateFatal(err)
	}

	return nil
}
