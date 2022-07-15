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

	"github.com/rook/rook/pkg/operator/k8sutil"

	"github.com/pkg/errors"
	cephv1 "github.com/rook/rook/pkg/apis/ceph.rook.io/v1"
	"github.com/rook/rook/pkg/clusterd"
	cephclient "github.com/rook/rook/pkg/daemon/ceph/client"
	"github.com/rook/rook/pkg/operator/ceph/file/mds"
	cephpool "github.com/rook/rook/pkg/operator/ceph/pool"
)

const (
	dataPoolSuffix     = "data"
	metaDataPoolSuffix = "metadata"
)

// Filesystem represents an instance of a Ceph filesystem (CephFS)
type Filesystem struct {
	Name      string
	Namespace string
}

// createFilesystem creates a Ceph filesystem with metadata servers
func createFilesystem(
	context *clusterd.Context,
	clusterInfo *cephclient.ClusterInfo,
	fs cephv1.CephFilesystem,
	clusterSpec *cephv1.ClusterSpec,
	ownerInfo *k8sutil.OwnerInfo,
	dataDirHostPath string,
) error {
	logger.Infof("start running mdses for filesystem %q", fs.Name)
	c := mds.NewCluster(clusterInfo, context, clusterSpec, fs, ownerInfo, dataDirHostPath)
	if err := c.Start(); err != nil {
		return err
	}

	if len(fs.Spec.DataPools) != 0 {
		f := newFS(fs.Name, fs.Namespace)
		if err := f.doFilesystemCreate(context, clusterInfo, clusterSpec, fs.Spec); err != nil {
			return errors.Wrapf(err, "failed to create filesystem %q", fs.Name)
		}
	}
	if fs.Spec.MetadataServer.ActiveStandby {
		if err := cephclient.AllowStandbyReplay(context, clusterInfo, fs.Name, fs.Spec.MetadataServer.ActiveStandby); err != nil {
			return errors.Wrapf(err, "failed to set allow_standby_replay to filesystem %q", fs.Name)
		}
	}

	// set the number of active mds instances
	if fs.Spec.MetadataServer.ActiveCount > 1 {
		if err := cephclient.SetNumMDSRanks(context, clusterInfo, fs.Name, fs.Spec.MetadataServer.ActiveCount); err != nil {
			logger.Warningf("failed setting active mds count to %d. %v", fs.Spec.MetadataServer.ActiveCount, err)
		}
	}

	return nil
}

// deleteFilesystem deletes the filesystem from Ceph
func deleteFilesystem(
	context *clusterd.Context,
	clusterInfo *cephclient.ClusterInfo,
	fs cephv1.CephFilesystem,
	clusterSpec *cephv1.ClusterSpec,
	ownerInfo *k8sutil.OwnerInfo,
	dataDirHostPath string,
) error {
	c := mds.NewCluster(clusterInfo, context, clusterSpec, fs, ownerInfo, dataDirHostPath)

	// Delete mds CephX keys and configuration in centralized mon database
	replicas := fs.Spec.MetadataServer.ActiveCount * 2
	for i := 0; i < int(replicas); i++ {
		daemonLetterID := k8sutil.IndexToName(i)
		daemonName := fmt.Sprintf("%s-%s", fs.Name, daemonLetterID)

		err := c.DeleteMdsCephObjects(daemonName)
		if err != nil {
			return errors.Wrapf(err, "failed to delete mds ceph objects for filesystem %q", fs.Name)
		}
	}

	// The most important part of deletion is that the filesystem gets removed from Ceph
	// The K8s resources will already be removed with the K8s owner references
	if err := downFilesystem(context, clusterInfo, fs.Name); err != nil {
		// If the fs isn't deleted from Ceph, leave the daemons so it can still be used.
		// Log the error for best effort and continue
		logger.Warningf("continuing to remove filesystem CR even though downing the filesystem failed. %v", err)
	}

	// TODO: should we move the `RemoveFilesystem()` call to be before removing MDSes? If the below
	// fails because the FS isn't empty, is it better to leave the filesystem active in case admins
	// want to recover data from it?
	//
	// Additionally, if PreserveFilesystemOnDelete is set, we won't have Ceph's safety net to do one
	// last check to see if the FS is in use before we delete it.

	// Permanently remove the filesystem if it was created by rook and the spec does not prevent it.
	if len(fs.Spec.DataPools) != 0 && !fs.Spec.PreserveFilesystemOnDelete {
		if err := cephclient.RemoveFilesystem(context, clusterInfo, fs.Name, fs.Spec.PreservePoolsOnDelete); err != nil {
			// log the error for best effort and continue
			logger.Warningf("continuing to remove filesystem CR even though removal failed. %v", err)
		}
	}
	return nil
}

func validateFilesystem(context *clusterd.Context, clusterInfo *cephclient.ClusterInfo, clusterSpec *cephv1.ClusterSpec, f *cephv1.CephFilesystem) error {
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
	if err := cephpool.ValidatePoolSpec(context, clusterInfo, clusterSpec, &f.Spec.MetadataPool); err != nil {
		return errors.Wrap(err, "invalid metadata pool")
	}
	for _, p := range f.Spec.DataPools {
		localpoolSpec := p.PoolSpec
		if err := cephpool.ValidatePoolSpec(context, clusterInfo, clusterSpec, &localpoolSpec); err != nil {
			return errors.Wrap(err, "Invalid data pool")
		}
	}

	return nil
}

// newFS creates a new instance of the file (MDS) service
func newFS(name, namespace string) *Filesystem {
	return &Filesystem{
		Name:      name,
		Namespace: namespace,
	}
}

// createOrUpdatePools function sets the sizes for MetadataPool and dataPool
func createOrUpdatePools(f *Filesystem, context *clusterd.Context, clusterInfo *cephclient.ClusterInfo, clusterSpec *cephv1.ClusterSpec, spec cephv1.FilesystemSpec) error {
	// generating the metadata pool's name
	metadataPool := cephv1.NamedPoolSpec{
		Name:     generateMetaDataPoolName(f),
		PoolSpec: spec.MetadataPool,
	}
	err := cephclient.CreatePool(context, clusterInfo, clusterSpec, metadataPool, "")
	if err != nil {
		return errors.Wrapf(err, "failed to update metadata pool %q", metadataPool.Name)
	}
	// generating the data pool's name
	dataPoolNames := generateDataPoolNames(f, spec)
	for i, dataPool := range spec.DataPools {
		dataPool.Name = dataPoolNames[i]
		err := cephclient.CreatePool(context, clusterInfo, clusterSpec, dataPool, "")
		if err != nil {
			return errors.Wrapf(err, "failed to update datapool  %q", dataPool.Name)
		}
	}
	return nil
}

// updateFilesystem ensures that a filesystem which already exists matches the provided spec.
func (f *Filesystem) updateFilesystem(context *clusterd.Context, clusterInfo *cephclient.ClusterInfo, clusterSpec *cephv1.ClusterSpec, spec cephv1.FilesystemSpec) error {
	// Even if the fs already exists, the num active mdses may have changed
	if err := cephclient.SetNumMDSRanks(context, clusterInfo, f.Name, spec.MetadataServer.ActiveCount); err != nil {
		logger.Errorf(
			fmt.Sprintf("failed to set num mds ranks (max_mds) to %d for filesystem %s, still continuing. ", spec.MetadataServer.ActiveCount, f.Name) +
				"this error is not critical, but mdses may not be as failure tolerant as desired. " +
				fmt.Sprintf("USER should verify that the number of active mdses is %d with 'ceph fs get %s'", spec.MetadataServer.ActiveCount, f.Name) +
				fmt.Sprintf(". %v", err),
		)
	}

	if err := createOrUpdatePools(f, context, clusterInfo, clusterSpec, spec); err != nil {
		return errors.Wrap(err, "failed to set pools size")
	}

	dataPoolNames := generateDataPoolNames(f, spec)
	for i := range spec.DataPools {
		if err := cephclient.AddDataPoolToFilesystem(context, clusterInfo, f.Name, dataPoolNames[i]); err != nil {
			return err
		}
	}
	return nil
}

// doFilesystemCreate starts the Ceph file daemons and creates the filesystem in Ceph.
func (f *Filesystem) doFilesystemCreate(context *clusterd.Context, clusterInfo *cephclient.ClusterInfo, clusterSpec *cephv1.ClusterSpec, spec cephv1.FilesystemSpec) error {

	_, err := cephclient.GetFilesystem(context, clusterInfo, f.Name)
	if err == nil {
		logger.Infof("filesystem %q already exists", f.Name)
		return f.updateFilesystem(context, clusterInfo, clusterSpec, spec)
	}
	if len(spec.DataPools) == 0 {
		return errors.New("at least one data pool must be specified")
	}

	poolNames, err := cephclient.GetPoolNamesByID(context, clusterInfo)
	if err != nil {
		return errors.Wrap(err, "failed to get pool names")
	}

	logger.Infof("creating filesystem %q", f.Name)

	// Make easy to locate a pool by name and avoid repeated searches
	reversedPoolMap := make(map[string]int)
	for key, value := range poolNames {
		reversedPoolMap[value] = key
	}

	metadataPool := cephv1.NamedPoolSpec{
		Name:     generateMetaDataPoolName(f),
		PoolSpec: spec.MetadataPool,
	}
	if _, poolFound := reversedPoolMap[metadataPool.Name]; !poolFound {
		err = cephclient.CreatePool(context, clusterInfo, clusterSpec, metadataPool, "")
		if err != nil {
			return errors.Wrapf(err, "failed to create metadata pool %q", metadataPool.Name)
		}
	}

	dataPoolNames := generateDataPoolNames(f, spec)
	for i, dataPool := range spec.DataPools {
		dataPool.Name = dataPoolNames[i]
		if _, poolFound := reversedPoolMap[dataPool.Name]; !poolFound {
			err = cephclient.CreatePool(context, clusterInfo, clusterSpec, dataPool, "")
			if err != nil {
				return errors.Wrapf(err, "failed to create data pool %q", dataPool.Name)
			}
			if dataPool.IsErasureCoded() {
				// An erasure coded data pool used for a filesystem must allow overwrites
				if err := cephclient.SetPoolProperty(context, clusterInfo, dataPool.Name, "allow_ec_overwrites", "true"); err != nil {
					logger.Warningf("failed to set ec pool property. %v", err)
				}
			}
		}
	}

	// create the filesystem ('fs new' needs to be forced in order to reuse preexisting pools)
	// if only one pool is created new it won't work (to avoid inconsistencies).
	if err := cephclient.CreateFilesystem(context, clusterInfo, f.Name, metadataPool.Name, dataPoolNames); err != nil {
		return err
	}

	logger.Infof("created filesystem %q on %d data pool(s) and metadata pool %q", f.Name, len(dataPoolNames), metadataPool.Name)
	return nil
}

// downFilesystem marks the filesystem as down and the MDS' as failed
func downFilesystem(context *clusterd.Context, clusterInfo *cephclient.ClusterInfo, filesystemName string) error {
	logger.Infof("downing filesystem %q", filesystemName)

	if err := cephclient.FailFilesystem(context, clusterInfo, filesystemName); err != nil {
		return err
	}
	logger.Infof("downed filesystem %q", filesystemName)
	return nil
}

// generateDataPoolName generates DataPool name by prefixing the filesystem name to the constant DataPoolSuffix
// or get predefined name from spec
func generateDataPoolNames(f *Filesystem, spec cephv1.FilesystemSpec) []string {
	var dataPoolNames []string
	for i, pool := range spec.DataPools {
		poolName := ""
		if pool.Name == "" {
			poolName = fmt.Sprintf("%s-%s%d", f.Name, dataPoolSuffix, i)
		} else {
			poolName = fmt.Sprintf("%s-%s", f.Name, pool.Name)
		}
		dataPoolNames = append(dataPoolNames, poolName)
	}
	return dataPoolNames
}

// generateMetaDataPoolName generates MetaDataPool name by prefixing the filesystem name to the constant metaDataPoolSuffix
func generateMetaDataPoolName(f *Filesystem) string {
	return fmt.Sprintf("%s-%s", f.Name, metaDataPoolSuffix)
}
