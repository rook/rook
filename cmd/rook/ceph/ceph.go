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
	"os"

	"github.com/coreos/pkg/capnslog"
	"github.com/spf13/cobra"

	"github.com/rook/rook/cmd/rook/rook"
	"github.com/rook/rook/pkg/clusterd"
	cephclient "github.com/rook/rook/pkg/daemon/ceph/client"
	osdconfig "github.com/rook/rook/pkg/operator/ceph/cluster/osd/config"
	"github.com/rook/rook/pkg/operator/k8sutil"
)

// Cmd is the main command for operator and daemons.
var Cmd = &cobra.Command{
	Use:   "ceph",
	Short: "Main command for Ceph operator and daemons.",
}

var (
	cfg         = &config{}
	clusterInfo cephclient.ClusterInfo
	logger      = capnslog.NewPackageLogger("github.com/rook/rook", "cephcmd")
)

type config struct {
	devices            string
	metadataDevice     string
	dataDir            string
	forceFormat        bool
	location           string
	cephConfigOverride string
	storeConfig        osdconfig.StoreConfig
	monEndpoints       string
	nodeName           string
	pvcBacked          bool
}

func init() {
	Cmd.AddCommand(cleanUpCmd,
		operatorCmd,
		osdCmd,
		mgrCmd,
		configCmd)
}

func createContext() *clusterd.Context {
	context := rook.NewContext()
	context.ConfigDir = cfg.dataDir
	context.ConfigFileOverride = cfg.cephConfigOverride
	return context
}

func addCephFlags(command *cobra.Command) {
	command.Flags().StringVar(&clusterInfo.FSID, "fsid", "", "the cluster uuid")
	command.Flags().StringVar(&clusterInfo.MonitorSecret, "mon-secret", "", "the cephx keyring for monitors")
	command.Flags().StringVar(&clusterInfo.CephCred.Username, "ceph-username", "", "ceph username")
	command.Flags().StringVar(&clusterInfo.CephCred.Secret, "ceph-secret", "", "secret for the ceph user (random if not specified)")
	command.Flags().StringVar(&cfg.monEndpoints, "mon-endpoints", "", "ceph mon endpoints")
	command.Flags().StringVar(&cfg.dataDir, "config-dir", "/var/lib/rook", "directory for storing configuration")
	command.Flags().StringVar(&cfg.cephConfigOverride, "ceph-config-override", "", "optional path to a ceph config file that will be appended to the config files that rook generates")

	clusterInfo.Namespace = os.Getenv(k8sutil.PodNamespaceEnvVar)
}
