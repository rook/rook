/*
Copyright 2021 The Rook Authors. All rights reserved.

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
	"path"
	"time"

	"github.com/rook/rook/cmd/rook/rook"
	cephv1 "github.com/rook/rook/pkg/apis/ceph.rook.io/v1"
	"github.com/rook/rook/pkg/daemon/ceph/client"
	"github.com/rook/rook/pkg/operator/ceph/cluster/mgr"
	"github.com/rook/rook/pkg/operator/ceph/cluster/mon"
	opcontroller "github.com/rook/rook/pkg/operator/ceph/controller"
	cephver "github.com/rook/rook/pkg/operator/ceph/version"
	"github.com/rook/rook/pkg/operator/k8sutil"
	"github.com/rook/rook/pkg/util/flags"
	"github.com/spf13/cobra"
)

var mgrCmd = &cobra.Command{
	Use: "mgr",
}
var mgrSidecarCmd = &cobra.Command{
	Use: "watch-active",
}
var (
	updateMgrServicesInterval string
	daemonName                string
	clusterSpec               cephv1.ClusterSpec
	rawCephVersion            string
)

func init() {
	addCephFlags(mgrCmd)

	// add the subcommands to the parent mgr command
	mgrCmd.AddCommand(mgrSidecarCmd)

	mgrSidecarCmd.Flags().BoolVar(&clusterSpec.Dashboard.Enabled, "dashboard-enabled", false, "whether the dashboard is enabled")
	mgrSidecarCmd.Flags().BoolVar(&clusterSpec.Monitoring.Enabled, "monitoring-enabled", false, "whether the monitoring is enabled")
	mgrSidecarCmd.Flags().StringVar(&updateMgrServicesInterval, "update-interval", "", "the interval at which to update the mgr services")
	mgrSidecarCmd.Flags().StringVar(&ownerRefID, "cluster-id", "", "the UID of the cluster CR that owns this cluster")
	mgrSidecarCmd.Flags().StringVar(&clusterName, "cluster-name", "", "the name of the cluster CR that owns this cluster")
	mgrSidecarCmd.Flags().StringVar(&daemonName, "daemon-name", "", "the name of the local mgr daemon")
	mgrSidecarCmd.Flags().StringVar(&rawCephVersion, "ceph-version", "", "the version of ceph")

	flags.SetFlagsFromEnv(mgrCmd.Flags(), rook.RookEnvVarPrefix)
	flags.SetFlagsFromEnv(mgrSidecarCmd.Flags(), rook.RookEnvVarPrefix)
	mgrSidecarCmd.RunE = runMgrSidecar
}

// Start the mgr daemon sidecar
func runMgrSidecar(cmd *cobra.Command, args []string) error {
	rook.SetLogLevel()
	clusterInfo.Context = cmd.Context()

	if err := readCephSecret(path.Join(mon.CephSecretMountPath, mon.CephSecretFilename)); err != nil {
		rook.TerminateFatal(err)
	}

	context := createContext()
	clusterInfo.Monitors = opcontroller.ParseMonEndpoints(cfg.monEndpoints)
	rook.LogStartupInfo(mgrSidecarCmd.Flags())

	ownerRef := opcontroller.ClusterOwnerRef(clusterName, ownerRefID)
	clusterInfo.OwnerInfo = k8sutil.NewOwnerInfoWithOwnerRef(&ownerRef, clusterInfo.Namespace)

	if err := client.WriteCephConfig(context, &clusterInfo); err != nil {
		rook.TerminateFatal(err)
	}

	interval, err := time.ParseDuration(updateMgrServicesInterval)
	if err != nil {
		rook.TerminateFatal(err)
	}

	version, err := cephver.ExtractCephVersion(rawCephVersion)
	if err != nil {
		rook.TerminateFatal(err)
	}
	clusterInfo.CephVersion = *version

	m := mgr.New(context, &clusterInfo, clusterSpec, "")
	prevActiveMgr := "unknown"
	for {
		prevActiveMgr, err = m.UpdateActiveMgrLabel(daemonName, prevActiveMgr)
		if err != nil {
			logger.Errorf("failed to reconcile services. %v", err)
		} else {
			logger.Infof("successfully checked mgr_role label. checking again in %ds", (int)(interval.Seconds()))
		}
		time.Sleep(interval)
	}
}
