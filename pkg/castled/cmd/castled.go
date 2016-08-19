package cmd

import (
	"fmt"
	"log"
	"os"
	"os/signal"
	"path"
	"strconv"
	"strings"

	"github.com/spf13/cobra"

	"github.com/quantum/castle/pkg/castled"
	"github.com/quantum/clusterd/pkg/orchestrator"
	"github.com/quantum/clusterd/pkg/store"
)

var (
	discoveryURL string
	etcdURLs     string
	nodeID       string
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
	rootCmd.Flags().StringVar(&nodeID, "node-id", "12345", "unique ID for the node (required)")
	rootCmd.Flags().StringVar(&discoveryURL, "discovery-url", "http://discovery.castle.com/26bd83c92e7145e6b103f623263f61df",
		"etcd discovery URL")
	rootCmd.Flags().StringVar(&etcdURLs, "etcd-urls", "http://127.0.0.1:4001",
		"comma separated list of etcd listen URLs (required), ignored if the discovery URL is specified")
	rootCmd.Flags().StringVar(&privateIPv4, "private-ipv4", "", "private IPv4 address for this machine (required)")
	rootCmd.Flags().StringVar(&devices, "devices", "", "comma separated list of devices to use")
	rootCmd.Flags().BoolVar(&forceFormat, "force-format", false,
		"true to force the format of any specified devices, even if they already have a filesystem.  BE CAREFUL!")

	rootCmd.MarkFlagRequired("node-id")
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
	if err := verifyRequiredFlags(cmd, []string{"node-id", "private-ipv4"}); err != nil {
		return err
	}

	// TODO: Get the etcd client with the discovery token rather than the etcd endpoints
	// get an etcd client to coordinate with the rest of the cluster and load/save config
	etcdClient, err := store.GetEtcdClient(strings.Split(etcdURLs, ","))
	if err != nil {
		return err
	}

	// Write desired state for this machine to etcd
	baseKey := path.Join(orchestrator.DesiredNodesKey, nodeID)
	properties := map[string]string{
		"privateIPv4": privateIPv4,
		"devices":     devices,
		"forceFormat": strconv.FormatBool(forceFormat),
	}
	if err := orchestrator.StoreEtcdProperties(etcdClient, baseKey, properties); err != nil {
		return err
	}

	// initialize a leadership lease manager
	leaseManager, err := orchestrator.InitLeaseManager(etcdClient)
	if err != nil {
		log.Fatalf("failed to initialize lease manager: %s", err.Error())
		return err
	}

	context := &orchestrator.ClusterContext{
		EtcdClient: etcdClient,
		Executor:   &orchestrator.CommandExecutor{},
		NodeID:     nodeID,
		Services:   []*orchestrator.ClusterService{castled.NewMonitorService()},
	}
	clusterLeader := &orchestrator.SimpleLeader{LeaseName: orchestrator.LeaderElectionKey}
	clusterMember := orchestrator.NewClusterMember(context, leaseManager, clusterLeader)

	err = clusterMember.Initialize()
	if err != nil {
		log.Fatalf("failed to initialize local cluster: %v", err)
		return err
	}

	p := &orchestrator.ProcessManager{}
	defer p.Shutdown()
	go func() {
		// Watch for commands from the leader
		orchestrator.WatchForAgentServiceConfig(context, p)
	}()

	// wait for user to interrupt/terminate the process
	ch := make(chan os.Signal, 1)
	signal.Notify(ch, os.Interrupt)
	fmt.Println("waiting for ctrl-c interrupt...")
	<-ch
	fmt.Println("terminating due to ctrl-c interrupt...")

	return nil
}

func verifyRequiredFlags(cmd *cobra.Command, requiredFlags []string) error {
	for _, reqFlag := range requiredFlags {
		val, err := cmd.Flags().GetString(reqFlag)
		if err != nil || val == "" {
			return fmt.Errorf("%s is required for %s", reqFlag, cmd.Name())
		}
	}

	return nil
}
