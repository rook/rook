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
	"github.com/rook/rook/pkg/util/exec"
	"github.com/rook/rook/pkg/util/flags"
	"github.com/rook/rook/pkg/version"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
	v1 "k8s.io/api/core/v1"
	apiextensionsclient "k8s.io/apiextensions-apiserver/pkg/client/clientset/clientset"
	"k8s.io/apimachinery/pkg/util/uuid"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
)

const (
	RookEnvVarPrefix = "ROOK"
	terminationLog   = "/dev/termination-log"
)

var RootCmd = &cobra.Command{
	Use: "rook",
}

var (
	logLevelRaw        string
	operatorImage      string
	serviceAccountName string
	Cfg                = &Config{}
	logger             = capnslog.NewPackageLogger("github.com/rook/rook", "rookcmd")
)

type Config struct {
	LogLevel capnslog.LogLevel
}

// Initialize the configuration parameters. The precedence from lowest to highest is:
//  1) default value (at compilation)
//  2) environment variables (upper case, replace - with _, and rook prefix. For example, discovery-url is ROOK_DISCOVERY_URL)
//  3) command line parameter
func init() {
	RootCmd.PersistentFlags().StringVar(&logLevelRaw, "log-level", "INFO", "logging level for logging/tracing output (valid values: CRITICAL,ERROR,WARNING,NOTICE,INFO,DEBUG,TRACE)")
	RootCmd.PersistentFlags().StringVar(&operatorImage, "operator-image", "", "Override the image url that the operator uses. The default is read from the operator pod.")
	RootCmd.PersistentFlags().StringVar(&serviceAccountName, "service-account", "", "Override the service account that the operator uses. The default is read from the operator pod.")

	// load the environment variables
	flags.SetFlagsFromEnv(RootCmd.Flags(), RookEnvVarPrefix)
	flags.SetFlagsFromEnv(RootCmd.PersistentFlags(), RookEnvVarPrefix)
}

// SetLogLevel set log level based on provided log option.
func SetLogLevel() {
	// parse given log level string then set up corresponding global logging level
	ll, err := capnslog.ParseLevel(logLevelRaw)
	if err != nil {
		logger.Warningf("failed to set log level %s. %+v", logLevelRaw, err)
	}
	Cfg.LogLevel = ll
	capnslog.SetGlobalLogLevel(Cfg.LogLevel)
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
		LogLevel:  Cfg.LogLevel,
	}

	// Try to read config from in-cluster env
	context.KubeConfig, err = rest.InClusterConfig()
	if err != nil {

		// **Not** running inside a cluster - running the operator outside of the cluster.
		// This mode is for developers running the operator on their dev machines
		// for faster development, or to run operator cli tools manually to a remote cluster.
		// We setup the API server config from default user file locations (most notably ~/.kube/config),
		// and also change the executor to work remotely and run kubernetes jobs.
		logger.Info("setting up the context to outside of the cluster")

		// Try to read config from user config files
		context.KubeConfig, err = clientcmd.NewNonInteractiveDeferredLoadingClientConfig(
			clientcmd.NewDefaultClientConfigLoadingRules(),
			&clientcmd.ConfigOverrides{}).ClientConfig()
		TerminateOnError(err, "failed to get k8s config")

		// When running outside, we need to setup an executor that runs the commands as kubernetes jobs.
		// This allows the operator code to execute tools that are available in the operator image
		// or just have to be run inside the cluster in order to talk to the pods and services directly.
		context.Executor = &exec.TranslateCommandExecutor{
			Executor: context.Executor,
			Translator: func(
				command string,
				arg ...string,
			) (string, []string) {
				jobName := "rook-exec-job-" + string(uuid.NewUUID())
				transCommand := "kubectl"
				transArgs := append([]string{
					"run", jobName,
					"--image=" + operatorImage,
					"--serviceaccount=" + serviceAccountName,
					"--restart=Never",
					"--attach",
					"--rm",
					"--quiet",
					"--command", "--",
					command}, arg...)
				return transCommand, transArgs
			},
		}
	}

	context.Clientset, err = kubernetes.NewForConfig(context.KubeConfig)
	TerminateOnError(err, "failed to create k8s clientset")

	context.RemoteExecutor.ClientSet = context.Clientset
	context.RemoteExecutor.RestClient = context.KubeConfig

	// Dynamic clientset allows dealing with resources that aren't statically typed but determined
	// at runtime.
	context.DynamicClientset, err = dynamic.NewForConfig(context.KubeConfig)
	TerminateOnError(err, "failed to create dynamic clientset")

	context.APIExtensionClientset, err = apiextensionsclient.NewForConfig(context.KubeConfig)
	TerminateOnError(err, "failed to create k8s API extension clientset")

	context.RookClientset, err = rookclient.NewForConfig(context.KubeConfig)
	TerminateOnError(err, "failed to create rook clientset")

	context.NetworkClient, err = netclient.NewForConfig(context.KubeConfig)
	TerminateOnError(err, "failed to create network clientset")

	return context
}

func GetOperatorImage(clientset kubernetes.Interface, containerName string) string {

	// If provided as a flag then use that value
	if operatorImage != "" {
		return operatorImage
	}

	// Getting the info of the operator pod
	pod, err := k8sutil.GetRunningPod(clientset)
	TerminateOnError(err, "failed to get pod")

	// Get the actual operator container image name
	containerImage, err := k8sutil.GetContainerImage(pod, containerName)
	TerminateOnError(err, "failed to get container image")

	return containerImage
}

func GetOperatorServiceAccount(clientset kubernetes.Interface) string {

	// If provided as a flag then use that value
	if serviceAccountName != "" {
		return serviceAccountName
	}

	// Getting the info of the operator pod
	pod, err := k8sutil.GetRunningPod(clientset)
	TerminateOnError(err, "failed to get pod")

	return pod.Spec.ServiceAccountName
}

func CheckOperatorResources(clientset kubernetes.Interface) {
	// Getting the info of the operator pod
	pod, err := k8sutil.GetRunningPod(clientset)
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
		// #nosec G307 Calling defer to close the file without checking the error return is not a risk for a simple file open and close
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
