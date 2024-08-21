/*
Copyright 2023 The Rook Authors. All rights reserved.

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

package validation

import (
	"context"
	"fmt"
	"os"
	"strconv"
	"time"

	"github.com/coreos/pkg/capnslog"
	"github.com/rook/rook/cmd/rook/rook"
	"github.com/rook/rook/pkg/daemon/multus"
	"github.com/spf13/cobra"
)

// config
var (
	validationConfigFile = ""
	validationConfig     = multus.ValidationTest{
		Logger: capnslog.NewPackageLogger("github.com/rook/rook", "multus-validation"),
	}

	// keep special var for `--daemons-per-node` that needs put into node config for validation run
	flagDaemonsPerNode = -1

	// keep special var for --host-check-only flag that can override what is from config file
	flagHostCheckOnly = false
)

// commands
var (
	// parent 'validation' command
	Cmd = &cobra.Command{
		Use:   "validation",
		Short: "Run and manage Multus validation tests for Rook",
	}

	// 'validation run' command
	runCmd = &cobra.Command{
		Use:   "run [--public-network=<nad-name>] [--cluster-network=<nad-name>]",
		Short: "Run a Multus validation test for Rook",
		Long: `
Run a validation test that determines whether the current Multus and system
configurations will support Rook with Multus.

This should be run BEFORE Rook is installed.

This is a fairly long-running test. It starts up a web server and many
clients to verify that Multus network communication works properly.

It does *not* perform any load testing. Networks that cannot support high
volumes of Ceph traffic may still encounter runtime issues. This may be
particularly noticeable with high I/O load or during OSD rebalancing
(see: https://docs.ceph.com/en/latest/architecture/#rebalancing).
For example, during Rook or Ceph cluster upgrade.

Override the kube config file location by setting the KUBECONFIG environment variable.
`,
		Run: func(cmd *cobra.Command, args []string) {
			validationConfig.Clientset = rook.GetInternalOrExternalClient()
			runValidation(cmd.Context())
		},
		Args: cobra.NoArgs,
	}

	// 'validation cleanup' command
	cleanupCmd = &cobra.Command{
		Use:   "cleanup",
		Short: "Clean up Multus validation test resources",
		Long: `
Clean up Multus validation test resources

Override the kube config file location by setting the KUBECONFIG environment variable.
`,
		Run: func(cmd *cobra.Command, args []string) {
			validationConfig.Clientset = rook.GetInternalOrExternalClient()
			runCleanup(cmd.Context())
		},
		Args: cobra.NoArgs,
	}
)

func init() {
	Cmd.AddCommand(runCmd)
	Cmd.AddCommand(cleanupCmd)
	Cmd.AddCommand(configCmd)

	defaultConfig := multus.NewDefaultValidationTestConfig()

	// flags on run/cleanup subcommands - makes output more straightforward than using PersistentFlags() global flags on parent
	for _, subCommand := range []*cobra.Command{runCmd, cleanupCmd} {
		subCommand.Flags().StringVarP(&validationConfig.Namespace, "namespace", "n", defaultConfig.Namespace,
			"The namespace for validation test resources. "+
				"It is recommended to set this to the namespace in which Rook's Ceph cluster will be installed.")

		// VarPF() keeps the the specific var passed to it for setting at runtime, and the current
		// val of that var when VarPF() is called is used as the default
		validationConfig.ResourceTimeout = defaultConfig.ResourceTimeout
		t := (*timeoutMinutes)(&validationConfig.ResourceTimeout)
		subCommand.Flags().VarPF(t, "timeout-minutes", "", /* no shorthand */
			"The time to wait for resources to change to the expected state. For example, for the "+
				"test web server to start, for test clients to become ready, or for test resources to be deleted. "+
				"At longest, this may need to reflect the time it takes for client pods to to pull images, "+
				"get address assignments, and then for each client to determine that its network connection is stable. "+
				"Minimum: 1 minute. Recommended: 2 minutes or more.")
	}

	// flags for 'validation run'
	runCmd.Flags().StringVar(&validationConfig.PublicNetwork, "public-network", defaultConfig.PublicNetwork,
		"The name of the Network Attachment Definition (NAD) that will be used for Ceph's public network. "+
			"This should be a namespaced name in the form <namespace>/<name> if the NAD is defined in a different namespace from the cluster namespace.")
	runCmd.Flags().StringVar(&validationConfig.ClusterNetwork, "cluster-network", defaultConfig.ClusterNetwork,
		"The name of the Network Attachment Definition (NAD) that will be used for Ceph's cluster network. "+
			"This should be a namespaced name in the form <namespace>/<name> if the NAD is defined in a different namespace from the cluster namespace.")
	runCmd.Flags().IntVar(&flagDaemonsPerNode, "daemons-per-node", defaultConfig.TotalDaemonsPerNode(),
		"The number of validation test daemons to run per node. "+
			"It is recommended to set this to the maximum number of Ceph daemons that can run on any node in the worst case of node failure(s). "+
			"The default value is set to the worst-case value for a Rook Ceph cluster with 3 portable OSDs, 3 portable monitors, "+
			"and where all optional child resources have been created with 1 daemon such that they all might run on a single node in a failure scenario. "+
			"If you aren't sure what to choose for this value, add 1 for each additional OSD beyond 3.")
	runCmd.Flags().StringVar(&validationConfig.ServiceAccountName, "service-account", defaultConfig.ServiceAccountName,
		"The name of the service account that will be used for test resources.")
	runCmd.Flags().BoolVar(&flagHostCheckOnly, "host-check-only", defaultConfig.HostCheckOnly,
		"Only check that hosts can connect to the server via the public network. Do not start clients. "+
			"This mode is recommended when a Rook cluster is already running and consuming the public network specified.")
	runCmd.Flags().StringVar(&validationConfig.NginxImage, "nginx-image", defaultConfig.NginxImage,
		"The Nginx image used for the validation server and clients.")

	validationConfig.FlakyThreshold = defaultConfig.FlakyThreshold
	f := (*timeoutSeconds)(&validationConfig.FlakyThreshold)
	runCmd.Flags().VarPF(f, "flaky-threshold-seconds", "",
		"This is the time window in which validation clients are all expected to become 'Ready' together. Validation clients are all started "+
			"at approximately the same time, and they should all stabilize at approximately the same time. Once the first validation client "+
			"becomes 'Ready', the tool checks that all of the remaining clients become 'Ready' before this threshold duration elapses. In networks "+
			"that have connectivity issues, limited bandwidth, or high latency, clients will contend for network traffic with each other, causing some "+
			"clients to randomly fail and become 'Ready' later than others. These randomly-failing clients are considered 'flaky.' Adjust this value "+
			"to reflect expectations for the underlying network. For fast and reliable networks, this can be set to a smaller value. For networks that "+
			"are intended to be slow, this can be set to a larger value. Additionally, for very large Kubernetes clusters, it may take longer for all "+
			"clients to start, and it therefore may take longer for all clients to become 'Ready'; in that case, this value can be set slightly higher.")

	runCmd.Flags().StringVarP(&validationConfigFile, "config", "c", "",
		"The validation test config file to use. This cannot be used with other flags except --host-check-only.")
	// allow using --host-check-only in combo with --config so the same config can be used with that flag if desired
	runCmd.MarkFlagsMutuallyExclusive("config", "timeout-minutes")
	runCmd.MarkFlagsMutuallyExclusive("config", "namespace")
	runCmd.MarkFlagsMutuallyExclusive("config", "public-network")
	runCmd.MarkFlagsMutuallyExclusive("config", "cluster-network")
	runCmd.MarkFlagsMutuallyExclusive("config", "daemons-per-node")
	runCmd.MarkFlagsMutuallyExclusive("config", "nginx-image")
	runCmd.MarkFlagsMutuallyExclusive("config", "flaky-threshold-seconds")

	// flags for 'validation cleanup'
	// none
}

func runValidation(ctx context.Context) {
	if validationConfigFile != "" {
		f, err := os.ReadFile(validationConfigFile)
		if err != nil {
			fmt.Printf("failed to read config file %q: %s\n", validationConfigFile, err)
			os.Exit(1)
		}
		c, err := multus.ValidationTestConfigFromYAML(string(f))
		if err != nil {
			fmt.Printf("failed to parse config file %q: %s\n", validationConfigFile, err)
			os.Exit(22 /* EINVAL */)
		}
		validationConfig.ValidationTestConfig = *c
	} else {
		// the default CLI test is simplified and assumes all Ceph daemons are OSDs, which get both
		// public and cluster network attachments. This also preserves legacy CLI behavior.
		validationConfig.NodeTypes = map[string]multus.NodeConfig{
			multus.DefaultValidationNodeType: {
				OSDsPerNode:         flagDaemonsPerNode,
				OtherDaemonsPerNode: 0,
			},
		}
	}

	// allow --host-check-only(=true) flag to override default/configfile settings
	if flagHostCheckOnly {
		validationConfig.HostCheckOnly = true
	}

	if err := validationConfig.ValidationTestConfig.Validate(); err != nil {
		fmt.Print(err.Error() + "\n")
		os.Exit(22 /* EINVAL */)
	}

	results, err := validationConfig.Run(ctx)
	report := results.SuggestedDebuggingReport()

	// success/failure message
	fmt.Print("\n")
	switch {
	case err != nil:
		fmt.Printf("RESULT: multus validation test failed: %v\n\n", err)
	case report == "":
		fmt.Print("RESULT: multus validation test succeeded!\n\n")
		runCleanup(ctx)
		os.Exit(0) // success!
	case report != "":
		// suggestions are bad
		fmt.Print("RESULT: multus validation test succeeded, but there are suggestions\n\n")
	}

	// output report suggestions
	fmt.Print(report + "\n")

	// help users help us help them
	fmt.Println("leaving multus validation test resources running for manual debugging")
	fmt.Print(`
For assistance debugging, collect the following into an archive file:
  - Output of this utility
  - Network Attachment Definitions (NADs) used by this test
  - A write-up describing the network configuration you are trying to achieve including the
      intended network for Ceph public/client traffic, intended network for Ceph cluster traffic,
      interface names and CIDRs for both networks, and any other details that are relevant.
  - 'ifconfig' output from at least one Kubernetes worker node
  - 'kubectl get pods -o wide' output from the test namespace
  - 'kubectl describe pods' output from the test namespace
  - 'kubectl get pods -o yaml' output from the test namespace
  - 'kubectl get daemonsets' output from the test namespace
  - 'kubectl describe daemonsets' output from the test namespace
  - 'kubectl get daemonsets -o yaml' output from the test namespace
  - 'kubectl logs multus-validation-test-web-server' output from the test namespace
  - 'kubectl get nodes -o wide' output
`)

	// tell them how to cleanup
	fmt.Printf("\nTo clean up resources when you are done debugging: %s --namespace %s\n", cleanupCmd.CommandPath(), validationConfig.Namespace)

	os.Exit(1)
}

func runCleanup(ctx context.Context) {
	fmt.Printf("cleaning up multus validation test resources in namespace %q\n", validationConfig.Namespace)
	results, err := validationConfig.CleanUp(ctx)
	if err != nil {
		fmt.Printf("multus validation test cleanup failed: %v\n\n", err)
		fmt.Println(results.SuggestedDebuggingReport())
		return
	}
	fmt.Print("multus validation test resources were successfully cleaned up\n")
}

// custom flag types

// implements pflag.Value interface to validate and set resource timeout and enforce nonzero
type timeoutMinutes time.Duration

func (t *timeoutMinutes) String() string { return time.Duration(*t).String() }
func (t *timeoutMinutes) Set(v string) error {
	i, err := strconv.Atoi(v)
	if err != nil {
		return err
	}
	if i < 1 {
		return fmt.Errorf("timeout must be greater than 0")
	}
	*t = timeoutMinutes(time.Duration(i) * time.Minute)
	return nil
}
func (t timeoutMinutes) Type() string {
	return "timeoutMinutes"
}

type timeoutSeconds time.Duration

func (t *timeoutSeconds) String() string { return time.Duration(*t).String() }
func (t *timeoutSeconds) Set(v string) error {
	i, err := strconv.Atoi(v)
	if err != nil {
		return err
	}
	if i < 1 {
		return fmt.Errorf("timeout must be greater than 0")
	}
	*t = timeoutSeconds(time.Duration(i) * time.Second)
	return nil
}
func (t timeoutSeconds) Type() string {
	return "timeoutSeconds"
}
