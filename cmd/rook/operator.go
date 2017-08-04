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

	"github.com/rook/rook/pkg/clusterd"
	"github.com/rook/rook/pkg/operator"
	"github.com/rook/rook/pkg/operator/k8sutil"
	"github.com/rook/rook/pkg/util/flags"
	"github.com/spf13/cobra"
	apiextensionsclient "k8s.io/apiextensions-apiserver/pkg/client/clientset/clientset"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
)

var operatorCmd = &cobra.Command{
	Use:   "operator",
	Short: "Runs the rook operator tool for storage in a kubernetes cluster",
	Long: `Tool for running the rook storage components in a kubernetes cluster.
https://github.com/rook/rook`,
}

func init() {
	flags.SetFlagsFromEnv(operatorCmd.Flags(), "ROOK")

	operatorCmd.RunE = startOperator
}

func startOperator(cmd *cobra.Command, args []string) error {

	setLogLevel()

	clientset, apiExtClientset, err := getClientset()
	if err != nil {
		fmt.Printf("failed to get k8s client. %+v", err)
		os.Exit(1)
	}

	logger.Infof("starting operator")
	context := createContext()
	context.NetworkInfo = clusterd.NetworkInfo{}
	context.ConfigDir = k8sutil.DataDir
	context.Clientset = clientset
	context.APIExtensionClientset = apiExtClientset

	op := operator.New(context)
	if op == nil {
		fmt.Printf("failed to create operator.")
		os.Exit(1)
	}
	err = op.Run()
	if err != nil {
		fmt.Printf("failed to run operator. %+v\n", err)
		os.Exit(1)
	}

	return nil
}

func getClientset() (kubernetes.Interface, apiextensionsclient.Interface, error) {
	// create the k8s client
	config, err := rest.InClusterConfig()
	if err != nil {
		return nil, nil, fmt.Errorf("failed to get k8s config. %+v", err)
	}

	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to create k8s clientset. %+v", err)
	}
	apiExtClientset, err := apiextensionsclient.NewForConfig(config)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to create k8s API extension clientset. %+v", err)
	}
	return clientset, apiExtClientset, nil
}
