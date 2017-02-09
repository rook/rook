// +build linux,amd64 linux,arm64

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

	"github.com/rook/rook/pkg/api"
	apik8s "github.com/rook/rook/pkg/api/k8s"
	"github.com/rook/rook/pkg/cephmgr/cephd"
	"github.com/rook/rook/pkg/cephmgr/mon"
	"github.com/rook/rook/pkg/clusterd"
	"github.com/rook/rook/pkg/util/flags"
	"github.com/spf13/cobra"
)

var apiCmd = &cobra.Command{
	Use:    "api",
	Short:  "Runs the Rook REST API service",
	Hidden: true,
}
var apiPort int

func init() {
	apiCmd.Flags().IntVar(&apiPort, "api-port", 0, "port on which the api is listening")

	flags.SetFlagsFromEnv(rootCmd.Flags(), "ROOK_OPERATOR")

	apiCmd.RunE = startAPI
}

func startAPI(cmd *cobra.Command, args []string) error {
	if err := flags.VerifyRequiredFlags(apiCmd, []string{"data-dir", "cluster-name", "mon-endpoints", "mon-secret", "admin-secret"}); err != nil {
		return err
	}
	if apiPort == 0 {
		return fmt.Errorf("api-port is required")
	}

	setLogLevel()

	clientset, err := getClientset()
	if err != nil {
		fmt.Printf("failed to init k8s client. %+v\n", err)
		os.Exit(1)
	}

	cfg.clusterInfo.Monitors = mon.ParseMonEndpoints(cfg.monEndpoints)
	context := clusterd.NewDaemonContext(cfg.dataDir, cfg.cephConfigOverride, cfg.logLevel)
	factory := cephd.New()
	apiCfg := &api.Config{
		ConnFactory:    mon.NewConnectionFactoryWithClusterInfo(&cfg.clusterInfo),
		CephFactory:    factory,
		Port:           apiPort,
		ClusterHandler: apik8s.New(clientset, context, &cfg.clusterInfo, factory, cfg.containerVersion),
	}

	return api.Run(context, apiCfg)
}
