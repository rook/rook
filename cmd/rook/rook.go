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
	"os"
	"text/tabwriter"

	"github.com/coreos/pkg/capnslog"
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
	Use:   "rook",
	Short: "A command line client for working with a rook cluster",
	Long:  `https://github.com/rook/rook`,
}

var logLevelRaw string
var logger = capnslog.NewPackageLogger("github.com/rook/rook", "rook")

func init() {
	rootCmd.PersistentFlags().StringVar(&apiServerEndpoint, "api-server-endpoint", "127.0.0.1:8124", "IP endpoint of API server instance (required)")
	rootCmd.PersistentFlags().StringVar(&logLevelRaw, "log-level", "WARNING", "logging level for logging/tracing output (valid values: CRITICAL,ERROR,WARNING,NOTICE,INFO,DEBUG,TRACE)")

	rootCmd.MarkFlagRequired("api-server-endpoint")
}

func Main() {
	// parse the given log level and set it at a global level
	logLevel, err := capnslog.ParseLevel(logLevelRaw)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	capnslog.SetGlobalLogLevel(logLevel)

	addCommands()
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func addCommands() {
	rootCmd.AddCommand(nodeCmd)
	rootCmd.AddCommand(poolCmd)
	rootCmd.AddCommand(blockCmd)
	rootCmd.AddCommand(filesystemCmd)
	rootCmd.AddCommand(statusCmd)
	rootCmd.AddCommand(versionCmd)
}

func NewTableWriter(buffer io.Writer) *tabwriter.Writer {
	return tabwriter.NewWriter(buffer, outputMinWidth, outputTabWidth, outputPadding, outputPadChar, 0)
}
