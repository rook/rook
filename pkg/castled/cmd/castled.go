package cmd

import (
	"fmt"
	"os"
	"os/signal"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"

	"github.com/quantum/castle/pkg/castled"
	"github.com/quantum/castle/pkg/clusterd"
	"github.com/quantum/castle/pkg/proc"
	"github.com/quantum/castle/pkg/util"
)

var rootCmd = &cobra.Command{
	Use:   "castled",
	Short: "castled tool for bootstrapping and running castle storage.",
	Long:  `https://github.com/quantum/castle`,
}
var cfg = newConfig()

type config struct {
	discoveryURL string
	etcdMembers  string
	privateIPv4  string
	devices      string
	forceFormat  bool
	crushMap     *castled.CrushLocation
}

func newConfig() *config {
	return &config{crushMap: &castled.CrushLocation{}}
}

// Initialize the configuration parameters. The precedence from lowest to highest is:
//  1) default value (at compilation)
//  2) environment variables (upper case, replace - with _, and castle prefix. For example, discovery-url is CASTLE_DISCOVERY_URL)
//  3) command line parameter
func init() {
	rootCmd.Flags().StringVar(&cfg.discoveryURL, "discovery-url", "", "etcd discovery URL. Example: http://discovery.castle.com/26bd83c92e7145e6b103f623263f61df")
	rootCmd.Flags().StringVar(&cfg.etcdMembers, "etcd-members", "", "etcd members to connect to. Overrides the discovery URL. Example: http://10.23.45.56:2379")
	rootCmd.Flags().StringVar(&cfg.privateIPv4, "private-ipv4", "", "private IPv4 address for this machine (required)")
	rootCmd.Flags().StringVar(&cfg.devices, "devices", "", "comma separated list of devices to use")
	rootCmd.Flags().BoolVar(&cfg.forceFormat, "force-format", false,
		"true to force the format of any specified devices, even if they already have a filesystem.  BE CAREFUL!")

	// crush map attributes
	rootCmd.Flags().StringVar(&cfg.crushMap.Root, "crushmap-root", "", "osd crush map attribute for the root location")
	rootCmd.Flags().StringVar(&cfg.crushMap.Datacenter, "crushmap-datacenter", "", "osd crush map attribute for the datacenter location")
	rootCmd.Flags().StringVar(&cfg.crushMap.Room, "crushmap-room", "", "osd crush map attribute for the room location")
	rootCmd.Flags().StringVar(&cfg.crushMap.Row, "crushmap-row", "", "osd crush map attribute for the row location")
	rootCmd.Flags().StringVar(&cfg.crushMap.Pod, "crushmap-pod", "", "osd crush map attribute for the pod location")
	rootCmd.Flags().StringVar(&cfg.crushMap.PDU, "crushmap-pdu", "", "osd crush map attribute for the power distribution unit location")
	rootCmd.Flags().StringVar(&cfg.crushMap.Rack, "crushmap-rack", "", "osd crush map attribute for the rack location")
	rootCmd.Flags().StringVar(&cfg.crushMap.Chassis, "crushmap-chassis", "", "osd crush map attribute for the chassis location")

	// load the environment variables
	setFlagsFromEnv(rootCmd.Flags())

	rootCmd.MarkFlagRequired("private-ipv4")

	rootCmd.RunE = joinCluster
}

func Execute() error {
	addCommands()
	return rootCmd.Execute()
}

func addCommands() {
	rootCmd.AddCommand(versionCmd)
	rootCmd.AddCommand(daemonCmd)
}

func joinCluster(cmd *cobra.Command, args []string) error {

	if err := util.VerifyRequiredFlags(cmd, []string{"private-ipv4"}); err != nil {
		return err
	}
	if cfg.discoveryURL == "" && cfg.etcdMembers == "" {
		return fmt.Errorf("either discovery-url or etcd-members settings are required")
	}

	services := []*clusterd.ClusterService{castled.NewCephService(cfg.devices, cfg.forceFormat, cfg.crushMap)}
	procMan := &proc.ProcManager{}
	defer func() {
		procMan.Shutdown()
		<-time.After(time.Duration(1) * time.Second)
	}()

	// start the cluster orchestration services
	if err := clusterd.StartJoinCluster(services, procMan, cfg.discoveryURL, cfg.etcdMembers, cfg.privateIPv4); err != nil {
		return err
	}

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
		envVar := "CASTLE_" + strings.Replace(strings.ToUpper(f.Name), "-", "_", -1)
		value := os.Getenv(envVar)
		if value != "" {
			// Set the environment variable. Will override default values, but be overridden by command line parameters.
			flags.Set(f.Name, value)
		}
	})

	return nil
}
