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

package cassandra

import (
	"fmt"
	"os"

	"github.com/rook/rook/cmd/rook/rook"
	"github.com/rook/rook/pkg/operator/cassandra/sidecar"
	"github.com/rook/rook/pkg/operator/k8sutil"
	"github.com/rook/rook/pkg/util/flags"
	"github.com/spf13/cobra"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apiserver/pkg/server"
	kubeinformers "k8s.io/client-go/informers"
	"k8s.io/client-go/informers/internalinterfaces"
)

var sidecarCmd = &cobra.Command{
	Use:   "sidecar",
	Short: "Runs the cassandra sidecar to deploy and manage cassandra in Kubernetes",
	Long: `Runs the cassandra sidecar to deploy and manage cassandra in kubernetes clusters.
https://github.com/rook/rook`,
}

func init() {
	flags.SetFlagsFromEnv(operatorCmd.Flags(), rook.RookEnvVarPrefix)

	sidecarCmd.RunE = startSidecar
}

func startSidecar(cmd *cobra.Command, args []string) error {
	rook.SetLogLevel()
	rook.LogStartupInfo(operatorCmd.Flags())

	context := rook.NewContext()
	kubeClient := context.Clientset
	rookClient := context.RookClientset

	podName := os.Getenv(k8sutil.PodNameEnvVar)
	if podName == "" {
		rook.TerminateFatal(fmt.Errorf("cannot detect the pod name. Please provide it using the downward API in the manifest file"))
	}
	podNamespace := os.Getenv(k8sutil.PodNamespaceEnvVar)
	if podNamespace == "" {
		rook.TerminateFatal(fmt.Errorf("cannot detect the pod namespace. Please provide it using the downward API in the manifest file"))
	}

	// This func will make our informer only watch resources with the name of our member
	tweakListOptionsFunc := internalinterfaces.TweakListOptionsFunc(
		func(options *metav1.ListOptions) {
			options.FieldSelector = fmt.Sprintf("metadata.name=%s", podName)
		},
	)

	// kubeInformerFactory watches resources with:
	// namespace: podNamespace
	// name: podName
	kubeInformerFactory := kubeinformers.NewSharedInformerFactoryWithOptions(
		kubeClient,
		resyncPeriod,
		kubeinformers.WithNamespace(podNamespace),
		kubeinformers.WithTweakListOptions(tweakListOptionsFunc),
	)

	mc, err := sidecar.New(
		podName,
		podNamespace,
		kubeClient,
		rookClient,
		kubeInformerFactory.Core().V1().Services(),
	)

	if err != nil {
		rook.TerminateFatal(fmt.Errorf("failed to initialize member controller: %s", err.Error()))
	}
	logger.Infof("Initialized Member Controller: %+v", mc)

	// Create a channel to receive OS signals
	stopCh := server.SetupSignalHandler()
	go kubeInformerFactory.Start(stopCh)

	// Start the controller loop
	logger.Infof("Starting rook sidecar for Cassandra.")
	if err = mc.Run(1, stopCh); err != nil {
		logger.Fatalf("Error running sidecar: %s", err.Error())
	}

	return nil
}
