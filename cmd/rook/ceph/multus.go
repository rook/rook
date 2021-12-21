/*
Copyright 2021 The Rook Authors. All rights reserved.

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
	"net"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/containernetworking/plugins/pkg/ns"
	"github.com/pkg/errors"
	"github.com/rook/rook/cmd/rook/rook"
	"github.com/rook/rook/pkg/daemon/ceph/multus"
	"github.com/rook/rook/pkg/util/flags"
	"github.com/spf13/cobra"
	"github.com/vishvananda/netlink"
	corev1 "k8s.io/api/core/v1"
	k8sErrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/wait"
)

var multusControllerCmd = &cobra.Command{
	Use:   "multus-hostnet",
	Short: "Runs a daemonset to ensure multus connectiviy is present on hostnet",
}
var multusSetupCmd = &cobra.Command{
	Use:   "multus-setup",
	Short: "Called by controller to run a job that migrates the multus interface into the host network namespace.",
}
var multusTeardownCmd = &cobra.Command{
	Use:   "multus-teardown",
	Short: "Called by controller to run a job that removes the migrated multus interface from the host network namespace.",
}

func init() {
	flags.SetFlagsFromEnv(multusControllerCmd.Flags(), rook.RookEnvVarPrefix)
	multusControllerCmd.RunE = multusJobController

	flags.SetFlagsFromEnv(multusSetupCmd.Flags(), rook.RookEnvVarPrefix)
	multusSetupCmd.RunE = multusSetup

	flags.SetFlagsFromEnv(multusTeardownCmd.Flags(), rook.RookEnvVarPrefix)
	multusTeardownCmd.RunE = multusTeardown
}

func multusJobController(cmd *cobra.Command, args []string) error {
	rook.SetLogLevel()
	rook.LogStartupInfo(multusControllerCmd.Flags())

	controllerName, found := os.LookupEnv("CONTROLLER_NAME")
	if !found {
		return errors.New("CONTROLLER_NAME environment variable not found")
	}
	controllerNamespace, found := os.LookupEnv("CONTROLLER_NAMESPACE")
	if !found {
		return errors.New("CONTROLLER_NAMESPACE environment variable not found")
	}

	// Set up signal handler, so that a clean up procedure will be run if the pod running this code is deleted.
	signalChan := make(chan os.Signal, 1)
	signal.Notify(signalChan, syscall.SIGTERM)

	k8sClient, err := multus.SetupK8sClient()
	if err != nil {
		return errors.Wrap(err, "failed to set up k8s client")
	}

	var jobParams multus.JobParameters
	// The pod may start running before the pod data is available to the api server.
	var controllerPod *corev1.Pod
	err = wait.Poll(time.Second, 20*time.Second, func() (bool, error) {
		var err error
		controllerPod, err = k8sClient.CoreV1().Pods(controllerNamespace).Get(context.TODO(), controllerName, metav1.GetOptions{})
		if err != nil {
			if k8sErrors.IsNotFound(err) {
				logger.Infof("Controller: %s in namespace %s not found. retrying...\n", controllerName, controllerNamespace)
				return false, nil
			} else {
				return true, err
			}
		}
		return true, nil
	})
	if err != nil {
		return errors.Wrap(err, "failed to get controller data")
	}

	err = jobParams.SetControllerParams(controllerPod)
	if err != nil {
		return errors.Wrap(err, "failed to set job parameters")
	}

	err = multus.RunSetupJob(k8sClient, jobParams)
	if err != nil {
		return errors.Wrap(err, "failed to run setup job")
	}

	// The setup job will have annotated the controller pod with the migrated interface.
	// This data is needed when removing the interface.
	controllerPod, err = k8sClient.CoreV1().Pods(controllerNamespace).Get(context.TODO(), controllerName, metav1.GetOptions{})
	if err != nil {
		return errors.Wrap(err, "failed to get controller data")
	}
	err = jobParams.SetMigratedInterfaceName(controllerPod)
	if err != nil {
		return errors.Wrap(err, "failed to get migrated interface name")
	}

	<-signalChan
	logger.Info("Running teardown job")
	logger.Infof("Removing multus interface %s", jobParams.MigratedInterface)
	err = multus.RunTeardownJob(k8sClient, jobParams)
	if err != nil {
		logger.Errorf("failed to run teardown job: %v", err)
		// Sleep so that the developer can view the log before the pod is destroyed.
		// Pods being deleted have a grace period of 30 seconds after the SIGTERM.
		time.Sleep(30 * time.Second)
	}

	return nil
}

func multusSetup(cmd *cobra.Command, args []string) error {
	rook.SetLogLevel()
	rook.LogStartupInfo(multusSetupCmd.Flags())

	holderIP, found := os.LookupEnv("HOLDER_IP")
	if !found {
		return errors.New("HOLDER_IP environment variable not found")
	}

	multusLinkName, found := os.LookupEnv("MULTUS_IFACE")
	if !found {
		return errors.New("MULTUS_IFACE environment variable not found")
	}
	logger.Infof("The multus interface to migrate is: %s", multusLinkName)

	controllerName, found := os.LookupEnv("CONTROLLER_NAME")
	if !found {
		return errors.New("CONTROLLER_NAME environment variable not found")
	}
	controllerNamespace, found := os.LookupEnv("CONTROLLER_NAMESPACE")
	if !found {
		return errors.New("CONTROLLER_NAMESPACE environment variable not found")
	}

	k8sClient, err := multus.SetupK8sClient()
	if err != nil {
		return errors.Wrap(err, "failed to set up k8s client")
	}

	holderNS, err := multus.DetermineNetNS(holderIP)
	if err != nil {
		return errors.Wrap(err, "failed to determine holder network namespace")
	}

	hostNS, err := ns.GetCurrentNS()
	if err != nil {
		return errors.Wrap(err, "failed to determine host network namespace")
	}

	interfaces, err := net.Interfaces()
	if err != nil {
		return errors.Wrap(err, "failed to get interfaces in hostnet")
	}
	newLinkName, err := multus.DetermineNewLinkName(interfaces)
	if err != nil {
		return errors.Wrap(err, "failed to determine new link name")
	}

	err = multus.AnnotateController(k8sClient, controllerName, controllerNamespace, newLinkName)
	if err != nil {
		return errors.Wrap(err, "failed to annotate controller")
	}

	netConfig, err := multus.GetNetworkConfig(holderNS, multusLinkName)
	if err != nil {
		return errors.Wrap(err, "failed to determine multus config")
	}

	err = multus.MigrateInterface(holderNS, hostNS, multusLinkName, newLinkName)
	if err != nil {
		return errors.Wrap(err, "failed to migrate multus interface")
	}

	err = multus.ConfigureInterface(hostNS, newLinkName, netConfig)
	if err != nil {
		return errors.Wrap(err, "failed to configure migrated interface")
	}

	return nil
}

// main code
func multusTeardown(cmd *cobra.Command, args []string) error {
	rook.SetLogLevel()
	rook.LogStartupInfo(multusTeardownCmd.Flags())

	iface, found := os.LookupEnv("MIGRATED_IFACE")
	if !found {
		return errors.New("MIGRATED_IFACE environment variable not found")
	}

	link, err := netlink.LinkByName(iface)
	if err != nil {
		return errors.Wrap(err, "failed to get multus network interface")
	}

	err = netlink.LinkDel(link)
	if err != nil {
		return errors.Wrap(err, "failed to delete multus network interface")
	}
	return nil
}
