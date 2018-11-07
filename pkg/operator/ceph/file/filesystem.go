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

	cephv1beta1 "github.com/rook/rook/pkg/apis/ceph.rook.io/v1beta1"
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
	fs cephv1beta1.Filesystem,
	rookVersion string,
	cephVersion cephv1beta1.CephVersionSpec,
	hostNetwork bool,
	ownerRefs []metav1.OwnerReference,
) error {
	if err := validateFilesystem(context, fs); err != nil {
		return err
	}

	var dataPools []*model.Pool
	for _, p := range fs.Spec.DataPools {
		dataPools = append(dataPools, p.ToModel(""))
	}
	f := mdsdaemon.NewFS(fs.Name, fs.Spec.MetadataPool.ToModel(""), dataPools, fs.Spec.MetadataServer.ActiveCount)
	if err := f.CreateFilesystem(context, fs.Namespace); err != nil {
		return fmt.Errorf("failed to create file system %s: %+v", fs.Name, err)
	}

	filesystem, err := client.GetFilesystem(context, fs.Namespace, fs.Name)
	if err != nil {
		return fmt.Errorf("failed to get file system %s: %+v", fs.Name, err)
	}

	logger.Infof("start running mdses for file system %s", fs.Name)
	c := newCluster(context, rookVersion, cephVersion, hostNetwork, fs, filesystem, ownerRefs)
	if err := c.start(); err != nil {
		return err
	}

	return nil
}

// deleteFileSystem deletes the file system and the metadata servers
func deleteFilesystem(context *clusterd.Context, fs cephv1beta1.Filesystem) error {
	// The most important part of deletion is that the filesystem gets removed from Ceph
	if err := mdsdaemon.DeleteFilesystem(context, fs.Namespace, fs.Name); err != nil {
		// If the fs isn't deleted from Ceph, leave the daemons so it can still be used.
		return fmt.Errorf("failed to delete filesystem %s: %+v", fs.Name, err)
	}

	return deleteMdsCluster(context, fs.Namespace, fs.Name)
}

func validateFilesystem(context *clusterd.Context, f cephv1beta1.Filesystem) error {
	if f.Name == "" {
		return fmt.Errorf("missing name")
	}
	if f.Namespace == "" {
		return fmt.Errorf("missing namespace")
	}
	if len(f.Spec.DataPools) == 0 {
		return fmt.Errorf("at least one data pool required")
	}
	if err := pool.ValidatePoolSpec(context, f.Namespace, &f.Spec.MetadataPool); err != nil {
		return fmt.Errorf("invalid metadata pool: %+v", err)
	}
	for _, p := range f.Spec.DataPools {
		if err := pool.ValidatePoolSpec(context, f.Namespace, &p); err != nil {
			return fmt.Errorf("Invalid data pool: %+v", err)
		}
	}
	if f.Spec.MetadataServer.ActiveCount < 1 {
		return fmt.Errorf("MetadataServer.ActiveCount must be at least 1")
	}

	return nil
}
