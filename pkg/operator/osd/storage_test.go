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
package osd

import (
	"testing"

	"github.com/stretchr/testify/assert"

	cephosd "github.com/rook/rook/pkg/ceph/osd"
)

func TestResolveNodeNotExist(t *testing.T) {

	// a non existing node should return nil
	storageSpec := StorageSpec{}
	node := storageSpec.resolveNode("fake node")
	assert.Nil(t, node)

	// a node with no properties defined should inherit them from the cluster storage spec
	storageSpec = StorageSpec{
		Selection: Selection{
			DeviceFilter:   "^sd.",
			MetadataDevice: "nvme01",
		},
		Config: Config{
			Location: "root=default,row=a,rack=a2,chassis=a2a,host=a2a1",
			StoreConfig: cephosd.StoreConfig{
				StoreType:      cephosd.Bluestore,
				DatabaseSizeMB: 1024,
				WalSizeMB:      128,
				JournalSizeMB:  2048,
			},
		},
		Nodes: []Node{
			{Name: "node1"}, // node gets nothing but its name set
		},
	}
	node = storageSpec.resolveNode("node1")
	assert.NotNil(t, node)
	assert.Equal(t, "^sd.", node.Selection.DeviceFilter)
	assert.Equal(t, "nvme01", node.Selection.MetadataDevice)
	assert.False(t, node.Selection.getUseAllDevices())
	assert.Equal(t, "root=default,row=a,rack=a2,chassis=a2a,host=a2a1", node.Config.Location)
	assert.Equal(t, cephosd.Bluestore, node.Config.StoreConfig.StoreType)
	assert.Equal(t, 1024, node.Config.StoreConfig.DatabaseSizeMB)
	assert.Equal(t, 128, node.Config.StoreConfig.WalSizeMB)
	assert.Equal(t, 2048, node.Config.StoreConfig.JournalSizeMB)
}

func TestResolveNodeInherentFromCluster(t *testing.T) {
	// a node with no properties defined should inherit them from the cluster storage spec
	storageSpec := StorageSpec{
		Selection: Selection{
			DeviceFilter:   "^sd.",
			MetadataDevice: "nvme01",
		},
		Config: Config{
			Location: "root=default,row=a,rack=a2,chassis=a2a,host=a2a1",
			StoreConfig: cephosd.StoreConfig{
				StoreType:      cephosd.Bluestore,
				DatabaseSizeMB: 1024,
				WalSizeMB:      128,
				JournalSizeMB:  2048,
			},
		},
		Nodes: []Node{
			{Name: "node1"}, // node gets nothing but its name set
		},
	}
	node := storageSpec.resolveNode("node1")
	assert.NotNil(t, node)
	assert.Equal(t, "^sd.", node.Selection.DeviceFilter)
	assert.Equal(t, "nvme01", node.Selection.MetadataDevice)
	assert.False(t, node.Selection.getUseAllDevices())
	assert.Equal(t, "root=default,row=a,rack=a2,chassis=a2a,host=a2a1", node.Config.Location)
	assert.Equal(t, cephosd.Bluestore, node.Config.StoreConfig.StoreType)
	assert.Equal(t, 1024, node.Config.StoreConfig.DatabaseSizeMB)
	assert.Equal(t, 128, node.Config.StoreConfig.WalSizeMB)
	assert.Equal(t, 2048, node.Config.StoreConfig.JournalSizeMB)
}

func TestResolveNodeDefaultValues(t *testing.T) {

	// a node with no properties and none defined in the cluster storage spec should get the default values
	storageSpec := StorageSpec{
		Nodes: []Node{
			{Name: "node1"}, // node gets nothing but its name set
		},
	}

	node := storageSpec.resolveNode("node1")
	assert.NotNil(t, node)
	assert.Equal(t, "", node.Selection.DeviceFilter)
	assert.Equal(t, "", node.Selection.MetadataDevice)
	assert.False(t, node.Selection.getUseAllDevices())
	assert.Equal(t, "", node.Config.Location)
	assert.Equal(t, cephosd.Filestore, node.Config.StoreConfig.StoreType)
	assert.Equal(t, 0, node.Config.StoreConfig.DatabaseSizeMB)
	assert.Equal(t, 0, node.Config.StoreConfig.WalSizeMB)
	assert.Equal(t, 0, node.Config.StoreConfig.JournalSizeMB)
}

func TestResolveNodeUseAllDevices(t *testing.T) {

	storageSpec := StorageSpec{
		Selection: Selection{UseAllDevices: newBool(true)}, // UseAllDevices is set to true on the storage spec
		Nodes: []Node{
			{Name: "node1"}, // node gets nothing but its name set
		},
	}

	node := storageSpec.resolveNode("node1")
	assert.NotNil(t, node)
	assert.True(t, node.Selection.getUseAllDevices())
}

func TestUseAllDevices(t *testing.T) {
	storageSpec := StorageSpec{}
	assert.False(t, storageSpec.AnyUseAllDevices())

	storageSpec = StorageSpec{
		Selection: Selection{
			UseAllDevices: newBool(true)}, // UseAllDevices is set to true on the storage spec
	}
	assert.True(t, storageSpec.AnyUseAllDevices())

	storageSpec = StorageSpec{
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
	storageSpec := StorageSpec{
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
