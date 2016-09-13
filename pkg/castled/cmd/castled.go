package cmd

import (
	"fmt"
	"os"
	"os/signal"
	"time"

	"github.com/spf13/cobra"

	"github.com/quantum/castle/pkg/castled"
	"github.com/quantum/castle/pkg/util"
	"github.com/quantum/clusterd/pkg/orchestrator"
	"github.com/quantum/clusterd/pkg/proc"
)

var (
	discoveryURL string
	etcdMembers  string
	privateIPv4  string
	devices      string
	forceFormat  bool
)

var rootCmd = &cobra.Command{
	Use:   "castled",
	Short: "castled tool for bootstrapping and running castle storage.",
	Long:  `https://github.com/quantum/castle`,
}

func init() {
	rootCmd.Flags().StringVar(&discoveryURL, "discovery-url", "", "etcd discovery URL. Example: http://discovery.castle.com/26bd83c92e7145e6b103f623263f61df")
	rootCmd.Flags().StringVar(&etcdMembers, "etcd-members", "", "etcd members to connect to. Overrides the discovery URL. Example: http://10.23.45.56:2379")
	rootCmd.Flags().StringVar(&privateIPv4, "private-ipv4", "", "private IPv4 address for this machine (required)")
	rootCmd.Flags().StringVar(&devices, "devices", "", "comma separated list of devices to use")
	rootCmd.Flags().BoolVar(&forceFormat, "force-format", false,
		"true to force the format of any specified devices, even if they already have a filesystem.  BE CAREFUL!")

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
	if discoveryURL == "" && etcdMembers == "" {
		return fmt.Errorf("either discovery-url or etcd-members settings are required")
	}

	services := []*orchestrator.ClusterService{castled.NewCephService(devices, forceFormat)}
	procMan := &proc.ProcManager{}
	defer func() {
		procMan.Shutdown()
		<-time.After(time.Duration(1) * time.Second)
	}()

	// start the cluster orchestration services
	if err := orchestrator.StartJoinCluster(services, procMan, discoveryURL, etcdMembers, privateIPv4); err != nil {
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
