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

	"github.com/rook/rook/pkg/clusterd"
	"github.com/rook/rook/pkg/daemon/ceph/client"
	"github.com/rook/rook/pkg/daemon/ceph/model"
)

const (
	defaultFilesystemName = "rookfs"
	defaultPoolName       = "fspool"
	poolKeyName           = "pool"
	idKeyName             = "id"
	dataPoolSuffix        = "data"
	metadataPoolSuffix    = "metadata"
	mdsKeyName            = "mds"
	FilesystemKey         = "fs"
	appName               = "cephfs"
)

// A file system (an instance of CephFS)
type Filesystem struct {
	Name           string
	metadataPool   *model.Pool
	dataPools      []*model.Pool
	activeMDSCount int32
}

// Create a new file service struct
func NewFS(name string, metadataPool *model.Pool, dataPools []*model.Pool, activeMDSCount int32) *Filesystem {

	metadataPool.Name = fmt.Sprintf("%s-%s", name, metadataPoolSuffix)
	for i, pool := range dataPools {
		pool.Name = fmt.Sprintf("%s-%s%d", name, dataPoolSuffix, i)
	}

	return &Filesystem{
		Name:           name,
		metadataPool:   metadataPool,
		dataPools:      dataPools,
		activeMDSCount: activeMDSCount,
	}
}

func (f *Filesystem) CreateFilesystem(context *clusterd.Context, clusterName string) error {
	_, err := client.GetFilesystem(context, clusterName, f.Name)
	if err == nil {
		logger.Infof("file system %s already exists", f.Name)
		return nil
	}
	if len(f.dataPools) == 0 {
		return fmt.Errorf("at least one data pool must be specified")
	}

	fslist, err := client.ListFilesystems(context, clusterName)
	if err != nil {
		return fmt.Errorf("Unable to list existing file systems. %+v", err)
	}
	if len(fslist) > 0 && !client.IsMultiFSEnabled() {
		return fmt.Errorf("Cannot create multiple filesystems. Enable %s env variable to create more than one", client.MultiFsEnv)
	}

	logger.Infof("Creating file system %s", f.Name)
	err = client.CreatePoolWithProfile(context, clusterName, *f.metadataPool, appName)
	if err != nil {
		return fmt.Errorf("failed to create metadata pool '%s': %+v", f.metadataPool.Name, err)
	}

	var dataPoolNames []string
	for _, pool := range f.dataPools {
		dataPoolNames = append(dataPoolNames, pool.Name)
		err = client.CreatePoolWithProfile(context, clusterName, *pool, appName)
		if err != nil {
			return fmt.Errorf("failed to create data pool %s. %+v", pool.Name, err)
		}
		if pool.Type == model.ErasureCoded {
			// An erasure coded data pool used for a file system must allow overwrites
			if err := client.SetPoolProperty(context, clusterName, pool.Name, "allow_ec_overwrites", "true"); err != nil {
				logger.Warningf("failed to set ec pool property. %+v", err)
			}
		}
	}

	// create the file system
	if err := client.CreateFilesystem(context, clusterName, f.Name, f.metadataPool.Name, dataPoolNames, f.activeMDSCount); err != nil {
		return err
	}

	logger.Infof("created file system %s on %d data pool(s) and metadata pool %s", f.Name, len(f.dataPools), f.metadataPool.Name)
	return nil
}

// Remove the file system in ceph
func DeleteFilesystem(context *clusterd.Context, clusterName, filesystemName string) error {
	logger.Infof("Removing file system %s", filesystemName)

	// mark the cephFS instance as cluster_down before removing
	if err := client.MarkFilesystemAsDown(context, clusterName, filesystemName); err != nil {
		return err
	}

	// mark each MDS associated with the file system to "failed"
	fsDetails, err := client.GetFilesystem(context, clusterName, filesystemName)
	if err != nil {
		return err
	}
	for _, mdsInfo := range fsDetails.MDSMap.Info {
		if err := client.FailMDS(context, clusterName, mdsInfo.GID); err != nil {
			return err
		}
	}

	// Permanently remove the file system
	if err := client.RemoveFilesystem(context, clusterName, filesystemName); err != nil {
		return err
	}

	logger.Infof("Removed file system %s", filesystemName)
	return nil
}
