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
	"strings"

	"github.com/rook/rook/cmd/rook/rook"
	"github.com/rook/rook/pkg/daemon/ceph/client"
	"github.com/rook/rook/pkg/daemon/ceph/mon"
	"github.com/rook/rook/pkg/daemon/ceph/osd"
	"github.com/rook/rook/pkg/operator/ceph/cluster"
	oposd "github.com/rook/rook/pkg/operator/ceph/cluster/osd"
	osdcfg "github.com/rook/rook/pkg/operator/ceph/cluster/osd/config"
	"github.com/rook/rook/pkg/operator/k8sutil"
	"github.com/rook/rook/pkg/util/flags"
	"github.com/spf13/cobra"
)

var osdCmd = &cobra.Command{
	Use:    "osd",
	Short:  "Generates osd config and runs the osd daemon",
	Hidden: true,
}
var (
	osdDataDeviceFilter string
	ownerRefID          string
)

func addOSDFlags(command *cobra.Command) {
	command.Flags().StringVar(&cfg.devices, "data-devices", "", "comma separated list of devices to use for storage")
	command.Flags().StringVar(&ownerRefID, "cluster-id", "", "the UID of the cluster CRD that owns this cluster")
	command.Flags().StringVar(&osdDataDeviceFilter, "data-device-filter", "", "a regex filter for the device names to use, or \"all\"")
	command.Flags().StringVar(&cfg.directories, "data-directories", "", "comma separated list of directory paths to use for storage")
	command.Flags().StringVar(&cfg.metadataDevice, "metadata-device", "", "device to use for metadata (e.g. a high performance SSD/NVMe device)")
	command.Flags().StringVar(&cfg.location, "location", "", "location of this node for CRUSH placement")
	command.Flags().BoolVar(&cfg.forceFormat, "force-format", false,
		"true to force the format of any specified devices, even if they already have a filesystem.  BE CAREFUL!")
	command.Flags().StringVar(&cfg.nodeName, "node-name", os.Getenv("HOSTNAME"), "the host name of the node")

	// OSD store config flags
	command.Flags().IntVar(&cfg.storeConfig.WalSizeMB, "osd-wal-size", osdcfg.WalDefaultSizeMB, "default size (MB) for OSD write ahead log (WAL) (bluestore)")
	command.Flags().IntVar(&cfg.storeConfig.DatabaseSizeMB, "osd-database-size", osdcfg.DBDefaultSizeMB, "default size (MB) for OSD database (bluestore)")
	command.Flags().IntVar(&cfg.storeConfig.JournalSizeMB, "osd-journal-size", osdcfg.JournalDefaultSizeMB, "default size (MB) for OSD journal (filestore)")
	command.Flags().StringVar(&cfg.storeConfig.StoreType, "osd-store", "", "type of backing OSD store to use (bluestore or filestore)")
}

func init() {
	addOSDFlags(osdCmd)
	addCephFlags(osdCmd)
	flags.SetFlagsFromEnv(osdCmd.Flags(), rook.RookEnvVarPrefix)

	osdCmd.RunE = startOSD
}

func startOSD(cmd *cobra.Command, args []string) error {
	required := []string{"cluster-name", "cluster-id", "mon-endpoints", "mon-secret", "admin-secret", "node-name", "public-ipv4", "private-ipv4"}
	if err := flags.VerifyRequiredFlags(osdCmd, required); err != nil {
		return err
	}

	var dataDevices string
	var usingDeviceFilter bool
	if osdDataDeviceFilter != "" {
		if cfg.devices != "" {
			return fmt.Errorf("Only one of --data-devices and --data-device-filter can be specified.")
		}

		dataDevices = osdDataDeviceFilter
		usingDeviceFilter = true
	} else {
		dataDevices = cfg.devices
	}

	rook.SetLogLevel()

	rook.LogStartupInfo(osdCmd.Flags())

	clientset, _, rookClientset, err := rook.GetClientset()
	if err != nil {
		rook.TerminateFatal(fmt.Errorf("failed to init k8s client. %+v\n", err))
	}

	context := createContext()
	context.Clientset = clientset
	context.RookClientset = rookClientset

	locArgs, err := client.FormatLocation(cfg.location, cfg.nodeName)
	if err != nil {
		rook.TerminateFatal(fmt.Errorf("invalid location. %+v\n", err))
	}
	crushLocation := strings.Join(locArgs, " ")

	forceFormat := false
	clusterInfo.Monitors = mon.ParseMonEndpoints(cfg.monEndpoints)
	ownerRef := cluster.ClusterOwnerRef(clusterInfo.Name, ownerRefID)
	kv := k8sutil.NewConfigMapKVStore(clusterInfo.Name, clientset, ownerRef)
	agent := osd.NewAgent(context, dataDevices, usingDeviceFilter, cfg.metadataDevice, cfg.directories, forceFormat,
		crushLocation, cfg.storeConfig, &clusterInfo, cfg.nodeName, kv)

	err = osd.Run(context, agent, nil)
	if err != nil {
		// something failed in the OSD orchestration, update the status map with failure details
		status := oposd.OrchestrationStatus{
			Status:  oposd.OrchestrationStatusFailed,
			Message: err.Error(),
		}
		oposd.UpdateOrchestrationStatusMap(clientset, clusterInfo.Name, cfg.nodeName, status)

		rook.TerminateFatal(err)
	}

	return nil
}
