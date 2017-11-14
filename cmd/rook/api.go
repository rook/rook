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

	"github.com/rook/rook/pkg/api"
	"github.com/rook/rook/pkg/ceph/mon"
	"github.com/rook/rook/pkg/util/flags"
	"github.com/spf13/cobra"
)

var (
	apiCmd = &cobra.Command{
		Use:   "api",
		Short: "Runs the Rook API service",
	}
	apiPort     int
	repoPrefix  string
	namespace   string
	versionTag  string
	hostNetwork bool
)

func init() {
	apiCmd.Flags().IntVar(&apiPort, "port", 0, "port on which the api is listening")
	apiCmd.Flags().StringVar(&repoPrefix, "repo-prefix", "rook", "the repo from which to pull images")
	apiCmd.Flags().StringVar(&versionTag, "version-tag", "latest", "version of the rook container to launch")
	apiCmd.Flags().StringVar(&namespace, "namespace", "", "the namespace in which the api service is running")
	apiCmd.Flags().BoolVar(&hostNetwork, "hostnetwork", false, "if the hostnetwork should be used")
	addCephFlags(apiCmd)

	flags.SetFlagsFromEnv(apiCmd.Flags(), RookEnvVarPrefix)

	apiCmd.RunE = startAPI
}

func startAPI(cmd *cobra.Command, args []string) error {
	required := []string{"repo-prefix", "namespace", "config-dir", "cluster-name", "mon-endpoints", "mon-secret", "admin-secret", "public-ipv4", "private-ipv4"}
	if err := flags.VerifyRequiredFlags(apiCmd, required); err != nil {
		return err
	}
	if apiPort == 0 {
		return fmt.Errorf("port is required")
	}

	setLogLevel()

	logStartupInfo(apiCmd.Flags())

	clientset, _, err := getClientset()
	if err != nil {
		terminateFatal(fmt.Errorf("failed to init k8s client. %+v\n", err))
	}

	clusterInfo.Monitors = mon.ParseMonEndpoints(cfg.monEndpoints)
	context := createContext()
	context.Clientset = clientset
	c := api.NewConfig(context, apiPort, &clusterInfo, namespace, versionTag, hostNetwork)

	err = api.Run(context, c)
	if err != nil {
		terminateFatal(err)
	}

	return nil
}
