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
	"io/ioutil"
	"os"

	"github.com/pkg/errors"
	"github.com/rook/rook/cmd/rook/rook"
	cephclient "github.com/rook/rook/pkg/daemon/ceph/client"
	"github.com/rook/rook/pkg/util"
	"github.com/spf13/cobra"
)

var configCmd = &cobra.Command{
	Use:   "config-init",
	Short: "Generates basic Ceph config",
	Long: `Generate the most basic Ceph config for connecting non-Ceph daemons to a Ceph
cluster (e.g., nfs-ganesha). Effectively what this means is that it generates
'/etc/ceph/ceph.conf' with 'mon_host' populated and a keyring path (given via
commandline flag) associated with the user given via commandline flag.
'mon_host' is determined by the 'ROOK_CEPH_MON_HOST' env var present in other
Ceph daemon pods, and the keyring is expected to be mounted into the container
with a Kubernetes pod volume+mount.`,
}

var (
	keyring  string
	username string
)

func init() {
	configCmd.Flags().StringVar(&keyring, "keyring", "", "path to the keyring file")
	if err := configCmd.MarkFlagRequired("keyring"); err != nil {
		panic(err)
	}

	configCmd.Flags().StringVar(&username, "username", "", "the daemon username")
	if err := configCmd.MarkFlagRequired("username"); err != nil {
		panic(err)
	}

	configCmd.RunE = initConfig
}

func initConfig(cmd *cobra.Command, args []string) error {
	rook.SetLogLevel()

	rook.LogStartupInfo(configCmd.Flags())

	if keyring == "" {
		rook.TerminateFatal(errors.New("keyring is empty string"))
	}
	if username == "" {
		rook.TerminateFatal(errors.New("username is empty string"))
	}

	monHost := os.Getenv("ROOK_CEPH_MON_HOST")
	if monHost == "" {
		rook.TerminateFatal(errors.New("ROOK_CEPH_MON_HOST is not set or is empty string"))
	}

	cfg := `
[global]
mon_host = ` + monHost + `

[` + username + `]
keyring = ` + keyring + `
`

	var fileMode os.FileMode = 0444 // read-only
	err := ioutil.WriteFile(cephclient.DefaultConfigFilePath(), []byte(cfg), fileMode)
	if err != nil {
		rook.TerminateFatal(errors.Wrapf(err, "failed to write config file"))
	}

	util.WriteFileToLog(logger, cephclient.DefaultConfigFilePath())

	return nil
}
