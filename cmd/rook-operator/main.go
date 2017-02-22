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
	"os"

	"github.com/coreos/pkg/capnslog"
	"github.com/rook/rook/pkg/cephmgr/cephd"
	"github.com/rook/rook/pkg/cephmgr/mon"
	"github.com/rook/rook/pkg/operator"
	"github.com/rook/rook/pkg/operator/k8sutil"
	"github.com/rook/rook/pkg/util/flags"
	"github.com/spf13/cobra"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
)

var rootCmd = &cobra.Command{
	Use:   "rook-operator",
	Short: "rook-operator tool for running rook storage in a kubernetes cluster",
	Long: `
Tool for running the rook storage components in a kubernetes cluster.
https://github.com/rook/rook`,
}

var cfg = &config{}

type config struct {
	containerVersion   string
	logLevel           capnslog.LogLevel
	dataDir            string
	cephConfigOverride string
	clusterInfo        mon.ClusterInfo
	monEndpoints       string
	useAllDevices      bool
}

var logLevelRaw string
var logger = capnslog.NewPackageLogger("github.com/rook/rook-operator", "rook-operator")

func main() {
	addCommands()

	if err := rootCmd.Execute(); err != nil {
		fmt.Printf("rookd error: %+v\n", err)
	}
}

func addCommands() {
	rootCmd.AddCommand(apiCmd)
	rootCmd.AddCommand(toolCmd)
}

func init() {
	rootCmd.PersistentFlags().StringVar(&cfg.containerVersion, "container-version", "latest", "version of the rook container to launch")
	rootCmd.PersistentFlags().StringVar(&cfg.clusterInfo.Name, "cluster-name", "rookcluster", "ceph cluster name")
	rootCmd.PersistentFlags().StringVar(&cfg.monEndpoints, "mon-endpoints", "", "ceph mon endpoints")
	rootCmd.PersistentFlags().StringVar(&cfg.clusterInfo.MonitorSecret, "mon-secret", "", "the cephx keyring for monitors")
	rootCmd.PersistentFlags().StringVar(&cfg.clusterInfo.AdminSecret, "admin-secret", "", "secret for the admin user (random if not specified)")
	rootCmd.PersistentFlags().StringVar(&cfg.dataDir, "data-dir", "/var/lib/rook", "directory for storing configuration")
	rootCmd.PersistentFlags().StringVar(&logLevelRaw, "log-level", "INFO", "logging level for logging/tracing output (valid values: CRITICAL,ERROR,WARNING,NOTICE,INFO,DEBUG,TRACE)")

	rootCmd.Flags().BoolVar(&cfg.useAllDevices, "use-all-devices", false, "true to use all storage devices, false to require local device selection")

	// load the environment variables
	flags.SetFlagsFromEnv(rootCmd.Flags(), "ROOK_OPERATOR")
	flags.SetFlagsFromEnv(rootCmd.PersistentFlags(), "ROOK_OPERATOR")

	rootCmd.RunE = startOperator
}

func startOperator(cmd *cobra.Command, args []string) error {
	// verify required flags
	if err := flags.VerifyRequiredFlags(cmd, []string{}); err != nil {
		return err
	}

	setLogLevel()

	clientset, err := getClientset()
	if err != nil {
		fmt.Printf("failed to get k8s client. %+v", err)
		os.Exit(1)
	}

	logger.Infof("starting operator. containerVersion=%s", cfg.containerVersion)
	op := operator.New(k8sutil.Namespace, cephd.New(), clientset, cfg.containerVersion, cfg.useAllDevices)
	err = op.Run()
	if err != nil {
		fmt.Printf("failed to run operator. %+v\n", err)
		os.Exit(1)
	}

	return nil
}

func setLogLevel() {
	// parse given log level string then set up corresponding global logging level
	ll, err := capnslog.ParseLevel(logLevelRaw)
	if err != nil {
		logger.Warningf("failed to set log level %s. %+v", logLevelRaw, err)
	}
	cfg.logLevel = ll
	capnslog.SetGlobalLogLevel(cfg.logLevel)
}

func getClientset() (*kubernetes.Clientset, error) {
	// create the k8s client
	config, err := rest.InClusterConfig()
	if err != nil {
		return nil, fmt.Errorf("failed to get k8s config. %+v", err)
	}

	return kubernetes.NewForConfig(config)
}
