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
	"github.com/rook/rook/pkg/ceph/mon"
	"github.com/rook/rook/pkg/util/flags"
	"github.com/spf13/cobra"
)

var (
	apiCmd = &cobra.Command{
		Use:   "api",
		Short: "Runs the Rook API service",
	}
	apiPort    int
	repoPrefix string
	namespace  string
	versionTag string
)

func init() {
	apiCmd.Flags().IntVar(&apiPort, "port", 0, "port on which the api is listening")
	apiCmd.Flags().StringVar(&repoPrefix, "repo-prefix", "quay.io/rook", "the repo from which to pull images")
	apiCmd.Flags().StringVar(&versionTag, "version-tag", "latest", "version of the rook container to launch")
	apiCmd.Flags().StringVar(&namespace, "namespace", "", "the namespace in which the api service is running")
	addCephFlags(apiCmd)

	flags.SetFlagsFromEnv(apiCmd.Flags(), "ROOKD")

	apiCmd.RunE = startAPI
}

func startAPI(cmd *cobra.Command, args []string) error {
	if err := flags.VerifyRequiredFlags(apiCmd, []string{"repo-prefix", "namespace", "config-dir", "cluster-name", "mon-endpoints", "mon-secret", "admin-secret"}); err != nil {
		return err
	}
	if apiPort == 0 {
		return fmt.Errorf("port is required")
	}

	setLogLevel()

	_, clientset, err := getClientset()
	if err != nil {
		fmt.Printf("failed to init k8s client. %+v\n", err)
		os.Exit(1)
	}

	clusterInfo.Monitors = mon.ParseMonEndpoints(cfg.monEndpoints)
	context := createContext()
	context.Clientset = clientset
	apiCfg := &api.Config{
		Port:           apiPort,
		ClusterInfo:    &clusterInfo,
		ClusterHandler: apik8s.New(context, &clusterInfo, namespace, versionTag),
	}

	err = api.Run(context, apiCfg)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}

	return nil
}
