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
package topology

import (
	"fmt"
	"strings"

	"github.com/coreos/pkg/capnslog"
	"github.com/rook/rook/pkg/daemon/ceph/client"
	"github.com/rook/rook/pkg/operator/k8sutil"
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

	logger = capnslog.NewPackageLogger("github.com/rook/rook", "osd-topology")
)

const (
	topologyLabelPrefix = "topology.rook.io/"
)

// ExtractOSDTopologyFromLabels extracts rook topology from labels and returns a map from topology type to value
func ExtractOSDTopologyFromLabels(labels map[string]string) (map[string]string, string) {
	topology, topologyAffinity := extractTopologyFromLabels(labels)

	// Ensure the topology names are normalized for CRUSH
	for name, value := range topology {
		topology[name] = client.NormalizeCrushName(value)
	}
	return topology, topologyAffinity
}

func rookTopologyLabelsOrdered() []string {
	topologyLabelsOrdered := []string{}
	for i := len(CRUSHTopologyLabels) - 1; i >= 0; i-- {
		label := CRUSHTopologyLabels[i]
		topologyLabelsOrdered = append(topologyLabelsOrdered, topologyLabelPrefix+label)
	}
	return topologyLabelsOrdered
}

func allKubernetesTopologyLabelsOrdered() []string {
	return append(
		append([]string{
			corev1.LabelTopologyRegion,
			corev1.LabelTopologyZone,
		},
			rookTopologyLabelsOrdered()...),
		k8sutil.LabelHostname(), //  host is the lowest level in the crush map hierarchy
	)
}

func kubernetesTopologyLabelToCRUSHLabel(label string) string {
	if label == k8sutil.LabelHostname() {
		return "host"
	}
	crushLabel := strings.Split(label, "/")
	return crushLabel[len(crushLabel)-1]
}

// ExtractTopologyFromLabels extracts rook topology from labels and returns a map from topology type to value
func extractTopologyFromLabels(labels map[string]string) (map[string]string, string) {
	topology := make(map[string]string)

	// The topology affinity for the osd is the lowest topology label found in the hierarchy,
	// not including the host name
	var topologyAffinity string
	allKubernetesTopologyLabels := allKubernetesTopologyLabelsOrdered()

	// get the labels for the CRUSH map hierarchy
	// iterate in a way so the last topology found will be the lowest level in the hierarchy
	// for the topology affinity
	for _, label := range allKubernetesTopologyLabels {
		topologyID := kubernetesTopologyLabelToCRUSHLabel(label)
		if value, ok := labels[label]; ok {
			topology[topologyID] = value
			if topologyID != "host" {
				topologyAffinity = formatTopologyAffinity(label, value)
			}
		}
	}
	// iterate in lowest to highest order as the lowest level should be sustained and higher level duplicate
	// should be removed
	duplicateTopology := make(map[string][]string)
	for i := len(allKubernetesTopologyLabels) - 1; i >= 0; i-- {
		topologyLabel := allKubernetesTopologyLabels[i]
		if value, ok := labels[topologyLabel]; ok {
			if _, ok := duplicateTopology[value]; ok {
				delete(topology, kubernetesTopologyLabelToCRUSHLabel(topologyLabel))
			}
			duplicateTopology[value] = append(duplicateTopology[value], topologyLabel)
		}
	}

	// remove non-duplicate entries, and report if any duplicate entries were found
	for value, duplicateKeys := range duplicateTopology {
		if len(duplicateKeys) <= 1 {
			delete(duplicateTopology, value)
		}
	}
	if len(duplicateTopology) != 0 {
		logger.Warningf("Found duplicate location values with labels: %v", duplicateTopology)
	}

	return topology, topologyAffinity
}

func formatTopologyAffinity(label, value string) string {
	return fmt.Sprintf("%s=%s", label, value)
}

// GetDefaultTopologyLabels returns the supported default topology labels.
func GetDefaultTopologyLabels() string {
	Labels := []string{k8sutil.LabelHostname(), corev1.LabelZoneRegionStable, corev1.LabelZoneFailureDomainStable}
	for _, label := range CRUSHTopologyLabels {
		Labels = append(Labels, topologyLabelPrefix+label)
	}

	return strings.Join(Labels, ",")
}

// CheckTopologyConflicts verifies that:
// 1. No child domain (e.g. rack) has fewer distinct values than its parent (e.g. zone).
// 2. No topology value is used under more than one label key.
func CheckTopologyConflicts(nodes []corev1.Node) error {
	// 1. Build our ordered list of topology labels (region -> zone -> rack -> …), dropping hostname.
	allLabels := allKubernetesTopologyLabelsOrdered()
	var topologyHierarchy []string
	for _, lbl := range allLabels {
		if lbl != k8sutil.LabelHostname() {
			topologyHierarchy = append(topologyHierarchy, lbl)
		}
	}

	// 2. Gather distinct values for each topology key.
	topologyValues := make(map[string]map[string]struct{}, len(topologyHierarchy))
	for _, key := range topologyHierarchy {
		topologyValues[key] = make(map[string]struct{})
	}
	for _, node := range nodes {
		labels := node.GetLabels()
		for _, key := range topologyHierarchy {
			v := client.NormalizeCrushName(labels[key])
			if v == "" {
				continue
			}
			topologyValues[key][v] = struct{}{}
		}
	}

	// 3. Parent-child count check: each child domain must have bigger or equal values than its parent.
	for i, parentKey := range topologyHierarchy[:len(topologyHierarchy)-1] {
		parentCount := len(topologyValues[parentKey])
		for _, childKey := range topologyHierarchy[i+1:] {
			childCount := len(topologyValues[childKey])
			if childCount > 0 && childCount < parentCount {
				return fmt.Errorf(
					"invalid topology: parent %q has %d values but child %q has %d",
					parentKey, parentCount, childKey, childCount,
				)
			}
		}
	}

	// 4. Cross-key uniqueness: no value reused across multiple label keys.
	valueToFirstKey := make(map[string]string)
	for key, vals := range topologyValues {
		for v := range vals {
			if firstKey, ok := valueToFirstKey[v]; ok {
				return fmt.Errorf(
					"invalid topology: value %q appears under both %q and %q",
					v, firstKey, key,
				)
			}
			valueToFirstKey[v] = key
		}
	}

	// 5. All checks passed.
	return nil
}
