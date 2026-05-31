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
	"testing"

	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
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
		corev1.LabelZoneRegionStable:  "r.region",
		"kubernetes.io/hostname":      "host.name",
		"my_custom_hostname_label":    "host.custom.name",
		"topology.rook.io/rack":       "r.rack",
		"topology.rook.io/row":        "r.row",
		"topology.rook.io/datacenter": "d.datacenter",
		"topology.rook.io/room":       "test",
		"topology.rook.io/chassis":    "test",
		"topology.rook.io/pod":        "test",
	}
	topology, affinity := ExtractOSDTopologyFromLabels(nodeLabels)
	assert.Equal(t, 6, len(topology))
	assert.Equal(t, "r-region", topology["region"])
	assert.Equal(t, "host-name", topology["host"])
	assert.Equal(t, "r-rack", topology["rack"])
	assert.Equal(t, "r-row", topology["row"])
	assert.Equal(t, "d-datacenter", topology["datacenter"])
	assert.Equal(t, "topology.rook.io/chassis=test", affinity)
	assert.Equal(t, "test", topology["chassis"])
	assert.Equal(t, "", topology["pod"])
	assert.Equal(t, "", topology["room"])

	t.Setenv("ROOK_CUSTOM_HOSTNAME_LABEL", "my_custom_hostname_label")
	topology, affinity = ExtractOSDTopologyFromLabels(nodeLabels)
	assert.Equal(t, 6, len(topology))
	assert.Equal(t, "r-region", topology["region"])
	assert.Equal(t, "host-custom-name", topology["host"])
	assert.Equal(t, "r-rack", topology["rack"])
	assert.Equal(t, "r-row", topology["row"])
	assert.Equal(t, "d-datacenter", topology["datacenter"])
	assert.Equal(t, "topology.rook.io/chassis=test", affinity)
	assert.Equal(t, "test", topology["chassis"])
	assert.Equal(t, "", topology["pod"])
	assert.Equal(t, "", topology["room"])
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
		corev1.LabelZoneRegionStable:  "r1",
		"kubernetes.io/hostname":      "myhost",
		"topology.rook.io/rack":       "rack1",
		"topology.rook.io/row":        "row1",
		"topology.rook.io/datacenter": "d1",
	}
	topology, affinity = extractTopologyFromLabels(nodeLabels)
	assert.Equal(t, 5, len(topology))
	assert.Equal(t, "r1", topology["region"])
	assert.Equal(t, "myhost", topology["host"])
	assert.Equal(t, "rack1", topology["rack"])
	assert.Equal(t, "row1", topology["row"])
	assert.Equal(t, "d1", topology["datacenter"])
	assert.Equal(t, "topology.rook.io/rack=rack1", affinity)

	// invalid labels under topology.rook.io return an error
	nodeLabels = map[string]string{
		"topology.rook.io/row/bad": "r1",
	}
	topology, affinity = extractTopologyFromLabels(nodeLabels)
	assert.Equal(t, 0, len(topology))
	assert.Equal(t, "", affinity)
}

func TestGetDefaultTopologyLabels(t *testing.T) {
	expectedLabels := "kubernetes.io/hostname," +
		"topology.kubernetes.io/region," +
		"topology.kubernetes.io/zone," +
		"topology.rook.io/chassis," +
		"topology.rook.io/rack," +
		"topology.rook.io/row," +
		"topology.rook.io/pdu," +
		"topology.rook.io/pod," +
		"topology.rook.io/room," +
		"topology.rook.io/datacenter"
	assert.Equal(t, expectedLabels, GetDefaultTopologyLabels())

	t.Setenv("ROOK_CUSTOM_HOSTNAME_LABEL", "my_custom_hostname_label")
	expectedLabels = "my_custom_hostname_label," +
		"topology.kubernetes.io/region," +
		"topology.kubernetes.io/zone," +
		"topology.rook.io/chassis," +
		"topology.rook.io/rack," +
		"topology.rook.io/row," +
		"topology.rook.io/pdu," +
		"topology.rook.io/pod," +
		"topology.rook.io/room," +
		"topology.rook.io/datacenter"
	assert.Equal(t, expectedLabels, GetDefaultTopologyLabels())
}

func TestCheckTopologyConflicts(t *testing.T) {
	node := func(name string, labels map[string]string) corev1.Node {
		return corev1.Node{
			ObjectMeta: metav1.ObjectMeta{
				Name:   name,
				Labels: labels,
			},
		}
	}

	t.Run("valid: multiple racks in same zone", func(t *testing.T) {
		nodes := []corev1.Node{
			node("node-a", map[string]string{"topology.kubernetes.io/zone": "zone1", "topology.rook.io/rack": "rack1"}),
			node("node-b", map[string]string{"topology.kubernetes.io/zone": "zone1", "topology.rook.io/rack": "rack2"}),
			node("node-c", map[string]string{"topology.kubernetes.io/zone": "zone1", "topology.rook.io/rack": "rack3"}),
			node("node-d", map[string]string{"topology.kubernetes.io/zone": "zone1", "topology.rook.io/rack": "rack3"}),
		}
		err := CheckTopologyConflicts(&nodes)
		assert.NoError(t, err)
	})

	t.Run("invalid: same rack across zones", func(t *testing.T) {
		nodes := []corev1.Node{
			node("node-a", map[string]string{"topology.kubernetes.io/zone": "zone1", "topology.rook.io/rack": "rack1"}),
			node("node-b", map[string]string{"topology.kubernetes.io/zone": "zone2", "topology.rook.io/rack": "rack1"}),
			node("node-c", map[string]string{"topology.kubernetes.io/zone": "zone3", "topology.rook.io/rack": "rack3"}),
		}
		err := CheckTopologyConflicts(&nodes)
		assert.Error(t, err)
	})

	t.Run("invalid: same row across datacenters", func(t *testing.T) {
		nodes := []corev1.Node{
			node("node-a", map[string]string{"topology.rook.io/datacenter": "dc1", "topology.rook.io/row": "row1"}),
			node("node-b", map[string]string{"topology.rook.io/datacenter": "dc2", "topology.rook.io/row": "row1"}),
		}
		err := CheckTopologyConflicts(&nodes)
		assert.Error(t, err)
	})

	t.Run("invalid: overlapping zone and row values", func(t *testing.T) {
		nodes := []corev1.Node{
			node("node-a", map[string]string{"topology.kubernetes.io/zone": "X", "topology.rook.io/row": "Y"}),
			node("node-b", map[string]string{"topology.kubernetes.io/zone": "Y", "topology.rook.io/row": "Z"}),
		}
		err := CheckTopologyConflicts(&nodes)
		assert.Error(t, err)
	})

	t.Run("valid: only zone labels used", func(t *testing.T) {
		nodes := []corev1.Node{
			node("node-a", map[string]string{"topology.kubernetes.io/zone": "zone1"}),
			node("node-b", map[string]string{"topology.kubernetes.io/zone": "zone2"}),
			node("node-c", map[string]string{"topology.kubernetes.io/zone": "zone3"}),
		}
		err := CheckTopologyConflicts(&nodes)
		assert.NoError(t, err)
	})

	t.Run("valid: only rack labels used", func(t *testing.T) {
		nodes := []corev1.Node{
			node("node-a", map[string]string{"topology.rook.io/rack": "rack1"}),
			node("node-b", map[string]string{"topology.rook.io/rack": "rack2"}),
			node("node-c", map[string]string{"topology.rook.io/rack": "rack3"}),
		}
		err := CheckTopologyConflicts(&nodes)
		assert.NoError(t, err)
	})

	t.Run("valid: same rack label on all nodes without parent", func(t *testing.T) {
		nodes := []corev1.Node{
			node("node-a", map[string]string{"topology.rook.io/rack": "shared"}),
			node("node-b", map[string]string{"topology.rook.io/rack": "shared"}),
		}
		err := CheckTopologyConflicts(&nodes)
		assert.NoError(t, err)
	})

	t.Run("invalid: same value reused for different topology keys", func(t *testing.T) {
		nodes := []corev1.Node{
			node("node-a", map[string]string{"topology.kubernetes.io/zone": "shared"}),
			node("node-b", map[string]string{"topology.rook.io/rack": "shared"}),
		}
		err := CheckTopologyConflicts(&nodes)
		assert.Error(t, err)
	})
	t.Run("valid: region-zone-hostname topology", func(t *testing.T) {
		nodes := []corev1.Node{
			node("master-0", map[string]string{
				"kubernetes.io/hostname":        "master-0",
				"topology.kubernetes.io/region": "us-south",
				"topology.kubernetes.io/zone":   "us-south-1",
			}),
			node("master-1", map[string]string{
				"kubernetes.io/hostname":        "master-1",
				"topology.kubernetes.io/region": "us-south",
				"topology.kubernetes.io/zone":   "us-south-2",
			}),
			node("master-2", map[string]string{
				"kubernetes.io/hostname":        "master-2",
				"topology.kubernetes.io/region": "us-south",
				"topology.kubernetes.io/zone":   "us-south-3",
			}),
			node("worker-1", map[string]string{
				"kubernetes.io/hostname":        "worker-1",
				"topology.kubernetes.io/region": "us-south",
				"topology.kubernetes.io/zone":   "us-south-1",
			}),
			node("worker-2", map[string]string{
				"kubernetes.io/hostname":        "worker-2",
				"topology.kubernetes.io/region": "us-south",
				"topology.kubernetes.io/zone":   "us-south-2",
			}),
			node("worker-3", map[string]string{
				"kubernetes.io/hostname":        "worker-3",
				"topology.kubernetes.io/region": "us-south",
				"topology.kubernetes.io/zone":   "us-south-3",
			}),
		}
		err := CheckTopologyConflicts(&nodes)
		assert.NoError(t, err)
	})
	t.Run("invalid: rack reused under multiple zones", func(t *testing.T) {
		nodes := []corev1.Node{
			node("node-1", map[string]string{
				"kubernetes.io/hostname":        "infra-zone1",
				"topology.kubernetes.io/region": "us-south",
				"topology.kubernetes.io/zone":   "zone1",
				"topology.rook.io/rack":         "rack1",
			}),
			node("node-2", map[string]string{
				"kubernetes.io/hostname":        "infra-zone2",
				"topology.kubernetes.io/region": "us-south",
				"topology.kubernetes.io/zone":   "zone2",
				"topology.rook.io/rack":         "rack1",
			}),
			node("node-3", map[string]string{
				"kubernetes.io/hostname":        "infra-zone3",
				"topology.kubernetes.io/region": "us-south",
				"topology.kubernetes.io/zone":   "zone3",
				"topology.rook.io/rack":         "rack1",
			}),
		}

		err := CheckTopologyConflicts(&nodes)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "topology.rook.io/rack")
	})
	t.Run("valid: child label missing from part of hierarchy", func(t *testing.T) {
		nodes := []corev1.Node{
			node("r1-z1-r1-n1", map[string]string{
				"topology.kubernetes.io/region": "region1",
				"topology.kubernetes.io/zone":   "r1-z1",
				"topology.rook.io/rack":         "r1-z1-r1",
			}),
			node("r1-z1-r2-n2", map[string]string{
				"topology.kubernetes.io/region": "region1",
				"topology.kubernetes.io/zone":   "r1-z1",
				// No rack label
			}),
		}

		err := CheckTopologyConflicts(&nodes)
		assert.NoError(t, err)
	})
	t.Run("invalid: duplicate values across zone and datacenter keys", func(t *testing.T) {
		nodes := []corev1.Node{
			node("node-a", map[string]string{
				"topology.kubernetes.io/region": "region1",
				"topology.kubernetes.io/zone":   "r1-dc1",
				"topology.rook.io/datacenter":   "r1-dc1",
			}),
			node("node-b", map[string]string{
				"topology.kubernetes.io/region": "region1",
				"topology.kubernetes.io/zone":   "r1-dc2",
				"topology.rook.io/datacenter":   "r1-dc2",
			}),
		}

		err := CheckTopologyConflicts(&nodes)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "r1-dc1")
		assert.Contains(t, err.Error(), "topology.kubernetes.io/zone")
		assert.Contains(t, err.Error(), "topology.rook.io/datacenter")
	})

	t.Run("valid: full topology hierarchy consistent", func(t *testing.T) {
		nodes := []corev1.Node{
			node("node-a", map[string]string{
				"topology.kubernetes.io/region": "region1",
				"topology.kubernetes.io/zone":   "zone1",
				"topology.rook.io/datacenter":   "dc1",
				"topology.rook.io/room":         "room1",
				"topology.rook.io/pod":          "pod1",
				"topology.rook.io/pdu":          "pdu1",
				"topology.rook.io/row":          "row1",
				"topology.rook.io/rack":         "rack1",
				"topology.rook.io/chassis":      "chassis1",
			}),
			node("node-b", map[string]string{
				"topology.kubernetes.io/region": "region1",
				"topology.kubernetes.io/zone":   "zone1",
				"topology.rook.io/datacenter":   "dc1",
				"topology.rook.io/room":         "room1",
				"topology.rook.io/pod":          "pod1",
				"topology.rook.io/pdu":          "pdu1",
				"topology.rook.io/row":          "row1",
				"topology.rook.io/rack":         "rack1",
				"topology.rook.io/chassis":      "chassis1",
			}),
		}
		err := CheckTopologyConflicts(&nodes)
		assert.NoError(t, err)
	})

	t.Run("valid: only region labels used", func(t *testing.T) {
		nodes := []corev1.Node{
			node("node-a", map[string]string{"topology.kubernetes.io/region": "region1"}),
			node("node-b", map[string]string{"topology.kubernetes.io/region": "region2"}),
		}
		err := CheckTopologyConflicts(&nodes)
		assert.NoError(t, err)
	})

	t.Run("invalid: fewer datacenters than zones", func(t *testing.T) {
		nodes := []corev1.Node{
			node("node-a", map[string]string{
				"topology.kubernetes.io/zone": "zone1",
				"topology.rook.io/datacenter": "dc1",
			}),
			node("node-b", map[string]string{
				"topology.kubernetes.io/zone": "zone2",
				"topology.rook.io/datacenter": "dc1",
			}),
		}
		err := CheckTopologyConflicts(&nodes)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "topology.rook.io/datacenter")
	})

	t.Run("valid: multiple chassis in same rack", func(t *testing.T) {
		nodes := []corev1.Node{
			node("node-a", map[string]string{
				"topology.rook.io/rack":    "rack1",
				"topology.rook.io/chassis": "chassis1",
			}),
			node("node-b", map[string]string{
				"topology.rook.io/rack":    "rack1",
				"topology.rook.io/chassis": "chassis2",
			}),
		}
		err := CheckTopologyConflicts(&nodes)
		assert.NoError(t, err)
	})

	t.Run("invalid: chassis reused under multiple racks", func(t *testing.T) {
		nodes := []corev1.Node{
			node("node-a", map[string]string{
				"topology.rook.io/rack":    "rack1",
				"topology.rook.io/chassis": "chassis1",
			}),
			node("node-b", map[string]string{
				"topology.rook.io/rack":    "rack2",
				"topology.rook.io/chassis": "chassis1",
			}),
		}
		err := CheckTopologyConflicts(&nodes)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "topology.rook.io/chassis")
	})

	t.Run("invalid: same value reused for pod and rack", func(t *testing.T) {
		nodes := []corev1.Node{
			node("node-a", map[string]string{"topology.rook.io/pod": "shared"}),
			node("node-b", map[string]string{"topology.rook.io/rack": "shared"}),
		}
		err := CheckTopologyConflicts(&nodes)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "topology.rook.io/pod")
		assert.Contains(t, err.Error(), "topology.rook.io/rack")
	})

	t.Run("valid: only pdu labels used", func(t *testing.T) {
		nodes := []corev1.Node{
			node("a", map[string]string{"topology.rook.io/pdu": "pduA"}),
			node("b", map[string]string{"topology.rook.io/pdu": "pduB"}),
		}
		err := CheckTopologyConflicts(&nodes)
		assert.NoError(t, err)
	})

	t.Run("valid: zone value equals hostname", func(t *testing.T) {
		nodes := []corev1.Node{
			{
				ObjectMeta: metav1.ObjectMeta{
					Name:   "foo-1",
					Labels: map[string]string{"kubernetes.io/hostname": "foo-1", "topology.kubernetes.io/zone": "foo-1"},
				},
			},
			node("bar-1", map[string]string{"topology.kubernetes.io/zone": "bar-1"}),
		}
		err := CheckTopologyConflicts(&nodes)
		assert.NoError(t, err)
	})

	t.Run("invalid: duplicate value across region and rack", func(t *testing.T) {
		nodes := []corev1.Node{
			node("a", map[string]string{"topology.kubernetes.io/region": "X"}),
			node("b", map[string]string{"topology.rook.io/rack": "X"}),
		}
		err := CheckTopologyConflicts(&nodes)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "topology.kubernetes.io/region")
		assert.Contains(t, err.Error(), "topology.rook.io/rack")
	})

	t.Run("valid: disjoint subtrees in hierarchy", func(t *testing.T) {
		nodes := []corev1.Node{
			node("a", map[string]string{"topology.kubernetes.io/zone": "Z1", "topology.rook.io/rack": "R1"}),
			node("b", map[string]string{"topology.rook.io/datacenter": "DC1", "topology.rook.io/room": "RM1"}),
		}
		err := CheckTopologyConflicts(&nodes)
		assert.NoError(t, err)
	})

	t.Run("valid: equal rooms and datacenters", func(t *testing.T) {
		nodes := []corev1.Node{
			node("n1", map[string]string{
				"topology.rook.io/datacenter": "dc1",
				"topology.rook.io/room":       "room1",
			}),
			node("n2", map[string]string{
				"topology.rook.io/datacenter": "dc2",
				"topology.rook.io/room":       "room2",
			}),
		}
		err := CheckTopologyConflicts(&nodes)
		assert.NoError(t, err)
	})

	t.Run("invalid: fewer pods than rooms", func(t *testing.T) {
		nodes := []corev1.Node{
			node("n1", map[string]string{
				"topology.rook.io/room": "room1",
				"topology.rook.io/pod":  "pod1",
			}),
			node("n2", map[string]string{
				"topology.rook.io/room": "room2",
				// missing pod label
			}),
		}
		err := CheckTopologyConflicts(&nodes)
		assert.NoError(t, err)
	})

	t.Run("valid: only chassis labels used", func(t *testing.T) {
		nodes := []corev1.Node{
			node("n1", map[string]string{"topology.rook.io/chassis": "c1"}),
			node("n2", map[string]string{"topology.rook.io/chassis": "c2"}),
		}
		err := CheckTopologyConflicts(&nodes)
		assert.NoError(t, err)
	})

	t.Run("valid: no topology labels on nodes", func(t *testing.T) {
		nodes := []corev1.Node{
			node("n1", map[string]string{}),
			node("n2", map[string]string{}),
		}
		err := CheckTopologyConflicts(&nodes)
		assert.NoError(t, err)
	})

	t.Run("invalid: same zone across multiple regions", func(t *testing.T) {
		nodes := []corev1.Node{
			node("n1", map[string]string{
				"topology.kubernetes.io/region": "r1",
				"topology.kubernetes.io/zone":   "zone1",
			}),
			node("n2", map[string]string{
				"topology.kubernetes.io/region": "r2",
				"topology.kubernetes.io/zone":   "zone1",
			}),
		}
		err := CheckTopologyConflicts(&nodes)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), `"topology.kubernetes.io/zone"`)
	})

	t.Run("valid: only room labels used", func(t *testing.T) {
		nodes := []corev1.Node{
			node("n1", map[string]string{"topology.rook.io/room": "r1"}),
			node("n2", map[string]string{"topology.rook.io/room": "r2"}),
		}
		err := CheckTopologyConflicts(&nodes)
		assert.NoError(t, err)
	})

	t.Run("valid: equal datacenters and zones", func(t *testing.T) {
		nodes := []corev1.Node{
			node("n1", map[string]string{
				"topology.kubernetes.io/zone": "z1",
				"topology.rook.io/datacenter": "dc1",
			}),
			node("n2", map[string]string{
				"topology.kubernetes.io/zone": "z2",
				"topology.rook.io/datacenter": "dc2",
			}),
		}
		err := CheckTopologyConflicts(&nodes)
		assert.NoError(t, err)
	})
	t.Run("valid: racks only under one zone out of many", func(t *testing.T) {
		nodes := []corev1.Node{
			node("node-1", map[string]string{
				"topology.kubernetes.io/region": "r1",
				"topology.kubernetes.io/zone":   "zone1",
				"topology.rook.io/rack":         "rack1",
			}),
			node("node-2", map[string]string{
				"topology.kubernetes.io/region": "r1",
				"topology.kubernetes.io/zone":   "zone1",
				"topology.rook.io/rack":         "rack2",
			}),
			node("node-3", map[string]string{
				"topology.kubernetes.io/region": "r1",
				"topology.kubernetes.io/zone":   "zone1",
				"topology.rook.io/rack":         "rack3",
			}),
			node("node-4", map[string]string{
				"topology.kubernetes.io/region": "r1",
				"topology.kubernetes.io/zone":   "zone2",
			}),
			node("node-5", map[string]string{
				"topology.kubernetes.io/region": "r1",
				"topology.kubernetes.io/zone":   "zone3",
			}),
			node("node-6", map[string]string{
				"topology.kubernetes.io/region": "r1",
				"topology.kubernetes.io/zone":   "zone4",
			}),
		}
		err := CheckTopologyConflicts(&nodes)
		assert.NoError(t, err)
	})
	t.Run("invalid: 3 regions but only 2 racks", func(t *testing.T) {
		nodes := []corev1.Node{
			node("node-1", map[string]string{
				"topology.kubernetes.io/region": "r1",
				"topology.rook.io/rack":         "rack1",
			}),
			node("node-2", map[string]string{
				"topology.kubernetes.io/region": "r2",
				"topology.rook.io/rack":         "rack2",
			}),
			node("node-3", map[string]string{
				"topology.kubernetes.io/region": "r3",
				"topology.rook.io/rack":         "rack1", // reused rack label
			}),
		}
		err := CheckTopologyConflicts(&nodes)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "topology.rook.io/rack")
	})
	t.Run("valid: each zone has exactly one rack", func(t *testing.T) {
		nodes := []corev1.Node{
			node("node-1", map[string]string{
				"topology.kubernetes.io/zone": "zoneA",
				"topology.rook.io/rack":       "rackA",
			}),
			node("node-2", map[string]string{
				"topology.kubernetes.io/zone": "zoneB",
				"topology.rook.io/rack":       "rackB",
			}),
			node("node-3", map[string]string{
				"topology.kubernetes.io/zone": "zoneC",
				"topology.rook.io/rack":       "rackC",
			}),
		}
		err := CheckTopologyConflicts(&nodes)
		assert.NoError(t, err)
	})
	t.Run("invalid: node with empty zone label", func(t *testing.T) {
		nodes := []corev1.Node{
			node("node-a", map[string]string{
				"topology.kubernetes.io/region": "region1",
				"topology.kubernetes.io/zone":   "zone1",
			}),
			node("node-b", map[string]string{
				"topology.kubernetes.io/region": "region1",
				"topology.kubernetes.io/zone":   "", // invalid: empty value
			}),
		}
		err := CheckTopologyConflicts(&nodes)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "topology.kubernetes.io/zone")
	})

	t.Run("invalid: same value reused under region and datacenter", func(t *testing.T) {
		nodes := []corev1.Node{
			node("node-a", map[string]string{"topology.kubernetes.io/region": "common"}),
			node("node-b", map[string]string{"topology.rook.io/datacenter": "common"}),
		}
		err := CheckTopologyConflicts(&nodes)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "value \"common\" appears under both")
	})
	t.Run("invalid: same value reused across keys on different nodes", func(t *testing.T) {
		nodes := []corev1.Node{
			node("node-a", map[string]string{
				"topology.kubernetes.io/zone": "shared", // used here under zone
			}),
			node("node-b", map[string]string{
				"topology.rook.io/rack": "shared", // same value under rack, different node
			}),
			node("node-c", map[string]string{
				"topology.rook.io/datacenter": "dc1",
				"topology.rook.io/row":        "row1",
			}),
		}
		err := CheckTopologyConflicts(&nodes)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), `"shared"`)
		assert.Contains(t, err.Error(), "topology.kubernetes.io/zone")
		assert.Contains(t, err.Error(), "topology.rook.io/rack")
	})
}
