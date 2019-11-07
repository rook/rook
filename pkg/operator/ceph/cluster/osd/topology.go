/*
Copyright 2019 The Rook Authors. All rights reserved.

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

// Package config provides methods for generating the Ceph config for a Ceph cluster and for
// producing a "ceph.conf" compatible file from the config as well as Ceph command line-compatible
// flags.
package osd

import (
	"fmt"
	"strings"

	"github.com/rook/rook/pkg/daemon/ceph/client"
	"github.com/rook/rook/pkg/operator/k8sutil"

	corev1 "k8s.io/api/core/v1"
)

var (

	// The labels that can be specified with the K8s labels such as failure-domain.beta.kubernetes.io/zone
	// These are all at the top layers of the CRUSH map.
	KubernetesTopologyLabels = []string{"zone", "region"}

	// The node labels that are supported with the topology.rook.io prefix such as topology.rook.io/rack
	CRUSHTopologyLabels = []string{"chassis", "rack", "row", "pdu", "pod", "room", "datacenter"}

	// The list of supported failure domains in the CRUSH map, ordered from lowest to highest
	CRUSHMapLevelsOrdered = append([]string{"host"}, append(CRUSHTopologyLabels, KubernetesTopologyLabels...)...)
)

// ExtractRookTopologyFromLabels extracts rook topology from labels and returns a map from topology type to value,
// and an array of any invalid labels with a topology prefix.
func ExtractRookTopologyFromLabels(labels map[string]string) (map[string]string, []string) {
	topology := make(map[string]string)

	// get zone
	zone, ok := labels[corev1.LabelZoneFailureDomain]
	if ok {
		topology["zone"] = client.NormalizeCrushName(zone)
	}
	// get region
	region, ok := labels[corev1.LabelZoneRegion]
	if ok {
		topology["region"] = client.NormalizeCrushName(region)
	}

	// get host
	host, ok := labels[corev1.LabelHostname]
	if ok {
		topology["host"] = client.NormalizeCrushName(host)
	}

	invalidEncountered := make([]string, 0)
	for labelKey, labelValue := range labels {
		for _, validTopologyType := range CRUSHTopologyLabels {
			if strings.HasPrefix(labelKey, k8sutil.TopologyLabelPrefix) {
				s := strings.Split(labelKey, "/")
				if len(s) != 2 {
					invalidEncountered = append(invalidEncountered, fmt.Sprintf("%s=%s", labelKey, labelValue))
					continue
				}
				topologyType := s[1]
				if topologyType == validTopologyType {
					topology[validTopologyType] = client.NormalizeCrushName(labelValue)
				}
			}
		}
	}
	return topology, invalidEncountered
}
