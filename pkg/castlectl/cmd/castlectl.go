package cmd

import (
	"io"
	"text/tabwriter"

	"github.com/spf13/cobra"
)

var (
	apiServerEndpoint string
)

const (
	outputPadding  = 3
	outputMinWidth = 10
	outputTabWidth = 0
	outputPadChar  = ' '
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
	rootCmd.AddCommand(poolCmd)
}

func NewTableWriter(buffer io.Writer) *tabwriter.Writer {
	return tabwriter.NewWriter(buffer, outputMinWidth, outputTabWidth, outputPadding, outputPadChar, 0)
}
