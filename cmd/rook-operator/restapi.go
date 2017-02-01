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

var restapiCmd = &cobra.Command{
	Use:    "restapi",
	Short:  "Runs the Rook REST API service",
	Hidden: true,
}
var restapiPort int

func init() {
	restapiCmd.Flags().IntVar(&restapiPort, "rest-api-port", 0, "rest api port number")

	flags.SetFlagsFromEnv(rootCmd.Flags(), "ROOK_OPERATOR")

	restapiCmd.RunE = startRest
}

func startRest(cmd *cobra.Command, args []string) error {
	if err := flags.VerifyRequiredFlags(restapiCmd, []string{"data-dir", "cluster-name", "mon-endpoints", "mon-secret", "admin-secret"}); err != nil {
		return err
	}
	if restapiPort == 0 {
		return fmt.Errorf("rest-port is required")
	}

	setLogLevel()

	clientset, err := getClientset()
	if err != nil {
		fmt.Printf("failed to init k8s client. %+v\n", err)
		os.Exit(1)
	}

	cfg.clusterInfo.Monitors = mon.ParseMonEndpoints(cfg.monEndpoints)
	context := clusterd.NewDaemonContext(cfg.dataDir, cfg.cephConfigOverride, cfg.logLevel)
	restCfg := &api.Config{
		ConnFactory:  mon.NewConnectionFactoryWithClusterInfo(&cfg.clusterInfo),
		CephFactory:  cephd.New(),
		Port:         restapiPort,
		StateHandler: apik8s.New(clientset),
	}

	return api.Run(context, restCfg)
}
