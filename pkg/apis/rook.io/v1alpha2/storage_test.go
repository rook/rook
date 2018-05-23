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
package v1alpha2

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestResolveNodeNotExist(t *testing.T) {
	// a non existing node should return nil
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
	assert.False(t, node.Selection.GetUseAllDevices())
	assert.Equal(t, "", node.Location)
	assert.Equal(t, storageSpec.Directories, node.Directories)
}

func TestResolveNodeInherentFromCluster(t *testing.T) {
	// a node with no properties defined should inherit them from the cluster storage spec
	storageSpec := StorageScopeSpec{
		Location: "root=default,row=a,rack=a2,chassis=a2a,host=a2a1",
		Selection: Selection{
			DeviceFilter: "^sd.",
			Directories:  []Directory{{Path: "/rook/datadir1"}},
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
	assert.False(t, node.Selection.GetUseAllDevices())
	assert.Equal(t, "root=default,row=a,rack=a2,chassis=a2a,host=a2a1", node.Location)
	assert.Equal(t, "bar", node.Config["foo"])
	assert.Equal(t, []Directory{{Path: "/rook/datadir1"}}, node.Directories)
}

func TestResolveNodeSpecificProperties(t *testing.T) {
	// a node with its own specific properties defined should keep those values, regardless of what the global cluster config is
	storageSpec := StorageScopeSpec{
		Location: "root=default,row=a,rack=a2,chassis=a2a,host=a2a1",
		Selection: Selection{
			DeviceFilter: "^sd.",
			Directories:  []Directory{{Path: "/rook/datadir1"}},
		},
		Config: map[string]string{
			"foo": "bar",
			"baz": "biz",
		},
		Nodes: []Node{
			{
				Name: "node1", // node has its own config that should override cluster level config
				Selection: Selection{
					DeviceFilter: "nvme.*",
					Directories:  []Directory{{Path: "/rook/node1data"}},
				},
				Location: "host=node1",
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
	assert.Equal(t, []Directory{{Path: "/rook/node1data"}}, node.Directories)
	assert.Equal(t, "host=node1", node.Location)
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

func TestClusterDirectoriesInherit(t *testing.T) {
	// test for no directories given
	storageSpec := StorageScopeSpec{
		Nodes: []Node{
			{
				Name: "node1",
			},
		},
	}
	node := storageSpec.ResolveNode("node1")
	assert.NotNil(t, node)
	// compare both `StorageScopeSpec` and `Node` `Directories` because both are empty (`omitempty`)
	//empty `Directories` is "interpreted" as `[]osd.Directory(nil)` and not `[]osd.Directory{}`
	//by `assert.Equal`
	assert.Equal(t, storageSpec.Directories, node.Directories)

	// test if cluster wide directories is inherited to no-directories node
	storageSpec = StorageScopeSpec{
		Selection: Selection{
			Directories: []Directory{{Path: "/rook/datadir1"}},
		},
		Nodes: []Node{
			{
				Name: "node1",
			},
		},
	}
	node = storageSpec.ResolveNode("node1")
	assert.NotNil(t, node)
	assert.Equal(t, []Directory{{Path: "/rook/datadir1"}}, node.Directories)

	// test if node directories is used
	storageSpec = StorageScopeSpec{
		Nodes: []Node{
			{
				Name: "node1",
				Selection: Selection{
					Directories: []Directory{{Path: "/rook/datadir2"}},
				},
			},
		},
	}
	node = storageSpec.ResolveNode("node1")
	assert.NotNil(t, node)
	assert.Equal(t, []Directory{{Path: "/rook/datadir2"}}, node.Directories)

	// test if cluster wide directories is and isn't inherited to nodes with and without directories
	storageSpec = StorageScopeSpec{
		Selection: Selection{
			Directories: []Directory{{Path: "/rook/datadir4"}},
		},
		Nodes: []Node{
			{
				Name: "node1",
				Selection: Selection{
					Directories: []Directory{{Path: "/rook/datadir3"}},
				},
			},
			{
				Name: "node2",
			},
		},
	}
	node = storageSpec.ResolveNode("node1")
	assert.NotNil(t, node)
	assert.Equal(t, []Directory{{Path: "/rook/datadir3"}}, node.Directories)
	node = storageSpec.ResolveNode("node2")
	assert.NotNil(t, node)
	assert.Equal(t, []Directory{{Path: "/rook/datadir4"}}, node.Directories)
}
