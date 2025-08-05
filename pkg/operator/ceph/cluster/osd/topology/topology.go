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
	"sort"
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
		if value, ok := labels[label]; ok && value != "" {
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
// 1. No child domain (e.g. rack) has fewer distinct values than its immediate parent.
// 2. No topology value is used under more than one label key.
// 3. No label value lives under more than one parent label value.
func CheckTopologyConflicts(nodes *[]corev1.Node) error {
	// 1. Build our ordered list of topology labels (region -> zone -> datacenter -> …), dropping hostname.
	allLabels := allKubernetesTopologyLabelsOrdered()
	var hierarchy []string
	for _, label := range allLabels {
		if label != k8sutil.LabelHostname() {
			hierarchy = append(hierarchy, label)
		}
	}

	// 2. Gather distinct values for each topology key.
	values := make(map[string]map[string]struct{}, len(hierarchy))
	for _, key := range hierarchy {
		values[key] = make(map[string]struct{})
	}
	for _, node := range *nodes {
		labels := node.GetLabels()
		for _, key := range hierarchy {
			// If the label exists but has an empty value, consider it invalid
			if val, ok := labels[key]; ok && val == "" {
				return fmt.Errorf(
					"invalid topology: label %q has an empty value on node %q",
					key, node.Name,
				)
			}
			// Collect non-empty normalized values
			if crushName := client.NormalizeCrushName(labels[key]); crushName != "" {
				values[key][crushName] = struct{}{}
			}
		}
	}

	// 3. Parent‑consistency: each child‑value only ever under one parent‑value.
	for levelIndex, key := range hierarchy {
		if levelIndex == 0 {
			continue // no parent
		}
		for topologyValue := range values[key] {
			seenParents := map[string]struct{}{}
			for _, node := range *nodes {
				labels := node.GetLabels()
				if client.NormalizeCrushName(labels[key]) != topologyValue {
					continue
				}
				// find nearest non-empty parent label
				for parentIndex := levelIndex - 1; parentIndex >= 0; parentIndex-- {
					parKey := hierarchy[parentIndex]
					pv := client.NormalizeCrushName(labels[parKey])
					if pv != "" {
						seenParents[pv] = struct{}{}
						break
					}
				}
			}
			if len(seenParents) > 1 {
				ps := make([]string, 0, len(seenParents))
				for p := range seenParents {
					ps = append(ps, p)
				}
				sort.Strings(ps)
				return fmt.Errorf(
					"invalid topology: %q value %q appears under both %q and %q",
					key, topologyValue, ps[0], ps[1],
				)
			}
		}
	}

	// 4. Cross‑key uniqueness (deterministic).
	seenKey := map[string]string{}
	for _, key := range hierarchy {
		vs := make([]string, 0, len(values[key]))
		for v := range values[key] {
			vs = append(vs, v)
		}
		sort.Strings(vs)
		for _, v := range vs {
			if fk, ok := seenKey[v]; ok {
				return fmt.Errorf(
					"invalid topology: value %q appears under both %q and %q",
					v, fk, key,
				)
			}
			seenKey[v] = key
		}
	}

	return nil
}
