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
	"io/ioutil"
	"net/http"
	"os"
	"os/signal"
	"path"
	"path/filepath"
	"strings"
	"time"

	"github.com/coreos/pkg/capnslog"
	"github.com/spf13/cobra"

	"github.com/rook/rook/pkg/api"
	"github.com/rook/rook/pkg/ceph"
	"github.com/rook/rook/pkg/ceph/mon"
	"github.com/rook/rook/pkg/ceph/osd"
	"github.com/rook/rook/pkg/clusterd"
	"github.com/rook/rook/pkg/model"
	"github.com/rook/rook/pkg/util"
	"github.com/rook/rook/pkg/util/exec"
	"github.com/rook/rook/pkg/util/flags"
	"github.com/rook/rook/pkg/util/proc"
)

var rootCmd = &cobra.Command{
	Use:   "rookd",
	Short: "rookd tool for bootstrapping and running rook storage",
	Long: `
Tool for bootstrapping and running the rook storage daemon.
https://github.com/rook/rook`,
}
var cfg = &config{}
var clusterInfo mon.ClusterInfo

var logLevelRaw string
var logger = capnslog.NewPackageLogger("github.com/rook/rook", "rookd")

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
}

func main() {
	addCommands()

	if err := rootCmd.Execute(); err != nil {
		fmt.Printf("rookd error: %+v\n", err)
	}
}

// Initialize the configuration parameters. The precedence from lowest to highest is:
//  1) default value (at compilation)
//  2) environment variables (upper case, replace - with _, and rook prefix. For example, discovery-url is ROOKD_DISCOVERY_URL)
//  3) command line parameter
func init() {
	addStandaloneRookFlags(rootCmd)
	rootCmd.PersistentFlags().StringVar(&logLevelRaw, "log-level", "INFO", "logging level for logging/tracing output (valid values: CRITICAL,ERROR,WARNING,NOTICE,INFO,DEBUG,TRACE)")

	// load the environment variables
	flags.SetFlagsFromEnv(rootCmd.Flags(), "ROOKD")
	flags.SetFlagsFromEnv(rootCmd.PersistentFlags(), "ROOKD")

	rootCmd.RunE = startJoinCluster
}

func addCommands() {
	rootCmd.AddCommand(versionCmd)
	rootCmd.AddCommand(monCmd)
	rootCmd.AddCommand(osdCmd)
	rootCmd.AddCommand(mgrCmd)
	rootCmd.AddCommand(rgwCmd)
	rootCmd.AddCommand(mdsCmd)
	rootCmd.AddCommand(apiCmd)
	rootCmd.AddCommand(operatorCmd)
}

func addStandaloneRookFlags(command *cobra.Command) {
	command.Flags().StringVar(&cfg.discoveryURL, "discovery-url", "", "etcd discovery URL. Example: http://discovery.rook.com/26bd83c92e7145e6b103f623263f61df")
	command.Flags().StringVar(&cfg.etcdMembers, "etcd-members", "", "etcd members to connect to. Overrides the discovery URL. Example: http://10.23.45.56:2379")
	command.Flags().StringVar(&cfg.networkInfo.PublicNetwork, "public-network", "", "public (front-side) network and subnet mask for the cluster, using CIDR notation (e.g., 192.168.0.0/24)")
	command.Flags().StringVar(&cfg.networkInfo.ClusterNetwork, "private-network", "", "private (back-side) network and subnet mask for the cluster, using CIDR notation (e.g., 10.0.0.0/24)")
	addOSDFlags(command)
	addCephFlags(command)
}

func startJoinCluster(cmd *cobra.Command, args []string) error {
	// verify required flags
	if err := flags.VerifyRequiredFlags(cmd, []string{}); err != nil {
		return err
	}

	setLogLevel()

	if err := joinCluster(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}

	return nil
}

func joinCluster() error {
	// get the absolute path for the data dir
	var err error
	if cfg.dataDir, err = filepath.Abs(cfg.dataDir); err != nil {
		return fmt.Errorf("invalid data directory %s. %+v", cfg.dataDir, err)
	}

	// ensure the data root exists
	if err := os.MkdirAll(cfg.dataDir, 0744); err != nil {
		logger.Warningf("failed to create data directory at %s: %+v", cfg.dataDir, err)
		return nil
	}

	// load the etcd discovery url
	if cfg.discoveryURL == "" && cfg.etcdMembers == "" {
		// if discovery isn't specified and etcd members aren't specified, try to request a default discovery URL
		discURL, err := loadDefaultDiscoveryURL()
		if err != nil {
			return fmt.Errorf("discovery-url and etcd-members not provided, attempt to request a discovery URL failed: %+v", err)
		}
		cfg.discoveryURL = discURL
	}

	services := []*clusterd.ClusterService{
		ceph.NewCephService(cfg.devices, cfg.metadataDevice, cfg.directories,
			cfg.forceFormat, cfg.location, clusterInfo.AdminSecret, cfg.storeConfig),
	}

	cfg.nodeID, err = util.LoadPersistedNodeID(cfg.dataDir)
	if err != nil {
		return fmt.Errorf("failed to load the id. %v", err)
	}

	// start the cluster orchestration services
	context, err := clusterd.StartJoinCluster(services, cfg.dataDir, cfg.nodeID, cfg.discoveryURL,
		cfg.etcdMembers, cfg.networkInfo, cfg.cephConfigOverride, cfg.logLevel)
	if err != nil {
		return err
	}
	defer func() {
		context.ProcMan.Shutdown()
		<-time.After(time.Duration(1) * time.Second)
	}()

	apiConfig := &api.Config{
		Port:           model.Port,
		ClusterInfo:    &clusterInfo,
		ClusterHandler: api.NewEtcdHandler(context),
	}
	go api.ServeRoutes(context, apiConfig)

	// wait for user to interrupt/terminate the process
	ch := make(chan os.Signal, 1)
	signal.Notify(ch, os.Interrupt)
	<-ch
	fmt.Println("terminating due to ctrl-c interrupt...")

	return nil
}

func loadDefaultDiscoveryURL() (string, error) {
	// try to load the cached discovery URL if it exists
	cachedPath := path.Join(cfg.dataDir, "rook-discovery-url")
	fileContent, err := ioutil.ReadFile(cachedPath)
	if err == nil {
		return strings.TrimSpace(string(fileContent)), nil
	}

	// fall back to requesting a discovery URL
	url := "https://discovery.etcd.io/new?size=1"
	resp, err := http.Get(url)
	if err != nil {
		return "", err
	}

	defer resp.Body.Close()
	respBody, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}

	discoveryURL := strings.TrimSpace(string(respBody))

	// cache the requested discovery URL
	if err := ioutil.WriteFile(cachedPath, []byte(discoveryURL), 0644); err != nil {
		return "", err
	}

	return discoveryURL, nil
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

func createContext() *clusterd.Context {
	executor := &exec.CommandExecutor{}
	return &clusterd.Context{
		Executor:           executor,
		ProcMan:            proc.New(executor),
		ConfigDir:          cfg.dataDir,
		ConfigFileOverride: cfg.cephConfigOverride,
		LogLevel:           cfg.logLevel,
	}
}
