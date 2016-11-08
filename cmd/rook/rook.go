/*
Copyright 2016 The Rook Authors. All rights reserved.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

	http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/
package rook

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
	debugLogVar    = "ROOK_DEBUG_DIR"
	logFileName    = "rook.log"
	outputPadding  = 3
	outputMinWidth = 10
	outputTabWidth = 0
	outputPadChar  = ' '
)

var rootCmd = &cobra.Command{
	Use:   "rook",
	Short: "A command line client for working with a rook cluster",
	Long:  `https://github.com/rook/rook`,
}

func Main() {
	enableLogging()

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
	rootCmd.AddCommand(statusCmd)
	rootCmd.AddCommand(versionCmd)
}

func NewTableWriter(buffer io.Writer) *tabwriter.Writer {
	return tabwriter.NewWriter(buffer, outputMinWidth, outputTabWidth, outputPadding, outputPadChar, 0)
}

func enableLogging() {
	debugDir := os.Getenv(debugLogVar)
	if debugDir == "" {
		return
	}

	// set up logging to a log file instead of stdout (only command output and errors should go to stdout/stderr)
	if err := os.MkdirAll(debugDir, 0744); err != nil {
		log.Fatalf("failed to create logging dir '%s': %+v", debugDir, err)
	}
	logFilePath := filepath.Join(debugDir, logFileName)
	logFile, err := os.OpenFile(logFilePath, os.O_RDWR|os.O_CREATE|os.O_APPEND, 0644)
	if err != nil {
		log.Fatalf("failed to open log file '%s': %v", logFilePath, err)
	}
	defer logFile.Close()
	log.SetOutput(logFile)
}
