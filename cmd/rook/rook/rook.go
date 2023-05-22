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

package rook

import (
	"context"
	"fmt"
	"os"
	"reflect"
	"strings"

	"github.com/coreos/pkg/capnslog"
	netclient "github.com/k8snetworkplumbingwg/network-attachment-definition-client/pkg/client/clientset/versioned/typed/k8s.cni.cncf.io/v1"
	"github.com/pkg/errors"
	rookclient "github.com/rook/rook/pkg/client/clientset/versioned"
	"github.com/rook/rook/pkg/clusterd"
	"github.com/rook/rook/pkg/operator/k8sutil"
	"github.com/rook/rook/pkg/util"
	"github.com/rook/rook/pkg/util/exec"
	"github.com/rook/rook/pkg/util/flags"
	"github.com/rook/rook/pkg/version"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"

	v1 "k8s.io/api/core/v1"
	apiextensionsclient "k8s.io/apiextensions-apiserver/pkg/client/clientset/clientset"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
)

const (
	RookEnvVarPrefix = "ROOK"
	terminationLog   = "/dev/termination-log"
)

var RootCmd = &cobra.Command{
	Use:    "rook",
	Short:  "Rook (rook.io) Kubernetes operator and user tools",
	Hidden: false,
}

var (
	logLevelRaw string
	logger      = capnslog.NewPackageLogger("github.com/rook/rook", "rookcmd")
)

// Initialize the configuration parameters. The precedence from lowest to highest is:
//  1. default value (at compilation)
//  2. environment variables (upper case, replace - with _, and rook prefix. For example, discovery-url is ROOK_DISCOVERY_URL)
//  3. command line parameter
func init() {
	RootCmd.PersistentFlags().StringVar(&logLevelRaw, "log-level", "INFO", "logging level for logging/tracing output (valid values: ERROR,WARNING,INFO,DEBUG)")
	RootCmd.InitDefaultHelpCmd()
	RootCmd.InitDefaultHelpFlag()
	RootCmd.InitDefaultCompletionCmd()

	// load the environment variables
	flags.SetFlagsFromEnv(RootCmd.Flags(), RookEnvVarPrefix)
	flags.SetFlagsFromEnv(RootCmd.PersistentFlags(), RookEnvVarPrefix)
}

// SetLogLevel set log level based on provided log option.
func SetLogLevel() {
	util.SetGlobalLogLevel(logLevelRaw, logger)
}

// LogStartupInfo log the version number, arguments, and all final flag values (environment variable overrides have already been taken into account)
func LogStartupInfo(cmdFlags *pflag.FlagSet) {

	flagValues := flags.GetFlagsAndValues(cmdFlags, "secret|keyring")
	logger.Infof("starting Rook %s with arguments '%s'", version.Version, strings.Join(os.Args, " "))
	logger.Infof("flag values: %s", strings.Join(flagValues, ", "))
}

// NewContext creates and initializes a cluster context
func NewContext() *clusterd.Context {
	var err error

	context := &clusterd.Context{
		Executor:  &exec.CommandExecutor{},
		ConfigDir: k8sutil.DataDir,
	}

	// Try to read config from in-cluster env
	context.KubeConfig, err = rest.InClusterConfig()
	if err != nil {
		TerminateOnError(err, "failed to get k8s cluster config")
	}

	context.Clientset, err = kubernetes.NewForConfig(context.KubeConfig)
	TerminateOnError(err, "failed to create k8s clientset")

	context.RemoteExecutor.ClientSet = context.Clientset
	context.RemoteExecutor.RestClient = context.KubeConfig

	context.RookClientset, err = rookclient.NewForConfig(context.KubeConfig)
	TerminateOnError(err, "failed to create rook clientset")

	context.NetworkClient, err = netclient.NewForConfig(context.KubeConfig)
	TerminateOnError(err, "failed to create network clientset")

	context.ApiExtensionsClient, err = apiextensionsclient.NewForConfig(context.KubeConfig)
	TerminateOnError(err, "failed to create crd extensions client")

	return context
}

func GetOperatorImage(ctx context.Context, clientset kubernetes.Interface, containerName string) string {
	// Getting the info of the operator pod
	pod, err := k8sutil.GetRunningPod(ctx, clientset)
	TerminateOnError(err, "failed to get pod")

	// Get the actual operator container image name
	containerImage, err := k8sutil.GetContainerImage(pod, containerName)
	TerminateOnError(err, "failed to get container image")

	return containerImage
}

func GetOperatorServiceAccount(ctx context.Context, clientset kubernetes.Interface) string {
	// Getting the info of the operator pod
	pod, err := k8sutil.GetRunningPod(ctx, clientset)
	TerminateOnError(err, "failed to get pod")

	return pod.Spec.ServiceAccountName
}

func CheckOperatorResources(ctx context.Context, clientset kubernetes.Interface) {
	// Getting the info of the operator pod
	pod, err := k8sutil.GetRunningPod(ctx, clientset)
	TerminateOnError(err, "failed to get pod")
	resource := pod.Spec.Containers[0].Resources
	// set env var if operator pod resources are set
	if !reflect.DeepEqual(resource, (v1.ResourceRequirements{})) {
		os.Setenv("OPERATOR_RESOURCES_SPECIFIED", "true")
	}
}

// TerminateOnError terminates if err is not nil
func TerminateOnError(err error, msg string) {
	if err != nil {
		TerminateFatal(fmt.Errorf("%s: %+v", msg, err))
	}
}

// TerminateFatal terminates the process with an exit code of 1
// and writes the given reason to stderr and the termination log file.
func TerminateFatal(reason error) {
	file, err := os.OpenFile(terminationLog, os.O_APPEND|os.O_WRONLY, 0600)
	if err != nil {
		fmt.Fprintln(os.Stderr, fmt.Errorf("failed to write message to termination log: %v", err))
	} else {
		//nolint:gosec // Calling defer to close the file without checking the error return is not a risk for a simple file open and close
		defer file.Close()
		if _, err = file.WriteString(reason.Error()); err != nil {
			fmt.Fprintln(os.Stderr, fmt.Errorf("failed to write message to termination log: %v", err))
		}
		if err := file.Close(); err != nil {
			logger.Fatalf("failed to close file. %v", err)
		}
	}

	logger.Fatalln(reason)
}

// GetOperatorBaseImageCephVersion returns the Ceph version of the operator image
func GetOperatorBaseImageCephVersion(context *clusterd.Context) (string, error) {
	output, err := context.Executor.ExecuteCommandWithOutput("ceph", "--version")
	if err != nil {
		return "", errors.Wrapf(err, "failed to execute command to detect ceph version")
	}

	return output, nil
}

// GetInternalOrExternalClient will get a Kubernetes client interface from the KUBECONFIG variable
// if it is set, or from the operator pod environment otherwise.
func GetInternalOrExternalClient() kubernetes.Interface {
	kubeconfig := os.Getenv("KUBECONFIG")

	var (
		config *rest.Config
		err    error = fmt.Errorf("did not attempt to load a client") // just in case
	)

	if kubeconfig != "" {
		for _, kConf := range strings.Split(kubeconfig, ":") {
			config, err = clientcmd.BuildConfigFromFlags("", kConf)
			if err == nil {
				break
			}
		}
	} else {
		config, err = rest.InClusterConfig()
	}
	if err != nil {
		TerminateOnError(err, "error creating client")
	}

	client, err := kubernetes.NewForConfig(config)
	if err != nil {
		TerminateOnError(err, "could not get Kubernetes client interface")
	}
	return client
}
