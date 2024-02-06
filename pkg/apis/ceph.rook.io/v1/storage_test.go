/*
Copyright 2018 The Rook Authors. All rights reserved.

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
package v1

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestNodeExists(t *testing.T) {
	t.Run("does not exist - no nodes specified", func(t *testing.T) {
		spec := StorageScopeSpec{}
		assert.False(t, spec.NodeExists("does-not-exist"))
	})

	t.Run("exists - single node specified", func(t *testing.T) {
		spec := StorageScopeSpec{
			Nodes: []Node{
				{Name: "node1"}, // node gets nothing but its name set
			},
		}
		assert.True(t, spec.NodeExists("node1"))
	})

	t.Run("exists and not exists - multiple nodes specified", func(t *testing.T) {
		spec := StorageScopeSpec{
			Nodes: []Node{
				{Name: "node1"}, // node gets nothing but its name set
				{Name: "node3"},
				{Name: "node4"},
			},
		}
		assert.True(t, spec.NodeExists("node1"))
		assert.False(t, spec.NodeExists("node2"))
		assert.True(t, spec.NodeExists("node3"))
		assert.True(t, spec.NodeExists("node4"))
		assert.False(t, spec.NodeExists("node5"))
		assert.False(t, spec.NodeExists("does-not-exist"))
	})
}

func TestResolveNodeNotExist(t *testing.T) {
	// a nonexistent node should return nil
	storageSpec := StorageScopeSpec{}
	node := storageSpec.ResolveNode("fake node")
	assert.Nil(t, node)
}

func TestResolveNodeDefaultValues(t *testing.T) {
	// a node with no properties and none defined in the cluster storage spec should get the default values
	storageSpec := StorageScopeSpec{
		Nodes: []Node{
			{Name: "node1"}, // node gets nothing but its name set
		},
	}

	node := storageSpec.ResolveNode("node1")
	assert.NotNil(t, node)
	assert.Equal(t, "", node.Selection.DeviceFilter)
	assert.Equal(t, "", node.Selection.DevicePathFilter)
	assert.False(t, node.Selection.GetUseAllDevices())
	assert.Equal(t, storageSpec.Devices, node.Devices)
}

func TestResolveNodeInherentFromCluster(t *testing.T) {
	// a node with no properties defined should inherit them from the cluster storage spec
	storageSpec := StorageScopeSpec{
		Selection: Selection{
			DeviceFilter:     "^sd.",
			DevicePathFilter: "^/dev/disk/by-path/pci-.*",
			Devices:          []Device{{Name: "sda"}},
		},
		Config: map[string]string{
			"foo": "bar",
		},
		Nodes: []Node{
			{Name: "node1"}, // node gets nothing but its name set
		},
	}

	node := storageSpec.ResolveNode("node1")
	assert.NotNil(t, node)
	assert.Equal(t, "^sd.", node.Selection.DeviceFilter)
	assert.Equal(t, "^/dev/disk/by-path/pci-.*", node.Selection.DevicePathFilter)
	assert.False(t, node.Selection.GetUseAllDevices())
	assert.Equal(t, "bar", node.Config["foo"])
	assert.Equal(t, []Device{{Name: "sda"}}, node.Devices)
}

func TestResolveNodeSpecificProperties(t *testing.T) {
	// a node with its own specific properties defined should keep those values, regardless of what the global cluster config is
	storageSpec := StorageScopeSpec{
		Selection: Selection{
			DeviceFilter:     "^sd.",
			DevicePathFilter: "^/dev/disk/by-path/pci-.*",
		},
		Config: map[string]string{
			"foo": "bar",
			"baz": "biz",
		},
		Nodes: []Node{
			{
				Name: "node1", // node has its own config that should override cluster level config
				Selection: Selection{
					DeviceFilter:     "nvme.*",
					DevicePathFilter: "^/dev/disk/by-id/.*foo.*",
					Devices:          []Device{{Name: "device026"}},
				},
				Config: map[string]string{
					"foo": "node1bar",
				},
			},
		},
	}

	node := storageSpec.ResolveNode("node1")
	assert.NotNil(t, node)
	assert.False(t, node.Selection.GetUseAllDevices())
	assert.Equal(t, "nvme.*", node.Selection.DeviceFilter)
	assert.Equal(t, "^/dev/disk/by-id/.*foo.*", node.Selection.DevicePathFilter)
	assert.Equal(t, []Device{{Name: "device026"}}, node.Devices)
	assert.Equal(t, "node1bar", node.Config["foo"])
	assert.Equal(t, "biz", node.Config["baz"])
}

func TestResolveNodeUseAllDevices(t *testing.T) {
	storageSpec := StorageScopeSpec{
		Selection: Selection{UseAllDevices: newBool(true)}, // UseAllDevices is set to true on the storage spec
		Nodes: []Node{
			{Name: "node1"}, // node gets nothing but its name set
		},
	}

	node := storageSpec.ResolveNode("node1")
	assert.NotNil(t, node)
	assert.True(t, node.Selection.GetUseAllDevices())
}

func TestUseAllDevices(t *testing.T) {
	storageSpec := StorageScopeSpec{}
	assert.False(t, storageSpec.AnyUseAllDevices())

	storageSpec = StorageScopeSpec{
		Selection: Selection{
			UseAllDevices: newBool(true)}, // UseAllDevices is set to true on the storage spec
	}
	assert.True(t, storageSpec.AnyUseAllDevices())

	storageSpec = StorageScopeSpec{
		Selection: Selection{UseAllDevices: newBool(false)},
		Nodes: []Node{
			{
				Name:      "node1",
				Selection: Selection{UseAllDevices: newBool(true)},
			},
		},
	}
	assert.True(t, storageSpec.AnyUseAllDevices())
}

func TestClearUseAllDevices(t *testing.T) {
	// create a storage spec with use all devices set to true for the cluster and for all nodes
	storageSpec := StorageScopeSpec{
		Selection: Selection{UseAllDevices: newBool(true)},
		Nodes: []Node{
			{
				Name:      "node1",
				Selection: Selection{UseAllDevices: newBool(true)},
			},
		},
	}
	assert.True(t, storageSpec.AnyUseAllDevices())

	// now clear the use all devices field, it should be cleared from the entire cluster and its nodes
	storageSpec.ClearUseAllDevices()
	assert.False(t, storageSpec.AnyUseAllDevices())
}

func TestClusterDirsDevsInherit(t *testing.T) {
	// test for no directories or devices given
	storageSpec := StorageScopeSpec{
		Nodes: []Node{
			{
				Name: "node1",
			},
		},
	}
	node := storageSpec.ResolveNode("node1")
	assert.NotNil(t, node)
	assert.Equal(t, storageSpec.Devices, node.Devices)

	// test if cluster wide devices are inherited to no-directories/devices node
	storageSpec = StorageScopeSpec{
		Selection: Selection{
			Devices: []Device{{Name: "device1"}},
		},
		Nodes: []Node{
			{
				Name: "node1",
			},
		},
	}
	node = storageSpec.ResolveNode("node1")
	assert.NotNil(t, node)
	assert.Equal(t, []Device{{Name: "device1"}}, node.Devices)

	// test if node directories and devices are used
	storageSpec = StorageScopeSpec{
		Nodes: []Node{
			{
				Name: "node1",
				Selection: Selection{
					Devices: []Device{{Name: "device2"}},
				},
			},
		},
	}
	node = storageSpec.ResolveNode("node1")
	assert.NotNil(t, node)
	assert.Equal(t, []Device{{Name: "device2"}}, node.Devices)

	// test if cluster wide devices are and aren't inherited to nodes with and without directories/devices
	storageSpec = StorageScopeSpec{
		Selection: Selection{
			Devices: []Device{{Name: "device4"}},
		},
		Nodes: []Node{
			{
				Name: "node1",
				Selection: Selection{
					Devices: []Device{{Name: "device3"}},
				},
			},
			{
				Name: "node2",
			},
		},
	}
	// node1 keeps its specified devices
	node = storageSpec.ResolveNode("node1")
	assert.NotNil(t, node)
	assert.Equal(t, []Device{{Name: "device3"}}, node.Devices)

	// node2 inherits the cluster wide devices since it specified none of its own
	node = storageSpec.ResolveNode("node2")
	assert.NotNil(t, node)
	assert.Equal(t, []Device{{Name: "device4"}}, node.Devices)
}

func TestStorageScopeSpec_NodeWithNameExists(t *testing.T) {
	spec := &StorageScopeSpec{
		Nodes: []Node{},
	}

	assert.False(t, spec.NodeWithNameExists("node0"))

	spec.Nodes = []Node{
		{Name: "node0-hostname"},
		{Name: "node1"},
		{Name: "node2"}}
	assert.True(t, spec.NodeWithNameExists("node0-hostname"))
	assert.False(t, spec.NodeWithNameExists("node0"))
	assert.True(t, spec.NodeWithNameExists("node1"))
	assert.True(t, spec.NodeWithNameExists("node2"))
}

func TestIsOnPVCEncrypted(t *testing.T) {
	s := &StorageScopeSpec{}
	assert.False(t, s.IsOnPVCEncrypted())

	s.StorageClassDeviceSets = []StorageClassDeviceSet{
		{Encrypted: true},
	}
	assert.True(t, s.IsOnPVCEncrypted())
}
