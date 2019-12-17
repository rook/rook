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

	"github.com/pkg/errors"
	cephv1 "github.com/rook/rook/pkg/apis/ceph.rook.io/v1"
	"github.com/rook/rook/pkg/clusterd"
	"github.com/rook/rook/pkg/daemon/ceph/client"
	cephconfig "github.com/rook/rook/pkg/daemon/ceph/config"
	"github.com/rook/rook/pkg/daemon/ceph/model"
	"github.com/rook/rook/pkg/operator/ceph/file/mds"
	"github.com/rook/rook/pkg/operator/ceph/pool"
	cephver "github.com/rook/rook/pkg/operator/ceph/version"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	dataPoolSuffix     = "data"
	metadataPoolSuffix = "metadata"
	appName            = "cephfs"
)

// Filesystem represents an instance of a Ceph filesystem (CephFS)
type Filesystem struct {
	Name           string
	metadataPool   *model.Pool
	dataPools      []*model.Pool
	activeMDSCount int32
}

// createFilesystem creates a Ceph filesystem with metadata servers
func createFilesystem(
	clusterInfo *cephconfig.ClusterInfo,
	context *clusterd.Context,
	fs cephv1.CephFilesystem,
	rookVersion string,
	clusterSpec *cephv1.ClusterSpec,
	ownerRefs metav1.OwnerReference,
	dataDirHostPath string,
	isUpgrade bool,
) error {
	if err := validateFilesystem(context, fs); err != nil {
		return err
	}

	if len(fs.Spec.DataPools) != 0 {
		var dataPools []*model.Pool
		for _, p := range fs.Spec.DataPools {
			dataPools = append(dataPools, p.ToModel(""))
		}
		f := newFS(fs.Name, fs.Spec.MetadataPool.ToModel(""), dataPools, fs.Spec.MetadataServer.ActiveCount)
		if err := f.doFilesystemCreate(context, clusterInfo.CephVersion, fs.Namespace); err != nil {
			return errors.Wrapf(err, "failed to create filesystem %s", fs.Name)
		}
	}

	filesystem, err := client.GetFilesystem(context, fs.Namespace, fs.Name)
	if err != nil {
		return errors.Wrapf(err, "failed to get filesystem %s", fs.Name)
	}

	// As of Nautilus, allow_standby_replay is a fs property so we need to apply it
	if clusterInfo.CephVersion.IsAtLeastNautilus() {
		if fs.Spec.MetadataServer.ActiveStandby {
			if err = client.AllowStandbyReplay(context, fs.Namespace, fs.Name, fs.Spec.MetadataServer.ActiveStandby); err != nil {
				return errors.Wrapf(err, "failed to set allow_standby_replay to filesystem %s", fs.Name)
			}
		}
	}

	// set the number of active mds instances
	if fs.Spec.MetadataServer.ActiveCount > 1 {
		if err = client.SetNumMDSRanks(context, clusterInfo.CephVersion, fs.Namespace, fs.Name, fs.Spec.MetadataServer.ActiveCount); err != nil {
			logger.Warningf("failed setting active mds count to %d. %v", fs.Spec.MetadataServer.ActiveCount, err)
		}
	}

	logger.Infof("start running mdses for filesystem %s", fs.Name)
	c := mds.NewCluster(clusterInfo, context, rookVersion, clusterSpec, fs, filesystem, ownerRefs, dataDirHostPath, isUpgrade)
	if err := c.Start(); err != nil {
		return err
	}

	return nil
}

// deleteFileSystem deletes the filesystem from Ceph
func deleteFilesystem(context *clusterd.Context, cephVersion cephver.CephVersion, fs cephv1.CephFilesystem) error {
	// The most important part of deletion is that the filesystem gets removed from Ceph
	// The K8s resources will already be removed with the K8s owner references
	if err := downFilesystem(context, cephVersion, fs.Namespace, fs.Name); err != nil {
		// If the fs isn't deleted from Ceph, leave the daemons so it can still be used.
		return errors.Wrapf(err, "failed to down filesystem %q", fs.Name)
	}

	// Permanently remove the filesystem if it was created by rook
	if len(fs.Spec.DataPools) != 0 {
		if err := client.RemoveFilesystem(context, fs.Namespace, fs.Name, fs.Spec.PreservePoolsOnDelete); err != nil {
			return errors.Wrapf(err, "failed to remove filesystem %q", fs.Name)
		}
	}
	return nil
}

func validateFilesystem(context *clusterd.Context, f cephv1.CephFilesystem) error {
	if f.Name == "" {
		return errors.New("missing name")
	}
	if f.Namespace == "" {
		return errors.New("missing namespace")
	}
	if f.Spec.MetadataServer.ActiveCount < 1 {
		return errors.New("MetadataServer.ActiveCount must be at least 1")
	}
	// No data pool means that we expect the fs to exist already
	if len(f.Spec.DataPools) == 0 {
		return nil
	}
	if err := pool.ValidatePoolSpec(context, f.Namespace, &f.Spec.MetadataPool); err != nil {
		return errors.Wrapf(err, "invalid metadata pool")
	}
	for _, p := range f.Spec.DataPools {
		if err := pool.ValidatePoolSpec(context, f.Namespace, &p); err != nil {
			return errors.Wrapf(err, "Invalid data pool")
		}
	}

	return nil
}

// newFS creates a new instance of the file (MDS) service
func newFS(name string, metadataPool *model.Pool, dataPools []*model.Pool, activeMDSCount int32) *Filesystem {

	metadataPool.Name = fmt.Sprintf("%s-%s", name, metadataPoolSuffix)
	for i, pool := range dataPools {
		pool.Name = fmt.Sprintf("%s-%s%d", name, dataPoolSuffix, i)
	}

	// For the filesystem pool we don't want to enable the application pool
	// since it's being done via 'fs new' already
	metadataPool.NotEnableAppPool = true

	return &Filesystem{
		Name:           name,
		metadataPool:   metadataPool,
		dataPools:      dataPools,
		activeMDSCount: activeMDSCount,
	}
}

// doFilesystemCreate starts the Ceph file daemons and creates the filesystem in Ceph.
func (f *Filesystem) doFilesystemCreate(context *clusterd.Context, cephVersion cephver.CephVersion, clusterName string) error {
	_, err := client.GetFilesystem(context, clusterName, f.Name)
	if err == nil {
		logger.Infof("filesystem %s already exists", f.Name)
		// Even if the fs already exists, the num active mdses may have changed

		if err := client.SetNumMDSRanks(context, cephVersion, clusterName, f.Name, f.activeMDSCount); err != nil {
			logger.Errorf(
				fmt.Sprintf("failed to set num mds ranks (max_mds) to %d for filesystem %s, still continuing. ", f.activeMDSCount, f.Name) +
					"this error is not critical, but mdses may not be as failure tolerant as desired. " +
					fmt.Sprintf("USER should verify that the number of active mdses is %d with 'ceph fs get %s'", f.activeMDSCount, f.Name) +
					fmt.Sprintf(". %v", err),
			)
		}
		return nil
	}
	if len(f.dataPools) == 0 {
		return errors.New("at least one data pool must be specified")
	}

	fslist, err := client.ListFilesystems(context, clusterName)
	if err != nil {
		return errors.Wrapf(err, "Unable to list existing filesystem")
	}
	if len(fslist) > 0 && !client.IsMultiFSEnabled() {
		return errors.Errorf("cannot create multiple filesystems. enable %s env variable to create more than one", client.MultiFsEnv)
	}

	poolNames, err := client.GetPoolNamesByID(context, clusterName)
	if err != nil {
		return errors.Wrapf(err, "failed to get pool names")
	}

	logger.Infof("Creating filesystem %s", f.Name)

	// Make easy to locate a pool by name and avoid repeated searches
	reversedPoolMap := make(map[string]int)
	for key, value := range poolNames {
		reversedPoolMap[value] = key
	}

	poolsCreated := false
	if _, poolFound := reversedPoolMap[f.metadataPool.Name]; !poolFound {
		poolsCreated = true
		err = client.CreatePoolWithProfile(context, clusterName, *f.metadataPool, appName)
		if err != nil {
			return errors.Wrapf(err, "failed to create metadata pool %q", f.metadataPool.Name)
		}
	}

	var dataPoolNames []string
	for _, pool := range f.dataPools {
		dataPoolNames = append(dataPoolNames, pool.Name)
		if _, poolFound := reversedPoolMap[pool.Name]; !poolFound {
			poolsCreated = true
			err = client.CreatePoolWithProfile(context, clusterName, *pool, appName)
			if err != nil {
				return errors.Wrapf(err, "failed to create data pool %q", pool.Name)
			}
			if pool.Type == model.ErasureCoded {
				// An erasure coded data pool used for a filesystem must allow overwrites
				if err := client.SetPoolProperty(context, clusterName, pool.Name, "allow_ec_overwrites", "true"); err != nil {
					logger.Warningf("failed to set ec pool property. %v", err)
				}
			}
		}
	}

	// create the filesystem ('fs new' needs to be forced in order to reuse pre-existing pools)
	// if only one pool is created new it wont work (to avoid inconsistencies).
	if err := client.CreateFilesystem(context, clusterName, f.Name, f.metadataPool.Name, dataPoolNames, !poolsCreated); err != nil {
		return err
	}

	logger.Infof("created filesystem %s on %d data pool(s) and metadata pool %s", f.Name, len(f.dataPools), f.metadataPool.Name)
	return nil
}

// downFilesystem marks the filesystem as down and the MDS' as failed
func downFilesystem(context *clusterd.Context, cephVersion cephver.CephVersion, clusterName, filesystemName string) error {
	logger.Infof("Downing filesystem %s", filesystemName)

	// From Ceph nautilus onwards, a single Ceph command marks the filesystem as down and
	// MDSes as failed
	if cephVersion.IsAtLeastNautilus() {
		if err := client.FailFilesystem(context, clusterName, filesystemName); err != nil {
			return err
		}
		logger.Infof("Downed filesystem %s", filesystemName)
		return nil
	}

	// mark the cephFS instance as cluster_down before removing
	if err := client.MarkFilesystemAsDown(context, clusterName, filesystemName); err != nil {
		return err
	}

	// mark each MDS associated with the filesystem to "failed"
	fsDetails, err := client.GetFilesystem(context, clusterName, filesystemName)
	if err != nil {
		return err
	}
	for _, mdsInfo := range fsDetails.MDSMap.Info {
		if err := client.FailMDS(context, clusterName, mdsInfo.GID); err != nil {
			return err
		}
	}

	logger.Infof("Downed filesystem %s", filesystemName)
	return nil
}
