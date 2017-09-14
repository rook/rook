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
	"strings"

	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"

	"github.com/coreos/pkg/capnslog"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"

	"github.com/rook/rook/pkg/ceph/mon"
	"github.com/rook/rook/pkg/ceph/osd"
	"github.com/rook/rook/pkg/clusterd"
	"github.com/rook/rook/pkg/util/exec"
	"github.com/rook/rook/pkg/util/flags"
	"github.com/rook/rook/pkg/util/proc"
	"github.com/rook/rook/pkg/version"
	apiextensionsclient "k8s.io/apiextensions-apiserver/pkg/client/clientset/clientset"
)

const (
	RookEnvVarPrefix = "ROOK"
)

var rootCmd = &cobra.Command{
	Use:    "rook",
	Hidden: true,
}
var cfg = &config{}
var clusterInfo mon.ClusterInfo

var logLevelRaw string
var logger = capnslog.NewPackageLogger("github.com/rook/rook", "rook")

type config struct {
	nodeID             string
	discoveryURL       string
	etcdMembers        string
	devices            string
	directories        string
	metadataDevice     string
	dataDir            string
	forceFormat        bool
	location           string
	logLevel           capnslog.LogLevel
	cephConfigOverride string
	storeConfig        osd.StoreConfig
	networkInfo        clusterd.NetworkInfo
	monEndpoints       string
	nodeName           string
}

func main() {
	addCommands()

	if err := rootCmd.Execute(); err != nil {
		fmt.Printf("rook error: %+v\n", err)
	}
}

// Initialize the configuration parameters. The precedence from lowest to highest is:
//  1) default value (at compilation)
//  2) environment variables (upper case, replace - with _, and rook prefix. For example, discovery-url is ROOK_DISCOVERY_URL)
//  3) command line parameter
func init() {
	rootCmd.PersistentFlags().StringVar(&logLevelRaw, "log-level", "INFO", "logging level for logging/tracing output (valid values: CRITICAL,ERROR,WARNING,NOTICE,INFO,DEBUG,TRACE)")

	// load the environment variables
	flags.SetFlagsFromEnv(rootCmd.Flags(), RookEnvVarPrefix)
	flags.SetFlagsFromEnv(rootCmd.PersistentFlags(), RookEnvVarPrefix)

	rootCmd.Run = func(cmd *cobra.Command, args []string) {
		fmt.Fprintln(os.Stderr, "Rook standalone support has been removed")
		os.Exit(1)
	}
}

func addCommands() {
	rootCmd.AddCommand(versionCmd)
	rootCmd.AddCommand(monCmd)
	rootCmd.AddCommand(osdCmd)
	rootCmd.AddCommand(mgrCmd)
	rootCmd.AddCommand(rgwCmd)
	rootCmd.AddCommand(mdsCmd)
	rootCmd.AddCommand(apiCmd)
	rootCmd.AddCommand(agentCmd)
	rootCmd.AddCommand(operatorCmd)
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

func logStartupInfo(cmdFlags *pflag.FlagSet) {
	// log the version number, arguments, and all final flag values (environment variable overrides
	// have already been taken into account)
	flagValues := flags.GetFlagsAndValues(cmdFlags, "secret")
	logger.Infof("starting Rook %s with arguments '%s'", version.Version, strings.Join(os.Args, " "))
	logger.Infof("flag values: %s", strings.Join(flagValues, ", "))
}

func createContext() *clusterd.Context {
	executor := &exec.CommandExecutor{}
	return &clusterd.Context{
		Executor:           executor,
		ProcMan:            proc.New(executor),
		ConfigDir:          cfg.dataDir,
		ConfigFileOverride: cfg.cephConfigOverride,
		LogLevel:           cfg.logLevel,
		NetworkInfo:        cfg.networkInfo,
	}
}

func getClientset() (kubernetes.Interface, apiextensionsclient.Interface, error) {
	// create the k8s client
	config, err := rest.InClusterConfig()
	if err != nil {
		return nil, nil, fmt.Errorf("failed to get k8s config. %+v", err)
	}

	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to create k8s clientset. %+v", err)
	}
	apiExtClientset, err := apiextensionsclient.NewForConfig(config)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to create k8s API extension clientset. %+v", err)
	}
	return clientset, apiExtClientset, nil
}
