/*
Copyright 2018 The Rook Authors. All rights reserved.

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
package minio

import (
	"fmt"

	"github.com/rook/rook/cmd/rook/rook"
	"github.com/rook/rook/pkg/operator/k8sutil"
	"github.com/rook/rook/pkg/operator/minio"
	"github.com/rook/rook/pkg/util/flags"
	"github.com/spf13/cobra"
)

const containerName = "rook-minio-operator"

// TODO: Consider some kind of operator_utils.go file that does all of this boilerplate.
var (
	operatorCmd = &cobra.Command{
		Use:   "operator",
		Short: "Runs the rook operator tool for storage in a kubernetes cluster",
		Long: `Tool for running the rook storage components in a kubernetes cluster.
	https://github.com/rook/rook`,
	}
)

func init() {
	flags.SetFlagsFromEnv(operatorCmd.Flags(), rook.RookEnvVarPrefix)
	operatorCmd.RunE = startOperator
}

func startOperator(cmd *cobra.Command, args []string) error {
	rook.SetLogLevel()

	rook.LogStartupInfo(operatorCmd.Flags())

	clientset, apiExtClientset, rookClientset, err := rook.GetClientset()
	if err != nil {
		rook.TerminateFatal(fmt.Errorf("failed to get k8s client. %+v", err))
	}

	logger.Infof("starting operator")
	context := createContext()
	context.Clientset = clientset
	context.APIExtensionClientset = apiExtClientset
	context.RookClientset = rookClientset
	if err != nil {
		rook.TerminateFatal(err)
	}

	// Using the rook-operator image to deploy other rook pods
	rookImage, err := k8sutil.GetContainerImage(clientset, containerName)
	if err != nil {
		rook.TerminateFatal(fmt.Errorf("failed to get container image. %+v\n", err))
	}

	op := minio.New(context, rookImage)
	err = op.Run()
	if err != nil {
		rook.TerminateFatal(fmt.Errorf("failed to run operator. %+v\n", err))
	}

	return nil
}
