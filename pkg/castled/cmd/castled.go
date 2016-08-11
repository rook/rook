package cmd

import (
	"fmt"
	"os"
	"os/exec"
	"os/signal"

	"github.com/quantum/castle/pkg/castled"
	"github.com/spf13/cobra"
)

var (
	clusterName string
	etcdURLs    string
	privateIPv4 string
	monNames    string
	initMonSet  string
	devices     string
)

var rootCmd = &cobra.Command{
	Use:   "castled",
	Short: "castled tool for bootstrapping and running castle storage.",
	Long:  `https://github.com/quantum/castle`,
}

func init() {
	rootCmd.Flags().StringVar(&clusterName, "cluster-name", "defaultCluster", "name of ceph cluster (required)")
	rootCmd.Flags().StringVar(&etcdURLs, "etcd-urls", "http://127.0.0.1:4001", "comma separated list of etcd listen URLs (required)")
	rootCmd.Flags().StringVar(&privateIPv4, "private-ipv4", "", "private IPv4 address for this machine (required)")
	rootCmd.Flags().StringVar(&monNames, "mon-names", "mon1", "comma separated list of monitor names to run on this machine (only 1 recommended)")
	rootCmd.Flags().StringVar(&initMonSet, "initial-monitors", "mon1", "comma separated list of initial monitor names in the cluster")
	rootCmd.Flags().StringVar(&devices, "devices", "", "comma separated list of devices to use")

	rootCmd.MarkFlagRequired("cluster-name")
	rootCmd.MarkFlagRequired("etcd-urls")
	rootCmd.MarkFlagRequired("private-ipv4")

	rootCmd.RunE = bootstrap
}

func Execute() error {
	addCommands()
	return rootCmd.Execute()
}

func addCommands() {
	rootCmd.AddCommand(versionCmd)
	rootCmd.AddCommand(daemonCmd)
}

func bootstrap(cmd *cobra.Command, args []string) error {
	if err := verifyRequiredFlags(rootCmd, []string{"cluster-name", "etcd-urls", "private-ipv4"}); err != nil {
		return err
	}

	// TODO: add the discovery URL to the command line/env var config,
	// then we could just ask the discovery service where to find things like etcd
	cfg := castled.NewConfig(clusterName, etcdURLs, privateIPv4, monNames, initMonSet, devices)
	var procs []*exec.Cmd
	go func(cfg castled.Config) {
		var err error
		procs, err = castled.Bootstrap(cfg)
		if err != nil {
			fmt.Printf("failed to bootstrap castled: %+v", err)
		} else {
			fmt.Println("castled bootstrapped successfully!")
		}
	}(cfg)

	// wait for user to interrupt/terminate the process
	ch := make(chan os.Signal, 1)
	signal.Notify(ch, os.Interrupt)
	fmt.Println("waiting for ctrl-c interrupt...")
	<-ch
	fmt.Println("terminating due to ctrl-c interrupt...")
	for i := range procs {
		fmt.Println("stopping child process")
		if err := castled.StopChildProcess(procs[i]); err != nil {
			fmt.Printf("failed to stop child process: %+v", err)
		} else {
			fmt.Println("child process stopped successfully")
		}
	}

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
