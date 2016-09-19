package cmd

import "github.com/spf13/cobra"

var (
	apiServerEndpoint string
)

var rootCmd = &cobra.Command{
	Use:   "castlectl",
	Short: "A command line client for working with a castle cluster",
	Long:  `https://github.com/quantum/castle`,
}

func init() {
	rootCmd.PersistentFlags().StringVar(&apiServerEndpoint, "api-server-endpoint", "127.0.0.1:8124", "IP endpoint of API server instance (required)")

	rootCmd.MarkFlagRequired("api-server-endpoint")
}

func Execute() error {
	addCommands()
	return rootCmd.Execute()
}

func addCommands() {
	rootCmd.AddCommand(nodeCmd)
}
