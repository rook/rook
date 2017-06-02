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
package mds

import (
	"fmt"
	"path"
	"strconv"
	"strings"

	etcd "github.com/coreos/etcd/client"
	ctx "golang.org/x/net/context"

	"github.com/rook/rook/pkg/ceph/client"
	"github.com/rook/rook/pkg/ceph/mon"
	"github.com/rook/rook/pkg/clusterd"
	"github.com/rook/rook/pkg/util"
)

const (
	defaultFileSystemName = "rookfs"
	defaultPoolName       = "fspool"
	poolKeyName           = "pool"
	idKeyName             = "id"
	dataPoolSuffix        = "-data"
	metadataPoolSuffix    = "-metadata"
	mdsKeyName            = "mds"
	FileSystemKey         = "fs"
)

// A castle file system (an instance of CephFS)
type FileSystem struct {
	context *clusterd.Context
	ID      string `json:"id"`
	Pool    string `json:"pool"`
}

type mdsInfo struct {
	nodeID     string
	mdsID      string
	fileSystem string
}

// Create a new file service struct
func NewFS(context *clusterd.Context, id, pool string) *FileSystem {
	return &FileSystem{
		ID:      id,
		Pool:    pool,
		context: context,
	}
}

func (f *FileSystem) CreateFilesystem(cluster *mon.ClusterInfo) error {
	logger.Infof("Creating file system %s", f.ID)
	_, err := client.GetFilesystem(f.context, cluster.Name, f.ID)
	if err == nil {
		logger.Infof("file system %s already exists", f.ID)
		return nil
	}

	dataPool := f.Pool + dataPoolSuffix
	metadataPool := f.Pool + metadataPoolSuffix

	// Create the metadata and data pools
	pool := client.CephStoragePoolDetails{Name: dataPool}
	_, err = client.CreatePool(f.context, cluster.Name, pool)
	if err != nil {
		return fmt.Errorf("failed to create data pool '%s': %+v", dataPool, err)
	}

	pool = client.CephStoragePoolDetails{Name: metadataPool}
	_, err = client.CreatePool(f.context, cluster.Name, pool)
	if err != nil {
		return fmt.Errorf("failed to create metadata pool '%s': %+v", metadataPool, err)
	}

	// create the file system
	if err := client.CreateFilesystem(f.context, cluster.Name, f.ID, metadataPool, dataPool); err != nil {
		return err
	}

	logger.Infof("created file system %s on data pool %s and metadata pool %s", f.ID, dataPool, metadataPool)
	return nil
}

// Create the file system in ceph
func (f *FileSystem) enable(cluster *mon.ClusterInfo) error {

	// start MDS instances in the cluster
	mdsCount := 1
	err := f.startMDSUnitInstances(mdsCount)
	if err != nil {
		return fmt.Errorf("Failed to start MDS. err=%v", err)
	}

	err = f.markApplied()
	if err != nil {
		return fmt.Errorf("failed to mark file system applied. %+v", err)
	}

	logger.Infof("Created file system %s", f.ID)
	return nil
}

// Remove the file system in ceph
func (f *FileSystem) disable(cluster *mon.ClusterInfo) error {
	logger.Infof("Removing file system %s", f.ID)

	// mark the cephFS instance as cluster_down before removing
	if err := client.MarkFilesystemAsDown(f.context, cluster.Name, f.ID); err != nil {
		return err
	}

	// mark each MDS associated with the file system to "failed"
	fsDetails, err := client.GetFilesystem(f.context, cluster.Name, f.ID)
	if err != nil {
		return err
	}
	for _, mdsInfo := range fsDetails.MDSMap.Info {
		if err := client.FailMDS(f.context, cluster.Name, mdsInfo.GID); err != nil {
			return err
		}
	}

	// Stop MDS
	if err := f.stopMDSUnitInstances(); err != nil {
		return fmt.Errorf("failed to stop mds instances. %+v", err)
	}

	// Permanently remove the file system
	if err := client.RemoveFilesystem(f.context, cluster.Name, f.ID); err != nil {
		return err
	}

	err = f.markUnapplied()
	if err != nil {
		return fmt.Errorf("failed to mark file system unapplied. %+v", err)
	}

	logger.Infof("Removed file system %s", f.ID)
	return nil
}

// Add the file system to the desired state.
func (f *FileSystem) AddToDesiredState() error {
	return f.storeSettings(false)
}

// Remove the file system from the desired state.
func (f *FileSystem) DeleteFromDesiredState() error {
	// remove the file system from desired state
	_, err := f.context.EtcdClient.Delete(ctx.Background(), getFileKey(f.ID, false), &etcd.DeleteOptions{Dir: true, Recursive: true})
	if err != nil && !util.IsEtcdKeyNotFound(err) {
		return fmt.Errorf("failed to delete file system %s from desired state. %+v", f.ID, err)
	}

	// remove the mds from desired state
	idMap, _, _, err := f.loadMDS(f.ID, false)
	if err != nil {
		return fmt.Errorf("failed to load desired MDS for removal. %+v", err)
	}

	for _, mds := range idMap {
		// TODO: remove the mds for this file system from desired state
		if err := f.removeMDSState(mds, false); err != nil {
			return fmt.Errorf("failed to remove mds from desired state. %+v", err)
		}
	}

	return err
}

func (f *FileSystem) removeMDSState(fs *mdsInfo, applied bool) error {
	key := path.Join(getMDSDesiredNodesKey(applied), fs.nodeID)
	_, err := f.context.EtcdClient.Delete(ctx.Background(), key, &etcd.DeleteOptions{Dir: true, Recursive: true})
	if err != nil {
		return fmt.Errorf("failed to delete mds %s from desired state. %+v", fs.mdsID, err)
	}

	return nil
}

// Persist the file system config to the applied key
func (f *FileSystem) markApplied() error {
	return f.storeSettings(true)
}

// Remove the file system from the applied key
func (f *FileSystem) markUnapplied() error {
	_, err := f.context.EtcdClient.Delete(ctx.Background(), getFileKey(f.ID, true), &etcd.DeleteOptions{Dir: true, Recursive: true})
	return err
}

func getFileRootKey(applied bool) string {
	return path.Join(mon.CephKey, FileSystemKey, getAppliedStr(applied))
}

func getFileKey(name string, applied bool) string {
	return path.Join(mon.CephKey, FileSystemKey, getAppliedStr(applied), name)
}

func getAppliedStr(applied bool) string {
	if applied {
		return clusterd.AppliedKey
	}
	return clusterd.DesiredKey
}

func (f *FileSystem) storeSettings(applied bool) error {

	key := path.Join(getFileKey(f.ID, applied), poolKeyName)
	_, err := f.context.EtcdClient.Set(ctx.Background(), key, f.Pool, nil)
	return err
}

func (f *FileSystem) startMDSUnitInstances(count int) error {
	desiredMDS, err := f.getDesiredMDSIDs(count)
	if err != nil {
		return fmt.Errorf("failed to get desired mds instances. %+v", err)
	}

	var nodeIDs []string
	for _, mds := range desiredMDS {
		nodeIDs = append(nodeIDs, mds.nodeID)
	}

	// trigger the mds to start on each node
	logger.Infof("Triggering mds on nodes: %+v", nodeIDs)
	err = clusterd.TriggerAgentsAndWaitForCompletion(f.context.EtcdClient, nodeIDs, mdsAgentName, len(desiredMDS))
	if err != nil {
		return fmt.Errorf("failed to deploy mds agents. %+v", err)
	}

	// set the mds as applied
	for _, mds := range desiredMDS {
		if err := f.storeMDSState(mds, true); err != nil {
			return fmt.Errorf("failed to set mds agent as applied. %+v", err)
		}
	}

	return nil
}

func (f *FileSystem) stopMDSUnitInstances() error {
	appliedMDS, _, _, err := f.loadMDS(f.ID, true)
	if err != nil {
		return fmt.Errorf("failed to get desired mds instances. %+v", err)
	}

	var nodeIDs []string
	for _, mds := range appliedMDS {
		nodeIDs = append(nodeIDs, mds.nodeID)
	}

	// trigger the monitors to start on each node
	logger.Infof("Triggering removal of mds on nodes: %+v", nodeIDs)
	err = clusterd.TriggerAgentsAndWaitForCompletion(f.context.EtcdClient, nodeIDs, mdsAgentName, len(nodeIDs))
	if err != nil {
		return fmt.Errorf("failed to deploy mds agents. %+v", err)
	}

	// remove the mds from applied
	for _, mds := range appliedMDS {
		if err := f.removeMDSState(mds, true); err != nil {
			return fmt.Errorf("failed to set mds agent as applied. %+v", err)
		}
	}

	return nil
}

func (f *FileSystem) getDesiredMDSIDs(count int) (map[string]*mdsInfo, error) {
	idMap, nodes, nextID, err := f.loadMDS("", false)
	if err != nil {
		return nil, fmt.Errorf("failed to load desired MDS IDs. %+v", err)
	}

	// Assign MDS to nodes if not already assigned
	for nodeID := range f.context.Inventory.Nodes {
		// we have enough mds instances
		if len(idMap) >= count {
			break
		}

		// cannot use the same node for more than one mds
		if nodes.Contains(nodeID) {
			continue
		}

		// assign this node an MDS ID
		mds := &mdsInfo{
			nodeID:     nodeID,
			mdsID:      strconv.Itoa(nextID),
			fileSystem: f.ID,
		}
		idMap[mds.mdsID] = mds
		nextID++

		if err := f.storeMDSState(mds, false); err != nil {
			return nil, err
		}
	}

	if len(idMap) < count {
		return nil, fmt.Errorf("not enough nodes for mds services. required=%d, actual=%d", count, len(idMap))
	}

	return idMap, nil
}

func (f *FileSystem) storeMDSState(info *mdsInfo, applied bool) error {
	// store the mds in desired state
	if _, err := f.context.EtcdClient.Set(ctx.Background(), getMDSIDKey(info.nodeID, applied), info.mdsID, nil); err != nil {
		return fmt.Errorf("failed to set desired mds %s. %+v", info.mdsID, err)
	}
	if _, err := f.context.EtcdClient.Set(ctx.Background(), getMDSFileSystemKey(info.nodeID, applied), f.ID, nil); err != nil {
		return fmt.Errorf("failed to set desired mds file system %s. %+v", f.ID, err)
	}

	return nil
}

// Load where the MDS services are intended to be running, on which nodes and for which file systems
func (f *FileSystem) loadMDS(fileSystemFilter string, applied bool) (map[string]*mdsInfo, *util.Set, int, error) {
	idToNodeMap := make(map[string]*mdsInfo)
	nextID := 1
	nodes := util.NewSet()

	key := getMDSDesiredNodesKey(applied)
	result, err := f.context.EtcdClient.Get(ctx.Background(), key, &etcd.GetOptions{Recursive: true})
	if err != nil {
		if util.IsEtcdKeyNotFound(err) {
			return idToNodeMap, nodes, nextID, nil
		}

		return nil, nil, -1, err
	}

	// load the desired IDs that have already been generated previously
	for _, nodeKey := range result.Node.Nodes {
		info := &mdsInfo{
			nodeID: util.GetLeafKeyPath(nodeKey.Key),
		}
		mdsID := -1
		for _, setting := range nodeKey.Nodes {
			if strings.HasSuffix(setting.Key, "/id") {
				var err error
				info.mdsID = setting.Value
				mdsID, err = strconv.Atoi(setting.Value)
				if err != nil {
					return nil, nil, -1, fmt.Errorf("bad mds id %s. %+v", setting.Value, err)
				}

			} else if strings.HasSuffix(setting.Key, "/filesystem") {
				info.fileSystem = setting.Value
			}
		}

		// if the file system name matches, or a specific file system name was not requested, add it to the result
		if f.ID == fileSystemFilter || fileSystemFilter == "" {
			idToNodeMap[info.mdsID] = info
			nodes.Add(info.nodeID)
		}

		if mdsID >= nextID {
			nextID = mdsID + 1
		}
	}

	return idToNodeMap, nodes, nextID, nil
}
