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

package file

import (
	"fmt"

	cephv1 "github.com/rook/rook/pkg/apis/ceph.rook.io/v1"
	"github.com/rook/rook/pkg/clusterd"
	"github.com/rook/rook/pkg/daemon/ceph/client"
	mdsdaemon "github.com/rook/rook/pkg/daemon/ceph/mds"
	"github.com/rook/rook/pkg/daemon/ceph/model"
	"github.com/rook/rook/pkg/operator/ceph/pool"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// createFilesystem creates a Ceph filesystem with metadata servers
func createFilesystem(
	context *clusterd.Context,
	fs cephv1.CephFilesystem,
	rookVersion string,
	cephVersion cephv1.CephVersionSpec,
	hostNetwork bool,
	ownerRefs []metav1.OwnerReference,
) error {
	if err := validateFilesystem(context, fs); err != nil {
		return err
	}

	if len(fs.Spec.DataPools) != 0 {
		var dataPools []*model.Pool
		for _, p := range fs.Spec.DataPools {
			dataPools = append(dataPools, p.ToModel(""))
		}
		f := mdsdaemon.NewFS(fs.Name, fs.Spec.MetadataPool.ToModel(""), dataPools, fs.Spec.MetadataServer.ActiveCount)
		if err := f.CreateFilesystem(context, fs.Namespace); err != nil {
			return fmt.Errorf("failed to create file system %s: %+v", fs.Name, err)
		}
	}

	filesystem, err := client.GetFilesystem(context, fs.Namespace, fs.Name)
	if err != nil {
		return fmt.Errorf("failed to get file system %s: %+v", fs.Name, err)
	}

	// set the number of active mds instances
	if fs.Spec.MetadataServer.ActiveCount > 1 {
		if err = client.SetNumMDSRanks(context, fs.Namespace, fs.Name, fs.Spec.MetadataServer.ActiveCount); err != nil {
			logger.Warningf("failed setting active mds count to %d. %+v", fs.Spec.MetadataServer.ActiveCount, err)
		}
	}

	logger.Infof("start running mdses for file system %s", fs.Name)
	c := newCluster(context, rookVersion, cephVersion, hostNetwork, fs, filesystem, ownerRefs)
	if err := c.start(); err != nil {
		return err
	}

	return nil
}

// deleteFileSystem deletes the file system and the metadata servers
func deleteFilesystem(context *clusterd.Context, fs cephv1.CephFilesystem) error {
	// The most important part of deletion is that the filesystem gets removed from Ceph
	if err := mdsdaemon.DownFilesystem(context, fs.Namespace, fs.Name); err != nil {
		// If the fs isn't deleted from Ceph, leave the daemons so it can still be used.
		return fmt.Errorf("failed to down filesystem %s: %+v", fs.Name, err)
	}

	// Permanently remove the file system if it was created by rook
	if len(fs.Spec.DataPools) != 0 {
		if err := client.RemoveFilesystem(context, fs.Namespace, fs.Name); err != nil {
			return fmt.Errorf("failed to remove filesystem %s: %+v", fs.Name, err)
		}
	}

	return deleteMdsCluster(context, fs.Namespace, fs.Name)
}

func validateFilesystem(context *clusterd.Context, f cephv1.CephFilesystem) error {
	if f.Name == "" {
		return fmt.Errorf("missing name")
	}
	if f.Namespace == "" {
		return fmt.Errorf("missing namespace")
	}
	if f.Spec.MetadataServer.ActiveCount < 1 {
		return fmt.Errorf("MetadataServer.ActiveCount must be at least 1")
	}
	// No data pool means that we expect the fs to exist already
	if len(f.Spec.DataPools) == 0 {
		return nil
	}
	if err := pool.ValidatePoolSpec(context, f.Namespace, &f.Spec.MetadataPool); err != nil {
		return fmt.Errorf("invalid metadata pool: %+v", err)
	}
	for _, p := range f.Spec.DataPools {
		if err := pool.ValidatePoolSpec(context, f.Namespace, &p); err != nil {
			return fmt.Errorf("Invalid data pool: %+v", err)
		}
	}

	return nil
}
