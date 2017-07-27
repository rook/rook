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

	ctx "golang.org/x/net/context"

	"github.com/rook/rook/pkg/ceph/mon"
	"github.com/rook/rook/pkg/clusterd"
	"github.com/rook/rook/pkg/model"
	"github.com/rook/rook/pkg/util"
)

const (
	MDSKey = "mds"
)

type Leader struct {
}

func NewLeader() *Leader {
	return &Leader{}
}

// Apply the desired state to the cluster. The context provides all the information needed to make changes to the service.
// Initialize CephFS. Must be idempotent.
func (r *Leader) Configure(context *clusterd.Context) error {
	applieds, err := r.loadFileSystems(context, true)
	if err != nil {
		return fmt.Errorf("failed to get applied file systems. %+v", err)
	}

	desireds, err := r.loadFileSystems(context, false)
	if err != nil {
		return fmt.Errorf("failed to get desired file systems. %+v", err)
	}

	cluster, err := mon.LoadClusterInfo(context.EtcdClient)
	if err != nil {
		return fmt.Errorf("failed to get cluster info for fs. %+v", err)
	}

	// add file systems that are desired
	for name, desired := range desireds {
		if _, ok := applieds[name]; ok {
			// the fs is already applied
			continue
		}

		if err := desired.enable(cluster); err != nil {
			return fmt.Errorf("failed to config file service. %+v", err)
		}
	}

	// remove file systems that are no longer desired
	for name, applied := range applieds {
		if _, ok := desireds[name]; ok {
			// the applied is also desired
			continue
		}

		if err := applied.disable(cluster); err != nil {
			return fmt.Errorf("failed to config file service. %+v", err)
		}
	}

	return nil
}

// Remove the given file system instance in the cluster. Multiple file systems are supported.
func RemoveFileSystem(context *clusterd.Context, fsr model.FilesystemRequest) error {
	logger.Infof("Removing file system %s", fsr.Name)
	fs := &FileSystem{context: context, ID: fsr.Name}
	return fs.DeleteFromDesiredState()
}

// Load the desired or applied file system state
func (r *Leader) loadFileSystems(context *clusterd.Context, applied bool) (map[string]*FileSystem, error) {
	fileSystems := map[string]*FileSystem{}
	children, err := util.GetDirChildKeys(context.EtcdClient, getFileRootKey(applied))
	if err != nil {
		if util.IsEtcdKeyNotFound(err) {
			return fileSystems, nil
		}
		return nil, fmt.Errorf("failed to get file systems. %+v", err)
	}

	for name := range children.Iter() {
		key := path.Join(getFileKey(name, applied), poolKeyName)
		poolVal, err := context.EtcdClient.Get(ctx.Background(), key, nil)
		if err != nil {
			return nil, fmt.Errorf("failed to get pool name for fs %s. %+v", name, err)
		}

		// Return the file system
		fileSystems[name] = NewFS(context, name, poolVal.Node.Value)
	}

	return fileSystems, nil
}
