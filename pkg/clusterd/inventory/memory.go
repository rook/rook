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
	"fmt"
	"path"
	"strconv"
	"syscall"

	etcd "github.com/coreos/etcd/client"
	ctx "golang.org/x/net/context"
)

func getSystemMemory() uint64 {
	sysInfo := new(syscall.Sysinfo_t)
	err := syscall.Sysinfo(sysInfo)
	if err == nil {
		return uint64(sysInfo.Totalram)
	}
	return 0
}

func storeMemory(etcdClient etcd.KeysAPI, nodeID string, size uint64) error {
	key := path.Join(NodesConfigKey, nodeID, memoryKey)
	_, err := etcdClient.Set(ctx.Background(), key, strconv.FormatUint(size, 10), nil)
	if err != nil {
		return fmt.Errorf("failed to store memory in etcd. %+v", err)
	}

	return nil
}

func loadMemoryConfig(nodeConfig *NodeConfig, rawMemory string) error {
	size, err := strconv.ParseUint(rawMemory, 10, 64)
	if err != nil {
		return fmt.Errorf("failed to parse memory. %+v", err)
	}
	nodeConfig.Memory = size
	return nil
}
