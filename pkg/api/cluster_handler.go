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
package api

import (
	"fmt"

	"github.com/rook/rook/pkg/ceph/mds"
	"github.com/rook/rook/pkg/ceph/mon"
	"github.com/rook/rook/pkg/ceph/rgw"
	"github.com/rook/rook/pkg/clusterd"
	"github.com/rook/rook/pkg/clusterd/inventory"
	"github.com/rook/rook/pkg/model"
	"github.com/rook/rook/pkg/util"
)

type ClusterHandler interface {
	GetClusterInfo() (*mon.ClusterInfo, error)
	EnableObjectStore() error
	RemoveObjectStore() error
	GetObjectStoreConnectionInfo() (s3info *model.ObjectStoreConnectInfo, found bool, err error)
	StartFileSystem(fs *model.FilesystemRequest) error
	RemoveFileSystem(fs *model.FilesystemRequest) error
	GetMonitors() (map[string]*mon.CephMonitorConfig, error)
	GetNodes() ([]model.Node, error)
}

type etcdHandler struct {
	context *clusterd.Context
}

func NewEtcdHandler(context *clusterd.Context) *etcdHandler {
	return &etcdHandler{context: context}
}

func (e *etcdHandler) GetClusterInfo() (*mon.ClusterInfo, error) {
	return mon.LoadClusterInfo(e.context.EtcdClient)
}

func (e *etcdHandler) EnableObjectStore() error {
	return rgw.EnableObjectStore(e.context.EtcdClient)
}

func (e *etcdHandler) RemoveObjectStore() error {
	return rgw.RemoveObjectStore(e.context.EtcdClient)
}

func (e *etcdHandler) GetObjectStoreConnectionInfo() (*model.ObjectStoreConnectInfo, bool, error) {

	clusterInventory, err := inventory.LoadDiscoveredNodes(e.context.EtcdClient)
	if err != nil {
		logger.Errorf("failed to load discovered nodes: %+v", err)
		return nil, true, err
	}

	host, ipEndpoint, found, err := rgw.GetRGWEndpoints(e.context.EtcdClient, clusterInventory)
	if err != nil {
		return nil, !util.IsEtcdKeyNotFound(err), err
	} else if !found {
		return nil, false, fmt.Errorf("failed to find rgw endpoints")
	}

	s3Info := &model.ObjectStoreConnectInfo{
		Host:       host,
		IPEndpoint: ipEndpoint,
	}

	return s3Info, true, nil
}

func (e *etcdHandler) StartFileSystem(fs *model.FilesystemRequest) error {
	f := mds.NewFS(e.context, fs.Name, fs.PoolName)
	return f.AddToDesiredState()
}

func (e *etcdHandler) RemoveFileSystem(fs *model.FilesystemRequest) error {
	return mds.RemoveFileSystem(e.context, *fs)
}

func (e *etcdHandler) GetMonitors() (map[string]*mon.CephMonitorConfig, error) {
	return mon.GetDesiredMonitors(e.context.EtcdClient)
}

func (e *etcdHandler) GetNodes() ([]model.Node, error) {
	clusterInventory, err := inventory.LoadDiscoveredNodes(e.context.EtcdClient)
	if err != nil {
		return nil, fmt.Errorf("failed to load discovered nodes: %+v", err)
	}

	clusterName, err := mon.GetClusterName(e.context.EtcdClient)
	if err != nil {
		return nil, fmt.Errorf("failed to get cluster name: %+v", err)
	}

	nodes := make([]model.Node, len(clusterInventory.Nodes))
	i := 0
	for nodeID, n := range clusterInventory.Nodes {
		storage := uint64(0)
		for _, d := range n.Disks {
			// Add up the space of all devices.
			// We should have a separate metric for osd devices, but keep it simple for now.
			storage += d.Size
		}

		// determine the node's state/health
		_, isUnhealthy := clusterd.IsNodeUnhealthy(n)
		var state model.NodeState
		if isUnhealthy {
			state = model.Unhealthy
		} else {
			state = model.Healthy
		}

		nodes[i] = model.Node{
			NodeID:      nodeID,
			ClusterName: clusterName,
			PublicIP:    n.PublicIP,
			PrivateIP:   n.PrivateIP,
			Storage:     storage,
			LastUpdated: n.HeartbeatAge,
			State:       state,
			Location:    n.Location,
		}

		i++
	}

	return nodes, nil
}
