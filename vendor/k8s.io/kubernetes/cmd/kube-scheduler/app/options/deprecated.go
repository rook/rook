/*
Copyright 2018 The Kubernetes Authors.

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

package options

import (
	"fmt"

	"github.com/spf13/pflag"

	"k8s.io/kubernetes/pkg/apis/componentconfig"
	"k8s.io/kubernetes/pkg/scheduler/factory"
)

// DeprecatedOptions contains deprecated options and their flags.
// TODO remove these fields once the deprecated flags are removed.
type DeprecatedOptions struct {
	// The fields below here are placeholders for flags that can't be directly
	// mapped into componentconfig.KubeSchedulerConfiguration.
	PolicyConfigFile         string
	PolicyConfigMapName      string
	PolicyConfigMapNamespace string
	UseLegacyPolicyConfig    bool
	AlgorithmProvider        string
}

// AddFlags adds flags for the deprecated options.
func (o *DeprecatedOptions) AddFlags(fs *pflag.FlagSet, cfg *componentconfig.KubeSchedulerConfiguration) {
	if o == nil {
		return
	}

	// TODO: unify deprecation mechanism, string prefix or MarkDeprecated (the latter hides the flag. We also don't want that).

	fs.StringVar(&o.AlgorithmProvider, "algorithm-provider", o.AlgorithmProvider, "DEPRECATED: the scheduling algorithm provider to use, one of: "+factory.ListAlgorithmProviders())
	fs.StringVar(&o.PolicyConfigFile, "policy-config-file", o.PolicyConfigFile, "DEPRECATED: file with scheduler policy configuration. This file is used if policy ConfigMap is not provided or --use-legacy-policy-config=true")
	usage := fmt.Sprintf("DEPRECATED: name of the ConfigMap object that contains scheduler's policy configuration. It must exist in the system namespace before scheduler initialization if --use-legacy-policy-config=false. The config must be provided as the value of an element in 'Data' map with the key='%v'", componentconfig.SchedulerPolicyConfigMapKey)
	fs.StringVar(&o.PolicyConfigMapName, "policy-configmap", o.PolicyConfigMapName, usage)
	fs.StringVar(&o.PolicyConfigMapNamespace, "policy-configmap-namespace", o.PolicyConfigMapNamespace, "DEPRECATED: the namespace where policy ConfigMap is located. The kube-system namespace will be used if this is not provided or is empty.")
	fs.BoolVar(&o.UseLegacyPolicyConfig, "use-legacy-policy-config", o.UseLegacyPolicyConfig, "DEPRECATED: when set to true, scheduler will ignore policy ConfigMap and uses policy config file")

	fs.BoolVar(&cfg.EnableProfiling, "profiling", cfg.EnableProfiling, "DEPRECATED: enable profiling via web interface host:port/debug/pprof/")
	fs.BoolVar(&cfg.EnableContentionProfiling, "contention-profiling", cfg.EnableContentionProfiling, "DEPRECATED: enable lock contention profiling, if profiling is enabled")
	fs.StringVar(&cfg.ClientConnection.KubeConfigFile, "kubeconfig", cfg.ClientConnection.KubeConfigFile, "DEPRECATED: path to kubeconfig file with authorization and master location information.")
	fs.StringVar(&cfg.ClientConnection.ContentType, "kube-api-content-type", cfg.ClientConnection.ContentType, "DEPRECATED: content type of requests sent to apiserver.")
	fs.Float32Var(&cfg.ClientConnection.QPS, "kube-api-qps", cfg.ClientConnection.QPS, "DEPRECATED: QPS to use while talking with kubernetes apiserver")
	fs.Int32Var(&cfg.ClientConnection.Burst, "kube-api-burst", cfg.ClientConnection.Burst, "DEPRECATED: burst to use while talking with kubernetes apiserver")
	fs.StringVar(&cfg.SchedulerName, "scheduler-name", cfg.SchedulerName, "DEPRECATED: name of the scheduler, used to select which pods will be processed by this scheduler, based on pod's \"spec.schedulerName\".")
	fs.StringVar(&cfg.LeaderElection.LockObjectNamespace, "lock-object-namespace", cfg.LeaderElection.LockObjectNamespace, "DEPRECATED: define the namespace of the lock object.")
	fs.StringVar(&cfg.LeaderElection.LockObjectName, "lock-object-name", cfg.LeaderElection.LockObjectName, "DEPRECATED: define the name of the lock object.")

	fs.Int32Var(&cfg.HardPodAffinitySymmetricWeight, "hard-pod-affinity-symmetric-weight", cfg.HardPodAffinitySymmetricWeight,
		"RequiredDuringScheduling affinity is not symmetric, but there is an implicit PreferredDuringScheduling affinity rule corresponding "+
			"to every RequiredDuringScheduling affinity rule. --hard-pod-affinity-symmetric-weight represents the weight of implicit PreferredDuringScheduling affinity rule.")
	fs.MarkDeprecated("hard-pod-affinity-symmetric-weight", "This option was moved to the policy configuration file")
	fs.StringVar(&cfg.FailureDomains, "failure-domains", cfg.FailureDomains, "Indicate the \"all topologies\" set for an empty topologyKey when it's used for PreferredDuringScheduling pod anti-affinity.")
	fs.MarkDeprecated("failure-domains", "Doesn't have any effect. Will be removed in future version.")
}

// Validate validates the deprecated scheduler options.
func (o *DeprecatedOptions) Validate() []error {
	if o == nil {
		return nil
	}
	return nil
}

// ApplyTo sets cfg.AlgorithmSource from flags passed on the command line in the following precedence order:
//
// 1. --use-legacy-policy-config to use a policy file.
// 2. --policy-configmap to use a policy config map value.
// 3. --algorithm-provider to use a named algorithm provider.
func (o *DeprecatedOptions) ApplyTo(cfg *componentconfig.KubeSchedulerConfiguration) error {
	if o == nil {
		return nil
	}

	switch {
	case o.UseLegacyPolicyConfig || (len(o.PolicyConfigFile) > 0 && o.PolicyConfigMapName == ""):
		cfg.AlgorithmSource = componentconfig.SchedulerAlgorithmSource{
			Policy: &componentconfig.SchedulerPolicySource{
				File: &componentconfig.SchedulerPolicyFileSource{
					Path: o.PolicyConfigFile,
				},
			},
		}
	case len(o.PolicyConfigMapName) > 0:
		cfg.AlgorithmSource = componentconfig.SchedulerAlgorithmSource{
			Policy: &componentconfig.SchedulerPolicySource{
				ConfigMap: &componentconfig.SchedulerPolicyConfigMapSource{
					Name:      o.PolicyConfigMapName,
					Namespace: o.PolicyConfigMapNamespace,
				},
			},
		}
	case len(o.AlgorithmProvider) > 0:
		cfg.AlgorithmSource = componentconfig.SchedulerAlgorithmSource{
			Provider: &o.AlgorithmProvider,
		}
	}

	return nil
}
