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
	"time"

	"github.com/rook/rook/cmd/rook/rook"
	rookinformers "github.com/rook/rook/pkg/client/informers/externalversions"
	"github.com/rook/rook/pkg/operator/cassandra/constants"
	"github.com/rook/rook/pkg/operator/cassandra/controller"
	"github.com/rook/rook/pkg/util/flags"
	"github.com/spf13/cobra"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apiserver/pkg/server"
	kubeinformers "k8s.io/client-go/informers"
	"k8s.io/client-go/informers/internalinterfaces"
)

const resyncPeriod = time.Second * 30

var operatorCmd = &cobra.Command{
	Use:   "operator",
	Short: "Runs the cassandra operator to deploy and manage cassandra in Kubernetes",
	Long: `Runs the cassandra operator to deploy and manage cassandra in kubernetes clusters.
https://github.com/rook/rook`,
}

func init() {
	flags.SetFlagsFromEnv(operatorCmd.Flags(), rook.RookEnvVarPrefix)

	operatorCmd.RunE = startOperator
}

func startOperator(cmd *cobra.Command, args []string) error {
	rook.SetLogLevel()
	rook.LogStartupInfo(operatorCmd.Flags())

	logger.Infof("starting cassandra operator")
	context := rook.NewContext()
	kubeClient := context.Clientset
	rookClient := context.RookClientset
	rookImage := rook.GetOperatorImage(kubeClient, "")

	// Only watch kubernetes resources relevant to our app
	var tweakListOptionsFunc internalinterfaces.TweakListOptionsFunc
	tweakListOptionsFunc = func(options *metav1.ListOptions) {

		options.LabelSelector = fmt.Sprintf("%s=%s", "app", constants.AppName)
	}

	kubeInformerFactory := kubeinformers.NewSharedInformerFactoryWithOptions(kubeClient, resyncPeriod, kubeinformers.WithTweakListOptions(tweakListOptionsFunc))
	rookInformerFactory := rookinformers.NewSharedInformerFactory(rookClient, resyncPeriod)

	c := controller.New(
		rookImage,
		kubeClient,
		rookClient,
		rookInformerFactory.Cassandra().V1alpha1().Clusters(),
		kubeInformerFactory.Apps().V1().StatefulSets(),
		kubeInformerFactory.Core().V1().Services(),
		kubeInformerFactory.Core().V1().Pods(),
	)

	// Create a channel to receive OS signals
	stopCh := server.SetupSignalHandler()

	// Start the informer factories
	go kubeInformerFactory.Start(stopCh)
	go rookInformerFactory.Start(stopCh)

	// Start the controller
	if err := c.Run(1, stopCh); err != nil {
		logger.Fatalf("Error running controller: %s", err.Error())
	}

	return nil
}
