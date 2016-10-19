package castle

import (
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"text/tabwriter"

	"github.com/spf13/cobra"
)

var (
	apiServerEndpoint string
)

const (
	systemLogDir   = "/var/log/castle"
	outputPadding  = 3
	outputMinWidth = 10
	outputTabWidth = 0
	outputPadChar  = ' '
	logFileName    = "castle.log"
)

var rootCmd = &cobra.Command{
	Use:   "castle",
	Short: "A command line client for working with a castle cluster",
	Long:  `https://github.com/quantum/castle`,
}

func Main() {
	// set up logging to a log file instead of stdout (only command output and errors should go to stdout/stderr)
	if err := os.MkdirAll(systemLogDir, 0744); err != nil {
		log.Fatalf("failed to create logging dir '%s': %+v", systemLogDir, err)
	}
	logFilePath := filepath.Join(systemLogDir, logFileName)
	logFile, err := os.OpenFile(logFilePath, os.O_RDWR|os.O_CREATE|os.O_APPEND, 0644)
	if err != nil {
		log.Fatalf("failed to open log file '%s': %v", logFilePath, err)
	}
	defer logFile.Close()
	log.SetOutput(logFile)

	addCommands()
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func init() {
	rootCmd.PersistentFlags().StringVar(&apiServerEndpoint, "api-server-endpoint", "127.0.0.1:8124", "IP endpoint of API server instance (required)")

	rootCmd.MarkFlagRequired("api-server-endpoint")
}

func addCommands() {
	rootCmd.AddCommand(nodeCmd)
	rootCmd.AddCommand(poolCmd)
	rootCmd.AddCommand(blockCmd)
}

func NewTableWriter(buffer io.Writer) *tabwriter.Writer {
	return tabwriter.NewWriter(buffer, outputMinWidth, outputTabWidth, outputPadding, outputPadChar, 0)
}
