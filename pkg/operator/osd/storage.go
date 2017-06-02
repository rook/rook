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
	cephosd "github.com/rook/rook/pkg/ceph/osd"
)

func (s *StorageSpec) AnyUseAllDevices() bool {
	if s.Selection.getUseAllDevices() {
		return true
	}

	for _, n := range s.Nodes {
		if n.Selection.getUseAllDevices() {
			return true
		}
	}

	return false
}

func (s *StorageSpec) ClearUseAllDevices() {
	clear := false
	s.Selection.UseAllDevices = &clear
	for i := range s.Nodes {
		s.Nodes[i].Selection.UseAllDevices = &clear
	}
}

// Fully resolves the config of the given node name, taking into account cluster level and node level specified config.
// In general, the more fine grained the configuration is specified, the more precedence it takes.  Fully resolved
// configuration for the node has the following order of precedence.
// 1) Node (config defined on the node itself)
// 2) Cluster (config defined on the cluster)
// 3) Default values (if no config exists for the node or cluster)
func (s *StorageSpec) resolveNode(nodeName string) *Node {
	// find the requested storage node first, if it exists
	var node *Node
	for i := range s.Nodes {
		if s.Nodes[i].Name == nodeName {
			node = &(s.Nodes[i])
			break
		}
	}

	if node == nil {
		// a node with the given name was not found
		return nil
	}

	// now resolve all properties that haven't already been set on the node
	s.resolveNodeSelection(node)
	s.resolveNodeConfig(node)

	return node
}

func (s *StorageSpec) resolveNodeSelection(node *Node) {
	resolveString(&(node.Selection.DeviceFilter), s.Selection.DeviceFilter, "")
	resolveString(&(node.Selection.MetadataDevice), s.Selection.MetadataDevice, "")

	if node.Selection.UseAllDevices == nil {
		if s.Selection.UseAllDevices != nil {
			// the node does not have a value specified for use all devices, but the cluster does. Use the cluster's.
			node.Selection.UseAllDevices = s.Selection.UseAllDevices
		} else {
			// neither node nor cluster have a value set for use all devices, use the default value.
			node.Selection.UseAllDevices = newBool(false)
		}
	}
}

func (s *StorageSpec) resolveNodeConfig(node *Node) {
	resolveString(&(node.Config.StoreConfig.StoreType), s.Config.StoreConfig.StoreType, cephosd.DefaultStore)
	resolveInt(&(node.Config.StoreConfig.DatabaseSizeMB), s.Config.StoreConfig.DatabaseSizeMB, 0)
	resolveInt(&(node.Config.StoreConfig.WalSizeMB), s.Config.StoreConfig.WalSizeMB, 0)
	resolveInt(&(node.Config.StoreConfig.JournalSizeMB), s.Config.StoreConfig.JournalSizeMB, 0)
	resolveString(&(node.Config.Location), s.Config.Location, "")
}

func (s *Selection) getUseAllDevices() bool {
	return s.UseAllDevices != nil && *(s.UseAllDevices)
}

func resolveString(setting *string, parent, defaultVal string) {
	if *setting == "" {
		if parent != "" {
			*setting = parent
		} else {
			*setting = defaultVal
		}
	}
}

func resolveInt(setting *int, parent, defaultVal int) {
	if *setting == 0 {
		if parent != 0 {
			*setting = parent
		} else {
			*setting = defaultVal
		}
	}
}

func newBool(val bool) *bool {
	return &val
}
