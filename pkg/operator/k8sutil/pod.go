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

Some of the code below came from https://github.com/coreos/etcd-operator
which also has the apache 2.0 license.
*/
package k8sutil

import (
	"encoding/json"
	"fmt"
	"os"

	"k8s.io/client-go/pkg/api"
	unversionedAPI "k8s.io/client-go/pkg/api/unversioned"
	"k8s.io/client-go/pkg/api/v1"
)

const (
	AppAttr           = "app"
	ClusterAttr       = "rook_cluster"
	VersionAttr       = "rook_version"
	PodIPEnvVar       = "ROOKD_PRIVATE_IPV4"
	DefaultRepoPrefix = "quay.io/rook"
	repoPrefixEnvVar  = "ROOKD_REPO_PREFIX"
	defaultVersion    = "latest"
)

func NamespaceEnvVar() v1.EnvVar {
	return v1.EnvVar{Name: "ROOKD_NAMESPACE", ValueFrom: &v1.EnvVarSource{FieldRef: &v1.ObjectFieldSelector{FieldPath: "metadata.namespace"}}}
}

func RepoPrefixEnvVar() v1.EnvVar {
	return v1.EnvVar{Name: repoPrefixEnvVar, Value: repoPrefix()}
}

func ConfigDirEnvVar() v1.EnvVar {
	return v1.EnvVar{Name: "ROOKD_CONFIG_DIR", Value: DataDir}
}

func repoPrefix() string {
	r := os.Getenv(repoPrefixEnvVar)
	if r == "" {
		r = DefaultRepoPrefix
	}
	return r
}

func getVersion(version string) string {
	if version == "" {
		version = defaultVersion
	}

	return version
}

func MakeRookImage(version string) string {
	return fmt.Sprintf("%s/rookd:%v", repoPrefix(), getVersion(version))
}

func PodWithAntiAffinity(pod *v1.Pod, attribute, value string) {
	// set pod anti-affinity with the pods that belongs to the same rook cluster
	affinity := v1.Affinity{
		PodAntiAffinity: &v1.PodAntiAffinity{
			RequiredDuringSchedulingIgnoredDuringExecution: []v1.PodAffinityTerm{
				{
					LabelSelector: &unversionedAPI.LabelSelector{
						MatchLabels: map[string]string{
							attribute: value,
						},
					},
					TopologyKey: "kubernetes.io/hostname",
				},
			},
		},
	}

	affinityb, err := json.Marshal(affinity)
	if err != nil {
		panic("failed to marshal affinty struct")
	}

	pod.Annotations[api.AffinityAnnotationKey] = string(affinityb)
}

func SetPodVersion(pod *v1.Pod, key, version string) {
	pod.Annotations[key] = version
}

func GetPodNames(pods []*api.Pod) []string {
	res := []string{}
	for _, p := range pods {
		res = append(res, p.Name)
	}
	return res
}
