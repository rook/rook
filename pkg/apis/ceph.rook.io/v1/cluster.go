/*
Copyright 2020 The Rook Authors. All rights reserved.

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

// RequireMsgr2 checks if the network settings require the msgr2 protocol
func (c *ClusterSpec) RequireMsgr2() bool {
	if c.Network.Connections == nil {
		return false
	}
	if c.Network.Connections.RequireMsgr2 {
		return true
	}
	if c.Network.Connections.Compression != nil && c.Network.Connections.Compression.Enabled {
		return true
	}
	if c.Network.Connections.Encryption != nil && c.Network.Connections.Encryption.Enabled {
		return true
	}
	return false
}

// RequireMsgr2 checks if the network settings require the msgr2 protocol
func (c *ClusterSpec) NetworkEncryptionEnabled() bool {
	if c.Network.Connections == nil {
		return false
	}
	if c.Network.Connections.Encryption == nil {
		return false
	}
	return c.Network.Connections.Encryption.Enabled
}

func (c *ClusterSpec) IsStretchCluster() bool {
	return c.Mon.StretchCluster != nil && len(c.Mon.StretchCluster.Zones) > 0
}

func (c *ClusterSpec) ZonesRequired() bool {
	return c.IsStretchCluster() || len(c.Mon.Zones) > 0
}

func (c *CephCluster) GetStatusConditions() *[]Condition {
	return &c.Status.Conditions
}
