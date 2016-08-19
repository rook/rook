package cmd

import "github.com/spf13/cobra"

var (
	clusterName    string
	configFilePath string
)

var rootCmd = &cobra.Command{
	Use:   "castlectl",
	Short: "A command line client for working with a castle cluster",
	Long:  `https://github.com/quantum/castle`,
}

func init() {
	rootCmd.PersistentFlags().StringVar(&clusterName, "cluster-name", "defaultCluster", "name of ceph cluster (required)")
	rootCmd.PersistentFlags().StringVar(&configFilePath, "config-file", "", "full path to the ceph config file to use (required)")

	rootCmd.MarkFlagRequired("cluster-name")
	rootCmd.MarkFlagRequired("config-file")
}

func Execute() error {
	addCommands()
	return rootCmd.Execute()
}

func addCommands() {
	rootCmd.AddCommand(healthCmd)
	rootCmd.AddCommand(pingCmd)
	rootCmd.AddCommand(readCmd)
	rootCmd.AddCommand(writeCmd)
	rootCmd.AddCommand(getPoolVarCmd)
	rootCmd.AddCommand(monCommandCmd)
}
