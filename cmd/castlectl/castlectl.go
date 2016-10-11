package main

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
	outputPadding  = 3
	outputMinWidth = 10
	outputTabWidth = 0
	outputPadChar  = ' '
	logFileDir     = "/tmp/castle/log"
	logFileName    = "castlectl.log"
)

var rootCmd = &cobra.Command{
	Use:   "castlectl",
	Short: "A command line client for working with a castle cluster",
	Long:  `https://github.com/quantum/castle`,
}

func main() {
	// set up logging to a log file instead of stdout (only command output and errors should go to stdout/stderr)
	if err := os.MkdirAll(logFileDir, 0744); err != nil {
		log.Fatalf("failed to create logging dir '%s': %+v", logFileDir, err)
	}
	logFilePath := filepath.Join(logFileDir, logFileName)
	logFile, err := os.OpenFile(logFilePath, os.O_RDWR|os.O_CREATE|os.O_APPEND, 0644)
	if err != nil {
		log.Fatalf("failed to open log file '%s': %v", logFilePath, err)
	}
	defer logFile.Close()
	log.SetOutput(logFile)

	addCommands()
	if err := rootCmd.Execute(); err != nil {
		fmt.Printf("castlectl error: %+v\n", err)
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
