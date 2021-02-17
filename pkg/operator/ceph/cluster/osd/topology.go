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

	"github.com/rook/rook/pkg/daemon/ceph/client"
	corev1 "k8s.io/api/core/v1"
)

var (

	// The labels that can be specified with the K8s labels such as topology.kubernetes.io/zone
	// These are all at the top layers of the CRUSH map.
	KubernetesTopologyLabels = []string{"zone", "region"}

	// The node labels that are supported with the topology.rook.io prefix such as topology.rook.io/rack
	// The labels are in order from lowest to highest in the CRUSH hierarchy
	CRUSHTopologyLabels = []string{"chassis", "rack", "row", "pdu", "pod", "room", "datacenter"}

	// The list of supported failure domains in the CRUSH map, ordered from lowest to highest
	CRUSHMapLevelsOrdered = append([]string{"host"}, append(CRUSHTopologyLabels, KubernetesTopologyLabels...)...)
)

const (
	topologyLabelPrefix = "topology.rook.io/"
)

// ExtractTopologyFromLabels extracts rook topology from labels and returns a map from topology type to value
func ExtractOSDTopologyFromLabels(labels map[string]string) (map[string]string, string) {
	topology, topologyAffinity := extractTopologyFromLabels(labels)

	// Ensure the topology names are normalized for CRUSH
	for name, value := range topology {
		topology[name] = client.NormalizeCrushName(value)
	}
	return topology, topologyAffinity
}

// ExtractTopologyFromLabels extracts rook topology from labels and returns a map from topology type to value
func extractTopologyFromLabels(labels map[string]string) (map[string]string, string) {
	topology := make(map[string]string)

	// The topology affinity for the osd is the lowest topology label found in the hierarchy,
	// not including the host name
	var topologyAffinity string

	// check for the region k8s topology label that was deprecated in 1.17
	const regionLabel = "region"
	region, ok := labels[corev1.LabelZoneRegion]
	if ok {
		topology[regionLabel] = region
		topologyAffinity = formatTopologyAffinity(corev1.LabelZoneRegion, region)
	}

	// check for the region k8s topology label that is GA in 1.17.
	region, ok = labels[corev1.LabelZoneRegionStable]
	if ok {
		topology[regionLabel] = region
		topologyAffinity = formatTopologyAffinity(corev1.LabelZoneRegionStable, region)
	}

	// check for the zone k8s topology label that was deprecated in 1.17
	const zoneLabel = "zone"
	zone, ok := labels[corev1.LabelZoneFailureDomain]
	if ok {
		topology[zoneLabel] = zone
		topologyAffinity = formatTopologyAffinity(corev1.LabelZoneFailureDomain, zone)
	}

	// check for the zone k8s topology label that is GA in 1.17.
	zone, ok = labels[corev1.LabelZoneFailureDomainStable]
	if ok {
		topology[zoneLabel] = zone
		topologyAffinity = formatTopologyAffinity(corev1.LabelZoneFailureDomainStable, zone)
	}

	// get host
	host, ok := labels[corev1.LabelHostname]
	if ok {
		topology["host"] = host
	}

	// get the labels for the CRUSH map hierarchy
	// iterate in reverse order so that the last topology found will be the lowest level in the hierarchy
	// for the topology affinity
	for i := len(CRUSHTopologyLabels) - 1; i >= 0; i-- {
		topologyID := CRUSHTopologyLabels[i]
		label := topologyLabelPrefix + topologyID
		if value, ok := labels[label]; ok {
			topology[topologyID] = value
			topologyAffinity = formatTopologyAffinity(label, value)
		}
	}
	return topology, topologyAffinity
}

func formatTopologyAffinity(label, value string) string {
	return fmt.Sprintf("%s=%s", label, value)
}
