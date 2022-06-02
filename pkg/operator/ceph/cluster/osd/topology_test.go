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
	"testing"

	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
)

func TestOrderedCRUSHLabels(t *testing.T) {
	assert.Equal(t, "host", CRUSHMapLevelsOrdered[0])
	assert.Equal(t, "chassis", CRUSHMapLevelsOrdered[1])
	assert.Equal(t, "rack", CRUSHMapLevelsOrdered[2])
	assert.Equal(t, "row", CRUSHMapLevelsOrdered[3])
	assert.Equal(t, "pdu", CRUSHMapLevelsOrdered[4])
	assert.Equal(t, "pod", CRUSHMapLevelsOrdered[5])
	assert.Equal(t, "room", CRUSHMapLevelsOrdered[6])
	assert.Equal(t, "datacenter", CRUSHMapLevelsOrdered[7])
	assert.Equal(t, "zone", CRUSHMapLevelsOrdered[8])
	assert.Equal(t, "region", CRUSHMapLevelsOrdered[9])
}

func TestCleanTopologyLabels(t *testing.T) {
	// load all the expected labels
	nodeLabels := map[string]string{
		corev1.LabelZoneRegionStable:        "r.region",
		corev1.LabelZoneFailureDomainStable: "z.zone",
		"kubernetes.io/hostname":            "host.name",
		"topology.rook.io/rack":             "r.rack",
		"topology.rook.io/row":              "r.row",
		"topology.rook.io/datacenter":       "d.datacenter",
	}
	topology, affinity := ExtractOSDTopologyFromLabels(nodeLabels)
	assert.Equal(t, 6, len(topology))
	assert.Equal(t, "r-region", topology["region"])
	assert.Equal(t, "z-zone", topology["zone"])
	assert.Equal(t, "host-name", topology["host"])
	assert.Equal(t, "r-rack", topology["rack"])
	assert.Equal(t, "r-row", topology["row"])
	assert.Equal(t, "d-datacenter", topology["datacenter"])
	assert.Equal(t, "topology.rook.io/rack=r.rack", affinity)
}

func TestTopologyLabels(t *testing.T) {
	nodeLabels := map[string]string{}
	topology, affinity := extractTopologyFromLabels(nodeLabels)
	assert.Equal(t, 0, len(topology))
	assert.Equal(t, "", affinity)

	// invalid non-namespaced zone and region labels are simply ignored
	nodeLabels = map[string]string{
		"region": "badregion",
		"zone":   "badzone",
	}
	topology, affinity = extractTopologyFromLabels(nodeLabels)
	assert.Equal(t, 0, len(topology))
	assert.Equal(t, "", affinity)

	// invalid zone and region labels are simply ignored
	nodeLabels = map[string]string{
		"topology.rook.io/region": "r1",
		"topology.rook.io/zone":   "z1",
	}
	topology, affinity = extractTopologyFromLabels(nodeLabels)
	assert.Equal(t, 0, len(topology))
	assert.Equal(t, "", affinity)

	// load all the expected labels
	nodeLabels = map[string]string{
		corev1.LabelZoneRegionStable:        "r1",
		corev1.LabelZoneFailureDomainStable: "z1",
		"kubernetes.io/hostname":            "myhost",
		"topology.rook.io/rack":             "rack1",
		"topology.rook.io/row":              "row1",
		"topology.rook.io/datacenter":       "d1",
	}
	topology, affinity = extractTopologyFromLabels(nodeLabels)
	assert.Equal(t, 6, len(topology))
	assert.Equal(t, "r1", topology["region"])
	assert.Equal(t, "z1", topology["zone"])
	assert.Equal(t, "myhost", topology["host"])
	assert.Equal(t, "rack1", topology["rack"])
	assert.Equal(t, "row1", topology["row"])
	assert.Equal(t, "d1", topology["datacenter"])
	assert.Equal(t, "topology.rook.io/rack=rack1", affinity)

	// ensure deprecated k8s labels are loaded
	nodeLabels = map[string]string{
		corev1.LabelZoneRegion:        "r1",
		corev1.LabelZoneFailureDomain: "z1",
	}
	topology, affinity = extractTopologyFromLabels(nodeLabels)
	assert.Equal(t, 2, len(topology))
	assert.Equal(t, "r1", topology["region"])
	assert.Equal(t, "z1", topology["zone"])
	assert.Equal(t, "failure-domain.beta.kubernetes.io/zone=z1", affinity)

	// ensure deprecated k8s labels are overridden
	nodeLabels = map[string]string{
		corev1.LabelZoneRegionStable:        "r1",
		corev1.LabelZoneFailureDomainStable: "z1",
		corev1.LabelZoneRegion:              "oldregion",
		corev1.LabelZoneFailureDomain:       "oldzone",
	}
	topology, affinity = extractTopologyFromLabels(nodeLabels)
	assert.Equal(t, 2, len(topology))
	assert.Equal(t, "r1", topology["region"])
	assert.Equal(t, "z1", topology["zone"])
	assert.Equal(t, "topology.kubernetes.io/zone=z1", affinity)

	// invalid labels under topology.rook.io return an error
	nodeLabels = map[string]string{
		"topology.rook.io/row/bad": "r1",
	}
	topology, affinity = extractTopologyFromLabels(nodeLabels)
	assert.Equal(t, 0, len(topology))
	assert.Equal(t, "", affinity)

	// ignore the region k8s topology label when it has the same value as the k8s zone label
	nodeLabels = map[string]string{
		corev1.LabelZoneRegionStable:        "zone",
		corev1.LabelZoneFailureDomainStable: "zone",
	}
	topology, affinity = extractTopologyFromLabels(nodeLabels)
	assert.Equal(t, 1, len(topology))
	assert.Equal(t, "zone", topology["zone"])
	assert.Equal(t, "topology.kubernetes.io/zone=zone", affinity)
}
