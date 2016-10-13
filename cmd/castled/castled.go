package main

import (
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"

	"github.com/quantum/castle/pkg/api"
	"github.com/quantum/castle/pkg/cephmgr"
	"github.com/quantum/castle/pkg/cephmgr/cephd"
	"github.com/quantum/castle/pkg/clusterd"
	"github.com/quantum/castle/pkg/util"
	"github.com/quantum/castle/pkg/util/flags"
	"github.com/quantum/castle/pkg/util/proc"
)

var rootCmd = &cobra.Command{
	Use:   "castled",
	Short: "castled tool for bootstrapping and running castle storage",
	Long:  `https://github.com/quantum/castle`,
}
var cfg = newConfig()

type config struct {
	nodeID       string
	discoveryURL string
	etcdMembers  string
	publicIPv4   string
	privateIPv4  string
	devices      string
	forceFormat  bool
	location     string
}

func newConfig() *config {
	return &config{}
}

func main() {
	addCommands()

	if err := rootCmd.Execute(); err != nil {
		fmt.Printf("castled error: %+v\n", err)
	}
}

// Initialize the configuration parameters. The precedence from lowest to highest is:
//  1) default value (at compilation)
//  2) environment variables (upper case, replace - with _, and castle prefix. For example, discovery-url is CASTLE_DISCOVERY_URL)
//  3) command line parameter
func init() {
	rootCmd.Flags().StringVar(&cfg.nodeID, "id", "", "unique identifier in the cluster for this machine. defaults to /etc/machine-id if found.")
	rootCmd.Flags().StringVar(&cfg.discoveryURL, "discovery-url", "", "etcd discovery URL. Example: http://discovery.castle.com/26bd83c92e7145e6b103f623263f61df")
	rootCmd.Flags().StringVar(&cfg.etcdMembers, "etcd-members", "", "etcd members to connect to. Overrides the discovery URL. Example: http://10.23.45.56:2379")
	rootCmd.Flags().StringVar(&cfg.publicIPv4, "public-ipv4", "", "public IPv4 address for this machine")
	rootCmd.Flags().StringVar(&cfg.privateIPv4, "private-ipv4", "127.0.0.1", "private IPv4 address for this machine")
	rootCmd.Flags().StringVar(&cfg.devices, "devices", "", "comma separated list of devices to use")
	rootCmd.Flags().BoolVar(&cfg.forceFormat, "force-format", false,
		"true to force the format of any specified devices, even if they already have a filesystem.  BE CAREFUL!")
	rootCmd.Flags().StringVar(&cfg.location, "location", "", "location of this node for CRUSH placement")

	// load the environment variables
	setFlagsFromEnv(rootCmd.Flags())

	rootCmd.RunE = joinCluster
}

func addCommands() {
	rootCmd.AddCommand(versionCmd)
	rootCmd.AddCommand(daemonCmd)
}

func joinCluster(cmd *cobra.Command, args []string) error {

	if err := flags.VerifyRequiredFlags(cmd, []string{}); err != nil {
		return err
	}
	if cfg.discoveryURL == "" && cfg.etcdMembers == "" {
		// if discovery isn't specified and etcd members aren't specified, try to request a default discovery URL
		discURL, err := loadDefaultDiscoveryURL()
		if err != nil {
			return fmt.Errorf("discovery-url and etcd-members not provided, attempt to request a discovery URL failed: %+v", err)
		}
		cfg.discoveryURL = discURL
	}

	services := []*clusterd.ClusterService{
		cephmgr.NewCephService(cephd.New(), cfg.devices, cfg.forceFormat, cfg.location),
		//etcdmgr.NewEtcdMgrService(cfg.discoveryURL),
	}
	procMan := &proc.ProcManager{}
	defer func() {
		procMan.Shutdown()
		<-time.After(time.Duration(1) * time.Second)
	}()

	if cfg.nodeID == "" {
		// read /etc/machine-id
		var err error
		cfg.nodeID, err = util.GetMachineID()
		if err != nil {
			return fmt.Errorf("id not provided and failed to read /etc/machine-id. %v", err)
		}
	}

	// start the cluster orchestration services
	context, err := clusterd.StartJoinCluster(services, procMan, cfg.nodeID, cfg.discoveryURL, cfg.etcdMembers, cfg.publicIPv4, cfg.privateIPv4)
	if err != nil {
		return err
	}

	go func() {
		// set up routes and start HTTP server for REST API
		h := api.NewHandler(context.EtcdClient, cephmgr.NewConnectionFactory(), cephd.New())
		r := api.NewRouter(h.GetRoutes())
		if err := http.ListenAndServe(":8124", r); err != nil {
			log.Printf("API server error: %+v", err)
		}
	}()

	// wait for user to interrupt/terminate the process
	ch := make(chan os.Signal, 1)
	signal.Notify(ch, os.Interrupt)
	fmt.Println("waiting for ctrl-c interrupt...")
	<-ch
	fmt.Println("terminating due to ctrl-c interrupt...")

	return nil
}

func setFlagsFromEnv(flags *pflag.FlagSet) error {
	flags.VisitAll(func(f *pflag.Flag) {
		envVar := "CASTLED_" + strings.Replace(strings.ToUpper(f.Name), "-", "_", -1)
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
	cachedPath := "/tmp/castle-discovery-url"
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
