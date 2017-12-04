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

	opkit "github.com/rook/operator-kit"
	rookalpha "github.com/rook/rook/pkg/apis/rook.io/v1alpha1"
	"github.com/rook/rook/pkg/clusterd"
	"github.com/rook/rook/pkg/daemon/agent/flexvolume/attachment"
	"github.com/rook/rook/pkg/operator"
	"github.com/rook/rook/pkg/operator/cluster/mon"
	"github.com/rook/rook/pkg/operator/k8sutil"
	"github.com/rook/rook/pkg/util/flags"
	"github.com/spf13/cobra"
)

var operatorCmd = &cobra.Command{
	Use:   "operator",
	Short: "Runs the rook operator tool for storage in a kubernetes cluster",
	Long: `Tool for running the rook storage components in a kubernetes cluster.
https://github.com/rook/rook`,
}

func init() {
	operatorCmd.Flags().DurationVar(&mon.HealthCheckInterval, "mon-healthcheck-interval", mon.HealthCheckInterval, "mon health check interval (duration)")
	operatorCmd.Flags().DurationVar(&mon.MonOutTimeout, "mon-out-timeout", mon.MonOutTimeout, "mon out timeout (duration)")
	flags.SetFlagsFromEnv(operatorCmd.Flags(), RookEnvVarPrefix)

	operatorCmd.RunE = startOperator
}

func startOperator(cmd *cobra.Command, args []string) error {

	setLogLevel()

	logStartupInfo(operatorCmd.Flags())

	clientset, apiExtClientset, rookClientset, err := getClientset()
	if err != nil {
		terminateFatal(fmt.Errorf("failed to get k8s client. %+v", err))
	}

	logger.Infof("starting operator")
	context := createContext()
	context.NetworkInfo = clusterd.NetworkInfo{}
	context.ConfigDir = k8sutil.DataDir
	context.Clientset = clientset
	context.APIExtensionClientset = apiExtClientset
	context.RookClientset = rookClientset
	volumeAttachmentClient, _, err := opkit.NewHTTPClient(rookalpha.CustomResourceGroup, rookalpha.Version, attachment.SchemeBuilder)
	if err != nil {
		terminateFatal(err)
	}
	volumeAttachmentController, err := attachment.CreateController(context.Clientset, volumeAttachmentClient)
	if err != nil {
		terminateFatal(err)
	}

	op := operator.New(context, volumeAttachmentController)
	if op == nil {
		terminateFatal(fmt.Errorf("failed to create operator."))
	}
	err = op.Run()
	if err != nil {
		terminateFatal(fmt.Errorf("failed to run operator. %+v\n", err))
	}

	return nil
}
