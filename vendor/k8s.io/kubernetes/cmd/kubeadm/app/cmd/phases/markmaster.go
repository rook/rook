/*
Copyright 2017 The Kubernetes Authors.

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

package phases

import (
	"github.com/spf13/cobra"

	kubeadmscheme "k8s.io/kubernetes/cmd/kubeadm/app/apis/kubeadm/scheme"
	kubeadmapiv1alpha2 "k8s.io/kubernetes/cmd/kubeadm/app/apis/kubeadm/v1alpha2"
	"k8s.io/kubernetes/cmd/kubeadm/app/apis/kubeadm/validation"
	cmdutil "k8s.io/kubernetes/cmd/kubeadm/app/cmd/util"
	markmasterphase "k8s.io/kubernetes/cmd/kubeadm/app/phases/markmaster"
	kubeadmutil "k8s.io/kubernetes/cmd/kubeadm/app/util"
	configutil "k8s.io/kubernetes/cmd/kubeadm/app/util/config"
	kubeconfigutil "k8s.io/kubernetes/cmd/kubeadm/app/util/kubeconfig"
	"k8s.io/kubernetes/pkg/util/normalizer"
)

var (
	markMasterLongDesc = normalizer.LongDesc(`
		Applies a label that specifies that a node is a master and a taint that forces workloads to be deployed accordingly.
		` + cmdutil.AlphaDisclaimer)

	markMasterExample = normalizer.Examples(`
		# Applies master label and taint to the current node, functionally equivalent to what executed by kubeadm init.
		kubeadm alpha phase mark-master

		# Applies master label and taint to a specific node
		kubeadm alpha phase mark-master --node-name myNode
		`)
)

// NewCmdMarkMaster returns the Cobra command for running the mark-master phase
func NewCmdMarkMaster() *cobra.Command {

	cfg := &kubeadmapiv1alpha2.MasterConfiguration{
		// KubernetesVersion is not used by mark master, but we set this explicitly to avoid
		// the lookup of the version from the internet when executing ConfigFileAndDefaultsToInternalConfig
		KubernetesVersion: "v1.10.0",
	}

	// Default values for the cobra help text
	kubeadmscheme.Scheme.Default(cfg)

	var cfgPath, kubeConfigFile string
	cmd := &cobra.Command{
		Use:     "mark-master",
		Short:   "Mark a node as master",
		Long:    markMasterLongDesc,
		Example: markMasterExample,
		Aliases: []string{"markmaster"},
		Run: func(cmd *cobra.Command, args []string) {
			if err := validation.ValidateMixedArguments(cmd.Flags()); err != nil {
				kubeadmutil.CheckErr(err)
			}

			// This call returns the ready-to-use configuration based on the configuration file that might or might not exist and the default cfg populated by flags
			internalcfg, err := configutil.ConfigFileAndDefaultsToInternalConfig(cfgPath, cfg)
			kubeadmutil.CheckErr(err)

			client, err := kubeconfigutil.ClientSetFromFile(kubeConfigFile)
			kubeadmutil.CheckErr(err)

			err = markmasterphase.MarkMaster(client, internalcfg.NodeRegistration.Name, internalcfg.NodeRegistration.Taints)
			kubeadmutil.CheckErr(err)
		},
	}

	cmd.Flags().StringVar(&kubeConfigFile, "kubeconfig", "/etc/kubernetes/admin.conf", "The KubeConfig file to use when talking to the cluster")
	cmd.Flags().StringVar(&cfgPath, "config", cfgPath, "Path to kubeadm config file. WARNING: Usage of a configuration file is experimental")
	cmd.Flags().StringVar(&cfg.NodeRegistration.Name, "node-name", cfg.NodeRegistration.Name, `The node name to which label and taints should apply`)

	return cmd
}
