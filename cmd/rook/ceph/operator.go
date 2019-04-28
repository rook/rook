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

package ceph

import (
	"fmt"

	"github.com/rook/rook/cmd/rook/rook"
	"github.com/rook/rook/pkg/clusterd"
	"github.com/rook/rook/pkg/daemon/ceph/agent/flexvolume/attachment"
	operator "github.com/rook/rook/pkg/operator/ceph"
	"github.com/rook/rook/pkg/operator/ceph/cluster/mon"
	"github.com/rook/rook/pkg/operator/ceph/csi"
	"github.com/rook/rook/pkg/operator/k8sutil"
	"github.com/rook/rook/pkg/util/flags"
	"github.com/spf13/cobra"
)

const containerName = "rook-ceph-operator"

var operatorCmd = &cobra.Command{
	Use:   "operator",
	Short: "Runs the Ceph operator for orchestrating and managing Ceph storage in a Kubernetes cluster",
	Long: `Runs the Ceph operator for orchestrating and managing Ceph storage in a Kubernetes cluster
https://github.com/rook/rook`,
}

func init() {
	operatorCmd.Flags().DurationVar(&mon.HealthCheckInterval, "mon-healthcheck-interval", mon.HealthCheckInterval, "mon health check interval (duration)")
	operatorCmd.Flags().DurationVar(&mon.MonOutTimeout, "mon-out-timeout", mon.MonOutTimeout, "mon out timeout (duration)")

	operatorCmd.Flags().BoolVar(&csi.EnableRBD, "csi-enable-rbd", false, "enable ceph-csi rbd support")
	operatorCmd.Flags().BoolVar(&csi.EnableCephFS, "csi-enable-cephfs", false, "enable ceph-csi cephfs support")
	// csi images
	operatorCmd.Flags().StringVar(&csi.CSIParam.RBDPluginImage, "csi-rbd-image", csi.DefaultRBDPluginImage, "ceph-csi rbd plugin image")
	operatorCmd.Flags().StringVar(&csi.CSIParam.CephFSPluginImage, "csi-cephfs-image", csi.DefaultCephFSPluginImage, "ceph-csi cephfs plugin image")
	operatorCmd.Flags().StringVar(&csi.CSIParam.RegistrarImage, "csi-registrar-image", csi.DefaultRegistrarImage, "csi registrar image")
	operatorCmd.Flags().StringVar(&csi.CSIParam.ProvisionerImage, "csi-provisioner-image", csi.DefaultProvisionerImage, "csi provisioner image")
	operatorCmd.Flags().StringVar(&csi.CSIParam.AttacherImage, "csi-attacher-image", csi.DefaultAttacherImage, "csi attacher image")
	operatorCmd.Flags().StringVar(&csi.CSIParam.SnapshotterImage, "csi-snapshotter-image", csi.DefaultSnapshotterImage, "csi snapshotter image")

	// csi deployment templates
	operatorCmd.Flags().StringVar(&csi.RBDPluginTemplatePath, "csi-rbd-plugin-template-path", csi.DefaultRBDPluginTemplatePath, "path to ceph-csi rbd plugin template")
	operatorCmd.Flags().StringVar(&csi.RBDProvisionerTemplatePath, "csi-rbd-provisioner-template-path", csi.DefaultRBDProvisionerTemplatePath, "path to ceph-csi rbd provisioner template")

	operatorCmd.Flags().StringVar(&csi.CephFSPluginTemplatePath, "csi-cephfs-plugin-template-path", csi.DefaultCephFSPluginTemplatePath, "path to ceph-csi cephfs plugin template")
	operatorCmd.Flags().StringVar(&csi.CephFSProvisionerTemplatePath, "csi-cephfs-provisioner-template-path", csi.DefaultCephFSProvisionerTemplatePath, "path to ceph-csi cephfs provisioner template")

	flags.SetFlagsFromEnv(operatorCmd.Flags(), rook.RookEnvVarPrefix)
	flags.SetLoggingFlags(operatorCmd.Flags())
	operatorCmd.RunE = startOperator
}

func startOperator(cmd *cobra.Command, args []string) error {

	rook.SetLogLevel()

	rook.LogStartupInfo(operatorCmd.Flags())

	clientset, apiExtClientset, rookClientset, err := rook.GetClientset()
	if err != nil {
		rook.TerminateFatal(fmt.Errorf("failed to get k8s client. %+v\n", err))
	}

	logger.Infof("starting operator")
	context := createContext()
	context.NetworkInfo = clusterd.NetworkInfo{}
	context.ConfigDir = k8sutil.DataDir
	context.Clientset = clientset
	context.APIExtensionClientset = apiExtClientset
	context.RookClientset = rookClientset
	volumeAttachment, err := attachment.New(context)
	if err != nil {
		rook.TerminateFatal(err)
	}

	// Using the current image version to deploy other rook pods
	pod, err := k8sutil.GetRunningPod(clientset)
	if err != nil {
		rook.TerminateFatal(fmt.Errorf("failed to get pod. %+v\n", err))
	}

	rookImage, err := k8sutil.GetContainerImage(pod, containerName)
	if err != nil {
		rook.TerminateFatal(fmt.Errorf("failed to get container image. %+v\n", err))
	}

	op := operator.New(context, volumeAttachment, rookImage, pod.Spec.ServiceAccountName)
	err = op.Run()
	if err != nil {
		rook.TerminateFatal(fmt.Errorf("failed to run operator. %+v\n", err))
	}

	return nil
}
