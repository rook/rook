/*
Copyright 2020 The Rook Authors. All rights reserved.

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

	"github.com/rook/rook/cmd/rook/rook"
	cephv1 "github.com/rook/rook/pkg/apis/ceph.rook.io/v1"
	cleanup "github.com/rook/rook/pkg/daemon/ceph/cleanup"
	"github.com/rook/rook/pkg/daemon/ceph/client"
	opcontroller "github.com/rook/rook/pkg/operator/ceph/controller"
	"github.com/rook/rook/pkg/operator/k8sutil"
	"github.com/rook/rook/pkg/util/flags"
	"github.com/spf13/cobra"
)

var (
	dataDirHostPath    string
	namespaceDir       string
	monSecret          string
	clusterFSID        string
	sanitizeMethod     string
	sanitizeDataSource string
	sanitizeIteration  int32
)

var cleanUpCmd = &cobra.Command{
	Use:   "clean",
	Short: "Starts the cleanup process",
}

var cleanUpHostCmd = &cobra.Command{
	Use:   "host",
	Short: "Starts the cleanup process on a host after the ceph cluster is deleted",
}

var cleanUpSubVolumeGroupCmd = &cobra.Command{
	// the subcommand matches CRD kind of the custom resource to be cleaned up
	Use:   "CephFilesystemSubVolumeGroup",
	Short: "Starts the cleanup process of a CephFilesystemSubVolumeGroup",
}

func init() {
	cleanUpCmd.Flags().StringVar(&dataDirHostPath, "data-dir-host-path", "", "dataDirHostPath on the node")
	cleanUpCmd.Flags().StringVar(&namespaceDir, "namespace-dir", "", "dataDirHostPath on the node")
	cleanUpHostCmd.Flags().StringVar(&monSecret, "mon-secret", "", "monitor secret from the keyring")
	cleanUpHostCmd.Flags().StringVar(&clusterFSID, "cluster-fsid", "", "ceph cluster fsid")
	cleanUpHostCmd.Flags().StringVar(&sanitizeMethod, "sanitize-method", string(cephv1.SanitizeMethodQuick), "sanitize method to use (metadata or data)")
	cleanUpHostCmd.Flags().StringVar(&sanitizeDataSource, "sanitize-data-source", string(cephv1.SanitizeDataSourceZero), "data source to sanitize the disk (zero or random)")
	cleanUpHostCmd.Flags().Int32Var(&sanitizeIteration, "sanitize-iteration", 1, "overwrite N times the disk")
	flags.SetFlagsFromEnv(cleanUpHostCmd.Flags(), rook.RookEnvVarPrefix)

	flags.SetFlagsFromEnv(cleanUpSubVolumeGroupCmd.Flags(), rook.RookEnvVarPrefix)

	cleanUpCmd.AddCommand(cleanUpHostCmd, cleanUpSubVolumeGroupCmd)

	cleanUpHostCmd.RunE = startHostCleanUp
	cleanUpSubVolumeGroupCmd.RunE = startSubVolumeGroupCleanUp
}

func startHostCleanUp(cmd *cobra.Command, args []string) error {
	rook.SetLogLevel()
	rook.LogStartupInfo(cleanUpCmd.Flags())

	ctx := cmd.Context()

	logger.Info("starting cluster clean up")
	// Delete dataDirHostPath
	if dataDirHostPath != "" {
		// Remove both dataDirHostPath and monitor store
		cleanup.StartHostPathCleanup(namespaceDir, dataDirHostPath, monSecret)
	}

	namespace := os.Getenv(k8sutil.PodNamespaceEnvVar)
	clusterInfo := client.AdminClusterInfo(ctx, namespace, "")
	clusterInfo.FSID = clusterFSID

	// Build Sanitizer
	s := cleanup.NewDiskSanitizer(createContext(),
		clusterInfo,
		&cephv1.SanitizeDisksSpec{
			Method:     cephv1.SanitizeMethodProperty(sanitizeMethod),
			DataSource: cephv1.SanitizeDataSourceProperty(sanitizeDataSource),
			Iteration:  sanitizeIteration,
		},
	)

	// Start OSD wipe process
	s.StartSanitizeDisks()

	return nil
}

func startSubVolumeGroupCleanUp(cmd *cobra.Command, args []string) error {
	rook.SetLogLevel()
	rook.LogStartupInfo(cleanUpSubVolumeGroupCmd.Flags())

	ctx := cmd.Context()
	context := createContext()
	namespace := os.Getenv(k8sutil.PodNamespaceEnvVar)
	clusterInfo := client.AdminClusterInfo(ctx, namespace, "")

	fsName := os.Getenv(opcontroller.CephFSNameEnv)
	if fsName == "" {
		rook.TerminateFatal(fmt.Errorf("ceph filesystem name is not available in the pod environment variables"))
	}
	subVolumeGroupName := os.Getenv(opcontroller.CephFSSubVolumeGroupNameEnv)
	if subVolumeGroupName == "" {
		rook.TerminateFatal(fmt.Errorf("cephFS SubVolumeGroup name is not available in the pod environment variables"))
	}
	csiNamespace := os.Getenv(opcontroller.CSICephFSRadosNamesaceEnv)
	if csiNamespace == "" {
		rook.TerminateFatal(fmt.Errorf("CSI rados namespace name is not available in the pod environment variables"))
	}
	poolName := os.Getenv(opcontroller.CephFSMetaDataPoolNameEnv)
	if poolName == "" {
		rook.TerminateFatal(fmt.Errorf("cephFS metadata pool name is not available in the pod environment variables"))
	}

	err := cleanup.SubVolumeGroupCleanup(context, clusterInfo, fsName, subVolumeGroupName, poolName, csiNamespace)
	if err != nil {
		rook.TerminateFatal(fmt.Errorf("failed to cleanup cephFS %q SubVolumeGroup %q in the namespace %q. %v", fsName, subVolumeGroupName, namespace, err))
	}

	return nil
}
