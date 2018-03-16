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
	"flag"
	"fmt"
	"os"
	"strings"

	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"

	"github.com/coreos/pkg/capnslog"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"

	rookalpha "github.com/rook/rook/pkg/apis/rook.io/v1alpha1"
	rookclient "github.com/rook/rook/pkg/client/clientset/versioned"
	"github.com/rook/rook/pkg/clusterd"
	"github.com/rook/rook/pkg/daemon/ceph/mon"
	"github.com/rook/rook/pkg/util/exec"
	"github.com/rook/rook/pkg/util/flags"
	"github.com/rook/rook/pkg/version"
	apiextensionsclient "k8s.io/apiextensions-apiserver/pkg/client/clientset/clientset"
)

const (
	RookEnvVarPrefix = "ROOK"
	terminationLog   = "/dev/termination-log"
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
	storeConfig        rookalpha.StoreConfig
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
}

func addCommands() {
	rootCmd.AddCommand(versionCmd)
	rootCmd.AddCommand(monCmd)
	rootCmd.AddCommand(osdCmd)
	rootCmd.AddCommand(mgrCmd)
	rootCmd.AddCommand(rgwCmd)
	rootCmd.AddCommand(mdsCmd)
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
	// workaround a k8s logging issue: https://github.com/kubernetes/kubernetes/issues/17162
	flag.CommandLine.Parse([]string{})

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
		ConfigDir:          cfg.dataDir,
		ConfigFileOverride: cfg.cephConfigOverride,
		LogLevel:           cfg.logLevel,
		NetworkInfo:        cfg.networkInfo,
	}
}

func getClientset() (kubernetes.Interface, apiextensionsclient.Interface, rookclient.Interface, error) {
	// create the k8s client
	config, err := rest.InClusterConfig()
	if err != nil {
		return nil, nil, nil, fmt.Errorf("failed to get k8s config. %+v", err)
	}

	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("failed to create k8s clientset. %+v", err)
	}
	apiExtClientset, err := apiextensionsclient.NewForConfig(config)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("failed to create k8s API extension clientset. %+v", err)
	}
	rookClientset, err := rookclient.NewForConfig(config)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("failed to create rook clientset. %+v", err)
	}
	return clientset, apiExtClientset, rookClientset, nil
}

func terminateFatal(reason error) {
	fmt.Fprintln(os.Stderr, reason)

	file, err := os.OpenFile(terminationLog, os.O_APPEND|os.O_WRONLY, 0644)
	if err != nil {
		fmt.Fprintln(os.Stderr, fmt.Errorf("failed to write message to termination log: %+v", err))
	} else {
		defer file.Close()
		if _, err = file.WriteString(reason.Error()); err != nil {
			fmt.Fprintln(os.Stderr, fmt.Errorf("failed to write message to termination log: %+v", err))
		}
	}

	os.Exit(1)
}
