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
package k8sutil

import (
	"encoding/json"
	"fmt"

	"k8s.io/client-go/1.5/pkg/api"
	unversionedAPI "k8s.io/client-go/1.5/pkg/api/unversioned"
	"k8s.io/client-go/1.5/pkg/api/v1"
)

func MakeRookImage(version string) string {
	return fmt.Sprintf("quay.io/rook/rookd:%v", version)
}

func PodWithAntiAffinity(pod *v1.Pod, clusterName string) *v1.Pod {
	// set pod anti-affinity with the pods that belongs to the same etcd cluster
	affinity := v1.Affinity{
		PodAntiAffinity: &v1.PodAntiAffinity{
			RequiredDuringSchedulingIgnoredDuringExecution: []v1.PodAffinityTerm{
				{
					LabelSelector: &unversionedAPI.LabelSelector{
						MatchLabels: map[string]string{
							"rook_cluster": clusterName,
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
	return pod
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
