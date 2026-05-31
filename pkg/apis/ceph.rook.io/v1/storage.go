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

import "fmt"

type StoreType string

const (
	// StoreTypeBlueStore is the bluestore backend storage for OSDs
	StoreTypeBlueStore StoreType = "bluestore"

	// StoreTypeBlueStoreRDR is the bluestore-rdr backed storage for OSDs
	StoreTypeBlueStoreRDR StoreType = "bluestore-rdr"
)

// AnyUseAllDevices gets whether to use all devices
func (s *StorageScopeSpec) AnyUseAllDevices() bool {
	if s.Selection.GetUseAllDevices() {
		return true
	}

	for _, n := range s.Nodes {
		if n.Selection.GetUseAllDevices() {
			return true
		}
	}

	return false
}

// ClearUseAllDevices clears all devices
func (s *StorageScopeSpec) ClearUseAllDevices() {
	clear := false
	s.Selection.UseAllDevices = &clear
	for i := range s.Nodes {
		s.Nodes[i].Selection.UseAllDevices = &clear
	}
}

// NodeExists returns true if the node exists in the storage spec. False otherwise.
func (s *StorageScopeSpec) NodeExists(nodeName string) bool {
	for i := range s.Nodes {
		if s.Nodes[i].Name == nodeName {
			return true
		}
	}
	return false
}

// Fully resolves the config of the given node name, taking into account cluster level and node level specified config.
// In general, the more fine grained the configuration is specified, the more precedence it takes.  Fully resolved
// configuration for the node has the following order of precedence.
// 1) Node (config defined on the node itself)
// 2) Cluster (config defined on the cluster)
// 3) Default values (if no config exists for the node or cluster)
func (s *StorageScopeSpec) ResolveNode(nodeName string) *Node {
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
	if node.Config == nil {
		node.Config = map[string]string{}
	}

	// now resolve all properties that haven't already been set on the node
	s.resolveNodeSelection(node)
	s.resolveNodeConfig(node)

	return node
}

func (s *StorageScopeSpec) resolveNodeSelection(node *Node) {
	if node.Selection.UseAllDevices == nil {
		if s.Selection.UseAllDevices != nil {
			// the node does not have a value specified for use all devices, but the cluster does. Use the cluster's.
			node.Selection.UseAllDevices = s.Selection.UseAllDevices
		} else {
			// neither node nor cluster have a value set for use all devices, use the default value.
			node.Selection.UseAllDevices = newBool(false)
		}
	}

	resolveString(&(node.Selection.DeviceFilter), s.Selection.DeviceFilter, "")
	resolveString(&(node.Selection.DevicePathFilter), s.Selection.DevicePathFilter, "")

	if len(node.Selection.Devices) == 0 {
		node.Selection.Devices = s.Devices
	}

	if len(node.Selection.VolumeClaimTemplates) == 0 {
		node.Selection.VolumeClaimTemplates = s.VolumeClaimTemplates
	}
}

func (s *StorageScopeSpec) resolveNodeConfig(node *Node) {
	// check for any keys the parent scope has that the node does not
	for scopeKey, scopeVal := range s.Config {
		if _, ok := node.Config[scopeKey]; !ok {
			// the node's config does not have an entry that the parent scope does, add the parent's
			// value for that key to the node's config.
			node.Config[scopeKey] = scopeVal
		}
	}
}

// NodeWithNameExists returns true if the storage spec defines a node with the given name.
func (s *StorageScopeSpec) NodeWithNameExists(name string) bool {
	for _, n := range s.Nodes {
		if name == n.Name {
			return true
		}
	}
	return false
}

// GetUseAllDevices return if all devices should be used.
func (s *Selection) GetUseAllDevices() bool {
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

func newBool(val bool) *bool {
	return &val
}

// NodesByName implements an interface to sort nodes by name
type NodesByName []Node

func (s NodesByName) Len() int {
	return len(s)
}

func (s NodesByName) Swap(i, j int) {
	s[i], s[j] = s[j], s[i]
}

func (s NodesByName) Less(i, j int) bool {
	return s[i].Name < s[j].Name
}

// IsOnPVCEncrypted returns whether a Ceph Cluster on PVC will be encrypted
func (s *StorageScopeSpec) IsOnPVCEncrypted() bool {
	for _, storageClassDeviceSet := range s.StorageClassDeviceSets {
		if storageClassDeviceSet.Encrypted {
			return true
		}
	}

	return false
}

// GetOSDStore returns osd backend store type provided in the cluster spec
func (s *StorageScopeSpec) GetOSDStore() string {
	if s.Store.Type == "" {
		return string(StoreTypeBlueStore)
	}
	return s.Store.Type
}

// GetOSDStoreFlag returns osd backend store type prefixed with "--"
func (s *StorageScopeSpec) GetOSDStoreFlag() string {
	if s.Store.Type == "" {
		return fmt.Sprintf("--%s", StoreTypeBlueStore)
	}
	return fmt.Sprintf("--%s", s.Store.Type)
}
