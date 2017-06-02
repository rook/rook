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
package ceph

import (
	"fmt"
	"path"
	"strings"

	etcd "github.com/coreos/etcd/client"
	"github.com/coreos/etcd/store"
	"github.com/rook/rook/pkg/ceph/mds"
	"github.com/rook/rook/pkg/ceph/mon"
	"github.com/rook/rook/pkg/ceph/osd"
	"github.com/rook/rook/pkg/ceph/rgw"
	"github.com/rook/rook/pkg/clusterd"
	"github.com/rook/rook/pkg/util"
)

// Interface implemented by a service that has been elected leader
type cephLeader struct {
	monLeader          *mon.Leader
	osdLeader          *osd.Leader
	mdsLeader          *mds.Leader
	rgwLeader          *rgw.Leader
	adminSecret        string
	refreshInitialized bool
}

func newLeader(adminSecret string) *cephLeader {
	return &cephLeader{
		monLeader:   mon.NewLeader(),
		osdLeader:   osd.NewLeader(),
		mdsLeader:   mds.NewLeader(),
		rgwLeader:   rgw.NewLeader(),
		adminSecret: adminSecret}
}

func (c *cephLeader) RefreshKeys() []*clusterd.RefreshKey {
	// when devices are added or removed we will want to trigger an orchestration
	deviceChange := &clusterd.RefreshKey{
		Path:      path.Join(mon.CephKey, "osd", clusterd.DesiredKey),
		Triggered: handleDeviceChanged,
	}
	fileChange := &clusterd.RefreshKey{
		Path:      path.Join(mon.CephKey, mds.FileSystemKey, clusterd.DesiredKey),
		Triggered: handleFileSystemChanged,
	}
	objectChange := &clusterd.RefreshKey{
		Path:      path.Join(mon.CephKey, rgw.ObjectStoreKey, clusterd.DesiredKey),
		Triggered: handleObjectStoreChanged,
	}
	return []*clusterd.RefreshKey{deviceChange, fileChange, objectChange}
}

func getOSDsToRefresh(e *clusterd.RefreshEvent, refreshInitialized bool) *util.Set {
	osds := util.NewSet()
	osds.AddSet(e.NodesAdded)
	osds.AddSet(e.NodesChanged)
	osds.AddSet(e.NodesRemoved)

	// Nothing changed in the event or this is the first refresh, so refresh osds on all nodes
	if osds.Count() == 0 || !refreshInitialized {
		for nodeID := range e.Context.Inventory.Nodes {
			osds.Add(nodeID)
		}
	}

	return osds
}

func getRefreshMons(e *clusterd.RefreshEvent) bool {
	return true
}

func getRefreshFile(e *clusterd.RefreshEvent) bool {
	return true
}

func getRefreshObject(e *clusterd.RefreshEvent) bool {
	return true
}

func (c *cephLeader) HandleRefresh(e *clusterd.RefreshEvent) {
	// Listen for events from the orchestrator indicating that a refresh is needed or nodes have been added
	logger.Infof("ceph leader received refresh event")

	refreshMons := getRefreshMons(e)
	osdsToRefresh := getOSDsToRefresh(e, c.refreshInitialized)
	refreshFile := getRefreshFile(e)
	refreshObject := getRefreshObject(e)

	if refreshMons {
		// Perform a full refresh of the cluster to ensure the monitors are running with quorum
		err := c.monLeader.Configure(e.Context, c.adminSecret)
		if err != nil {
			logger.Errorf("Failed to configure ceph mons. %v", err)
		}
	}

	if osdsToRefresh.Count() > 0 {
		// Configure the OSDs
		err := c.osdLeader.Configure(e.Context, osdsToRefresh.ToSlice())
		if err != nil {
			logger.Errorf("Failed to configure ceph OSDs. %v", err)
		}
	}

	if refreshFile {
		// Configure the file system(s)
		err := c.mdsLeader.Configure(e.Context)
		if err != nil {
			logger.Errorf("Failed to configure file service. %+v", err)
		}
	}

	if refreshObject {
		err := c.rgwLeader.Configure(e.Context)
		if err != nil {
			logger.Errorf("Failed to configure object service. %+v", err)
		}
	}

	c.refreshInitialized = true
	logger.Infof("ceph leader completed refresh")
}

func handleDeviceChanged(response *etcd.Response, refresher *clusterd.ClusterRefresher) {
	if response.Action == store.Create || response.Action == store.Delete {
		nodeID, err := extractNodeIDFromDesiredDevice(response.Node.Key)
		if err != nil {
			logger.Warningf("ignored device changed event. %v", err)
			return
		}

		logger.Infof("device changed: %s", nodeID)

		// trigger an orchestration to add or remove the device
		refresher.TriggerDevicesChanged(nodeID)
	}
}

// Get the node ID from the etcd key to a desired device
// For example: /rook/services/ceph/osd/desired/9b69e58300f9/device/sdb
func extractNodeIDFromDesiredDevice(path string) (string, error) {
	parts := strings.Split(path, "/")
	const nodeIDOffset = 6
	if len(parts) < nodeIDOffset+1 {
		return "", fmt.Errorf("cannot get node ID from %s", path)
	}

	return parts[nodeIDOffset], nil
}

func handleFileSystemChanged(response *etcd.Response, refresher *clusterd.ClusterRefresher) {
	logger.Debugf("handling file system changed. %+v", response)

	// trigger an orchestration to add or remove the file system
	refresher.TriggerRefresh()
}

func handleObjectStoreChanged(response *etcd.Response, refresher *clusterd.ClusterRefresher) {
	logger.Debugf("handling object store changed. %+v", response)

	// trigger an orchestration to add or remove the object store
	refresher.TriggerRefresh()
}
