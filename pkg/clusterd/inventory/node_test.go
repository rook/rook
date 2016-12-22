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
package inventory

import (
	"path"
	"testing"
	"time"

	"github.com/rook/rook/pkg/util"
	"github.com/stretchr/testify/assert"
)

func TestLoadDiscoveredNodes(t *testing.T) {
	etcdClient := &util.MockEtcdClient{}

	// Create some test config
	etcdClient.SetValue(path.Join(NodesConfigKey, "23", "publicIp"), "1.2.3.4")
	etcdClient.SetValue(path.Join(NodesConfigKey, "23", "privateIp"), "10.2.3.4")
	etcdClient.SetValue(path.Join(NodesConfigKey, "46", "publicIp"), "4.5.6.7")
	etcdClient.SetValue(path.Join(NodesConfigKey, "46", "privateIp"), "10.5.6.7")

	config, err := LoadDiscoveredNodes(etcdClient)
	assert.Nil(t, err)
	assert.Equal(t, 2, len(config.Nodes))
	assert.Equal(t, "1.2.3.4", config.Nodes["23"].PublicIP)
	assert.Equal(t, "10.2.3.4", config.Nodes["23"].PrivateIP)
	assert.Equal(t, "4.5.6.7", config.Nodes["46"].PublicIP)
	assert.Equal(t, "10.5.6.7", config.Nodes["46"].PrivateIP)
	assert.Equal(t, time.Hour*24*365, config.Nodes["23"].HeartbeatAge) // no heartbeat has an age of a year

	desiredPublicIP := "9.8.7.6"
	desiredPrivateIP := "10.7.6.7"
	err = SetIPAddress(etcdClient, "23", desiredPublicIP, desiredPrivateIP)
	assert.Nil(t, err)

	config, err = LoadDiscoveredNodes(etcdClient)
	assert.Nil(t, err)
	assert.Equal(t, 2, len(config.Nodes))
	assert.Equal(t, "9.8.7.6", config.Nodes["23"].PublicIP)
	assert.Equal(t, "10.7.6.7", config.Nodes["23"].PrivateIP)
	assert.Equal(t, "4.5.6.7", config.Nodes["46"].PublicIP)
	assert.Equal(t, "10.5.6.7", config.Nodes["46"].PrivateIP)
}

func TestLoadHardwareConfig(t *testing.T) {
	nodeID := "12345"
	etcdClient := util.NewMockEtcdClient()

	// setup disk info in etcd
	d1 := LocalDisk{Name: "sda", UUID: "uuid1", Size: 10737418240, Rotational: true, Readonly: false, Type: DiskType, HasChildren: true}
	d2 := LocalDisk{Name: "sda2", UUID: "uuid2", Size: 2097152, Rotational: false, Readonly: true, Type: PartType, HasChildren: false}
	err := storeDevices(etcdClient, nodeID, []LocalDisk{d1, d2})
	assert.Nil(t, err)

	// setup processor info in etcd
	p1 := ProcessorConfig{ID: 0, PhysicalID: 3, Siblings: 1, CoreID: 6, NumCores: 1, Speed: 1234.56, Bits: 64}
	p2 := ProcessorConfig{ID: 1, PhysicalID: 4, Siblings: 2, CoreID: 7, NumCores: 2, Speed: 8000.00, Bits: 32}
	p3 := ProcessorConfig{ID: 2, PhysicalID: 5, Siblings: 0, CoreID: 8, NumCores: 4, Speed: 4000.01, Bits: 32}
	err = storeProcessorConfig(etcdClient, nodeID, []ProcessorConfig{p1, p2, p3})
	assert.Nil(t, err)

	// setup memory info in etcd
	mem := getSystemMemory()
	err = storeMemory(etcdClient, nodeID, mem)
	assert.Nil(t, err)

	// set up network info in etcd
	n1 := NetworkConfig{Name: "eth0", IPv4Address: "172.17.42.1/16", IPv6Address: "fe80::42:4aff:fefe:13d7/64", Speed: 0}
	n2 := NetworkConfig{Name: "veth2b6453a", IPv6Address: "fe80::7c0f:acff:feff:478d/64", Speed: 10000}
	err = storeNetworkConfig(etcdClient, nodeID, []NetworkConfig{n1, n2})
	assert.Nil(t, err)

	// set IP address in etcd
	publicIP := "10.0.0.43"
	privateIP := "10.2.3.4"
	SetIPAddress(etcdClient, nodeID, publicIP, privateIP)

	// set location in etcd
	SetLocation(etcdClient, nodeID, "root=default,dc=datacenter1")

	// load the discovered node config
	nodeConfig, err := loadNodeConfig(etcdClient)
	assert.Nil(t, err, "loaded node config error should be nil")
	assert.NotNil(t, nodeConfig, "loaded node config should not be nil")
	assert.Equal(t, 1, len(nodeConfig))

	cfg := nodeConfig[nodeID]

	// verify the single disk (the partition is not saved)
	assert.Equal(t, 1, len(cfg.Disks))
	assert.True(t, cfg.Disks[0].Available)
	assert.Equal(t, d1.Rotational, cfg.Disks[0].Rotational)
	assert.Equal(t, d1.Size, cfg.Disks[0].Size)
	assert.Equal(t, d1.Type, cfg.Disks[0].Type)

	// verify the processors
	assert.Equal(t, 3, len(cfg.Processors))

	// verify the network adapters
	assert.Equal(t, 2, len(cfg.NetworkAdapters))

	// verify the simple values
	assert.Equal(t, mem, cfg.Memory)
	assert.Equal(t, "10.0.0.43", cfg.PublicIP)
	assert.Equal(t, "root=default,dc=datacenter1", cfg.Location)

	// verify that some values are in etcd
	key := path.Join(NodesConfigKey, nodeID)
	assert.Equal(t, publicIP, etcdClient.GetValue(path.Join(key, publicIpAddressKey)))
	assert.Equal(t, privateIP, etcdClient.GetValue(path.Join(key, privateIpAddressKey)))

}
