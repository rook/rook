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
	"net/http"
	"os"
	"strconv"
	"text/tabwriter"
	"time"

	"github.com/coreos/pkg/capnslog"
	"github.com/rook/rook/pkg/model"
	"github.com/rook/rook/pkg/rook/client"
	"github.com/rook/rook/pkg/util/flags"
	"github.com/spf13/cobra"
)

var (
	APIServerEndpoint string
	logLevelRaw       string
)

const (
	outputPadding  = 3
	outputMinWidth = 10
	outputTabWidth = 0
	outputPadChar  = ' '
	timeoutSecs    = 10
)

var RootCmd = &cobra.Command{
	Use:   "rook",
	Short: "A command line client for working with a rook cluster",
	Long: `A command line client for working with a rook cluster.
https://github.com/rook/rook`,
}

func init() {
	defaultHost := os.Getenv("ROOK_API_SERVICE_HOST")
	if defaultHost == "" {
		defaultHost = "127.0.0.1"
	}
	defaultPort := os.Getenv("ROOK_API_SERVICE_PORT")
	if defaultPort == "" {
		defaultPort = strconv.Itoa(model.Port)
	}
	defaultEndpoint := fmt.Sprintf("%s:%s", defaultHost, defaultPort)

	RootCmd.PersistentFlags().StringVar(&APIServerEndpoint, "api-server-endpoint", defaultEndpoint, "IP endpoint of API server instance (required)")
	RootCmd.PersistentFlags().StringVar(&logLevelRaw, "log-level", "WARNING", "logging level for logging/tracing output (valid values: CRITICAL,ERROR,WARNING,NOTICE,INFO,DEBUG,TRACE)")

	RootCmd.MarkFlagRequired("api-server-endpoint")

	// load the environment variables
	flags.SetFlagsFromEnv(RootCmd.PersistentFlags(), "ROOK")
}

func SetupLogging() {
	// parse the given log level and set it at a global level
	logLevel, err := capnslog.ParseLevel(logLevelRaw)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	capnslog.SetGlobalLogLevel(logLevel)
}

func NewRookNetworkRestClient() client.RookRestClient {
	return NewRookNetworkRestClientWithTimeout(time.Duration(timeoutSecs * time.Second))
}

func NewRookNetworkRestClientWithTimeout(timeout time.Duration) client.RookRestClient {
	httpClient := http.DefaultClient
	httpClient.Timeout = timeout
	return client.NewRookNetworkRestClient(client.GetRestURL(APIServerEndpoint), httpClient)
}

func NewTableWriter(buffer io.Writer) *tabwriter.Writer {
	return tabwriter.NewWriter(buffer, outputMinWidth, outputTabWidth, outputPadding, outputPadChar, 0)
}
