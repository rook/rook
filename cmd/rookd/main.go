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
	"log"
	"net/http"
	"os"
	"os/signal"
	"path"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"

	"github.com/rook/rook/pkg/api"
	"github.com/rook/rook/pkg/cephmgr"
	"github.com/rook/rook/pkg/cephmgr/cephd"
	"github.com/rook/rook/pkg/clusterd"

	"github.com/rook/rook/pkg/util"
	"github.com/rook/rook/pkg/util/flags"
)

var rootCmd = &cobra.Command{
	Use:   "rookd",
	Short: "rookd tool for bootstrapping and running rook storage",
	Long:  `https://github.com/rook/rook`,
}
var cfg = newConfig()

type config struct {
	nodeID       string
	discoveryURL string
	etcdMembers  string
	publicIPv4   string
	privateIPv4  string
	devices      string
	dataDir      string
	adminSecret  string
	forceFormat  bool
	location     string
	debug        bool
}

func newConfig() *config {
	return &config{}
}

func main() {
	addCommands()

	if err := rootCmd.Execute(); err != nil {
		fmt.Printf("rookd error: %+v\n", err)
	}
}

// Initialize the configuration parameters. The precedence from lowest to highest is:
//  1) default value (at compilation)
//  2) environment variables (upper case, replace - with _, and rook prefix. For example, discovery-url is ROOK_DISCOVERY_URL)
//  3) command line parameter
func init() {
	rootCmd.Flags().StringVar(&cfg.adminSecret, "admin-secret", "", "secret for the admin user (random if not specified)")
	rootCmd.Flags().StringVar(&cfg.nodeID, "id", "", "unique identifier in the cluster for this machine. defaults to /etc/machine-id if found.")
	rootCmd.Flags().StringVar(&cfg.discoveryURL, "discovery-url", "", "etcd discovery URL. Example: http://discovery.rook.com/26bd83c92e7145e6b103f623263f61df")
	rootCmd.Flags().StringVar(&cfg.etcdMembers, "etcd-members", "", "etcd members to connect to. Overrides the discovery URL. Example: http://10.23.45.56:2379")
	rootCmd.Flags().StringVar(&cfg.publicIPv4, "public-ipv4", "127.0.0.1", "public IPv4 address for this machine")
	rootCmd.Flags().StringVar(&cfg.privateIPv4, "private-ipv4", "127.0.0.1", "private IPv4 address for this machine")
	rootCmd.Flags().StringVar(&cfg.devices, "data-devices", "", "comma separated list of devices to use for storage")
	rootCmd.Flags().StringVar(&cfg.dataDir, "data-dir", "/var/lib/rook", "directory for storing configuration")
	rootCmd.Flags().BoolVar(&cfg.forceFormat, "force-format", false,
		"true to force the format of any specified devices, even if they already have a filesystem.  BE CAREFUL!")
	rootCmd.Flags().StringVar(&cfg.location, "location", "", "location of this node for CRUSH placement")

	rootCmd.PersistentFlags().BoolVar(&cfg.debug, "debug", false, "true to enable debug logging/tracing")

	// load the environment variables
	setFlagsFromEnv(rootCmd.Flags())

	rootCmd.RunE = startJoinCluster
}

func addCommands() {
	rootCmd.AddCommand(versionCmd)
	rootCmd.AddCommand(daemonCmd)
}

func startJoinCluster(cmd *cobra.Command, args []string) error {
	// verify required flags
	if err := flags.VerifyRequiredFlags(cmd, []string{}); err != nil {
		return err
	}

	if err := joinCluster(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}

	return nil
}

func joinCluster() error {
	// ensure the data root exists
	if err := os.MkdirAll(cfg.dataDir, 0744); err != nil {
		log.Printf("failed to create data directory at %s: %+v", cfg.dataDir, err)
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
		cephmgr.NewCephService(cephd.New(), cfg.devices, cfg.forceFormat, cfg.location, cfg.adminSecret),
	}

	if cfg.nodeID == "" {
		// read /etc/machine-id
		var err error
		cfg.nodeID, err = util.GetMachineID()
		if err != nil {
			return fmt.Errorf("id not provided and failed to read /etc/machine-id. %v", err)
		}
	}

	// start the cluster orchestration services
	context, err := clusterd.StartJoinCluster(services, cfg.dataDir, cfg.nodeID, cfg.discoveryURL,
		cfg.etcdMembers, cfg.publicIPv4, cfg.privateIPv4, cfg.debug)
	if err != nil {
		return err
	}
	defer func() {
		context.ProcMan.Shutdown()
		<-time.After(time.Duration(1) * time.Second)
	}()

	go func() {
		// set up routes and start HTTP server for REST API
		h := api.NewHandler(context, cephmgr.NewConnectionFactory(), cephd.New())
		r := api.NewRouter(h.GetRoutes())
		if err := http.ListenAndServe(":8124", r); err != nil {
			log.Printf("API server error: %+v", err)
		}
	}()

	// wait for user to interrupt/terminate the process
	ch := make(chan os.Signal, 1)
	signal.Notify(ch, os.Interrupt)
	<-ch
	fmt.Println("terminating due to ctrl-c interrupt...")

	return nil
}

func setFlagsFromEnv(flags *pflag.FlagSet) error {
	flags.VisitAll(func(f *pflag.Flag) {
		envVar := "ROOKD_" + strings.Replace(strings.ToUpper(f.Name), "-", "_", -1)
		value := os.Getenv(envVar)
		if value != "" {
			// Set the environment variable. Will override default values, but be overridden by command line parameters.
			flags.Set(f.Name, value)
		}
	})

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
