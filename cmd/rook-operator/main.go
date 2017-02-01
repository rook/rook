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
package main

import (
	"fmt"
	"os"

	"github.com/coreos/pkg/capnslog"
	"github.com/rook/rook/pkg/cephmgr/cephd"
	"github.com/rook/rook/pkg/operator"
	"github.com/rook/rook/pkg/operator/k8sutil"
	"github.com/rook/rook/pkg/util/flags"
	"github.com/spf13/cobra"
	"k8s.io/client-go/1.5/kubernetes"
	"k8s.io/client-go/1.5/rest"
)

var rootCmd = &cobra.Command{
	Use:   "rook-operator",
	Short: "rook-operator tool for running rook storage in a kubernetes cluster",
	Long: `
Tool for running the rook storage components in a kubernetes cluster.
https://github.com/rook/rook`,
}

var cfg = &config{}

type config struct {
	containerVersion string
	logLevel         capnslog.LogLevel
}

var logLevelRaw string
var logger = capnslog.NewPackageLogger("github.com/rook/rook-operator", "rook-operator")

func main() {
	addCommands()

	if err := rootCmd.Execute(); err != nil {
		fmt.Printf("rookd error: %+v\n", err)
	}
}

func addCommands() {
}

func init() {
	rootCmd.Flags().StringVar(&cfg.containerVersion, "container-version", "private-dev-build", "version of the rook container to launch")

	rootCmd.PersistentFlags().StringVar(&logLevelRaw, "log-level", "INFO", "logging level for logging/tracing output (valid values: CRITICAL,ERROR,WARNING,NOTICE,INFO,DEBUG,TRACE)")

	// load the environment variables
	flags.SetFlagsFromEnv(rootCmd.Flags(), "ROOK_OPERATOR")
	flags.SetFlagsFromEnv(rootCmd.PersistentFlags(), "ROOK_OPERATOR")

	rootCmd.RunE = startOperator
}

func startOperator(cmd *cobra.Command, args []string) error {
	// verify required flags
	if err := flags.VerifyRequiredFlags(cmd, []string{}); err != nil {
		return err
	}

	setLogLevel()

	// create the k8s client
	config, err := rest.InClusterConfig()
	if err != nil {
		fmt.Printf("failed to get k8s config. %+v", err)
		os.Exit(1)
	}
	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		fmt.Printf("failed to get k8s client. %+v", err)
		os.Exit(1)
	}

	logger.Infof("starting operator. containerVersion=%s", cfg.containerVersion)
	op := operator.New(k8sutil.Namespace, cephd.New(), clientset, cfg.containerVersion)
	err = op.Run()
	if err != nil {
		fmt.Printf("failed to run operator. %+v\n", err)
		os.Exit(1)
	}

	return nil
}

func setLogLevel() {
	// parse given log level string then set up corresponding global logging level
	ll, err := capnslog.ParseLevel(logLevelRaw)
	if err != nil {
		logger.Warningf("failed to set log level %s. %+v", logLevelRaw, err)
	}
	cfg.logLevel = ll
	capnslog.SetGlobalLogLevel(cfg.logLevel)
}
