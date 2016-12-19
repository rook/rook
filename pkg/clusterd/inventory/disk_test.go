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

	etcd "github.com/coreos/etcd/client"
	ctx "golang.org/x/net/context"

	"github.com/rook/rook/pkg/util"
	"github.com/stretchr/testify/assert"
)

func TestGetSimpleDiskPropertiesFromSerial(t *testing.T) {
	nodeID := "df1c87e8266843f2ab822c0d72f584d3"
	etcdClient := &util.MockEtcdClient{}
	hardwareKey := path.Join(NodesConfigKey, nodeID)
	etcdClient.Set(ctx.Background(), hardwareKey, "", &etcd.SetOptions{Dir: true})
	TestSetDiskInfo(etcdClient, hardwareKey, "sda", "abcd4869-29ee-4bfd-bf21-dfd597bd222e",
		10737418240, true, false, "btrfs", "/mnt/abc", Disk, "", true)

	diskNode, _ := etcdClient.Get(ctx.Background(), path.Join(hardwareKey, "disks", "sda"), nil)
	disk, err := getDiskInfo(diskNode.Node)
	assert.Nil(t, err)

	assert.Equal(t, "sda", disk.Name)
	assert.Equal(t, "abcd4869-29ee-4bfd-bf21-dfd597bd222e", disk.UUID)
}

func TestAvailableDisks(t *testing.T) {
	nodeID := "a"
	etcdClient := &util.MockEtcdClient{}
	hardwareKey := path.Join(NodesConfigKey, nodeID)

	// no disks discovered for a node is an error
	disks, err := GetAvailableDevices(nodeID, etcdClient)
	assert.Equal(t, 0, len(disks))
	assert.NotNil(t, err)

	// no available disks because of the formatting
	TestSetDiskInfo(etcdClient, hardwareKey, "sda", "myuuid1", 123, true, false, "btrfs", "/mnt/abc", Disk, "", true)
	disks, err = GetAvailableDevices(nodeID, etcdClient)
	assert.Equal(t, 0, len(disks))
	assert.Nil(t, err)

	// multiple available disks
	TestSetDiskInfo(etcdClient, hardwareKey, "sdb", "myuuid2", 123, true, false, "", "", Disk, "", true)
	TestSetDiskInfo(etcdClient, hardwareKey, "sdc", "myuuid3", 123, true, false, "", "", Disk, "", true)
	disks, err = GetAvailableDevices(nodeID, etcdClient)
	assert.Equal(t, 2, len(disks))
	assert.Nil(t, err)

	// partitions don't result in more available devices
	TestSetDiskInfo(etcdClient, hardwareKey, "sdb1", "myuuid4", 123, true, false, "", "", Part, "sdb", true)
	TestSetDiskInfo(etcdClient, hardwareKey, "sdb2", "myuuid5", 123, true, false, "", "", Part, "sdb", true)
	disks, err = GetAvailableDevices(nodeID, etcdClient)
	assert.Equal(t, 2, len(disks))
	assert.Nil(t, err)
}
