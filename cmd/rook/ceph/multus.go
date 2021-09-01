/*
Copyright 2022 The Rook Authors. All rights reserved.

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

package ceph

import (
	"context"
	"os"
	"os/signal"

	"github.com/pkg/errors"
	"github.com/rook/rook/cmd/rook/rook"
	"github.com/rook/rook/pkg/daemon/ceph/multus"
	operator "github.com/rook/rook/pkg/operator/ceph"
	"github.com/rook/rook/pkg/operator/k8sutil"
	"github.com/rook/rook/pkg/util/flags"
	"github.com/spf13/cobra"
)

var multusMoverCmd = &cobra.Command{
	Use:   "multus-mover",
	Short: "Ensures multus connectivity for CSI by moving multus interfaces to the host network",
}

func init() {
	flags.SetFlagsFromEnv(multusMoverCmd.Flags(), rook.RookEnvVarPrefix)
	multusMoverCmd.RunE = multusMover
}

func multusMover(cmd *cobra.Command, args []string) error {
	rook.SetLogLevel()
	rook.LogStartupInfo(multusMoverCmd.Flags())

	nodeEnvVar := "NODE_NAME"
	nodeName := ""
	if nodeName = os.Getenv(nodeEnvVar); nodeName == "" {
		rook.TerminateFatal(errors.Errorf("node name is not provided by env var %q", nodeEnvVar))
	}

	namespace := ""
	if namespace = os.Getenv(k8sutil.PodNamespaceEnvVar); namespace == "" {
		rook.TerminateFatal(errors.Errorf("pod namespace is not provided by env var %q", k8sutil.PodNamespaceEnvVar))
	}
	logger.Infof("starting multus mover for node %q in namespace %q", nodeName, namespace)

	// Set up signal handler, so that a clean up procedure will be run if the pod running this code is deleted.
	// shutdownSignalChan := make(chan os.Signal, 1)
	// signal.Notify(shutdownSignalChan, operator.ShutdownSignals...)
	ctx, stopFunc := signal.NotifyContext(context.Background(), operator.ShutdownSignals...)
	defer stopFunc()

	rookContext := rook.NewContext()

	mover := multus.Mover{
		ClusterContext: rookContext,
		Namespace:      namespace,
		NodeName:       nodeName,
	}
	err := mover.Run(ctx)
	if err != nil {
		rook.TerminateFatal(err)
	}

	return nil
}
