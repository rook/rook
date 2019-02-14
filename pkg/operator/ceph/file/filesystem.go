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
	"time"

	cephv1 "github.com/rook/rook/pkg/apis/ceph.rook.io/v1"
	"github.com/rook/rook/pkg/clusterd"
	"github.com/rook/rook/pkg/daemon/ceph/client"
	cephconfig "github.com/rook/rook/pkg/daemon/ceph/config"
	"github.com/rook/rook/pkg/daemon/ceph/model"
	"github.com/rook/rook/pkg/operator/ceph/pool"
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
		f := newFS(fs.Name, fs.Spec.MetadataPool.ToModel(""), dataPools, fs.Spec.MetadataServer.ActiveCount)
		if err := f.doFilesystemCreate(context, fs.Namespace); err != nil {
			return fmt.Errorf("failed to create filesystem %s: %+v", fs.Name, err)
		}
	}

	filesystem, err := client.GetFilesystem(context, fs.Namespace, fs.Name)
	if err != nil {
		return fmt.Errorf("failed to get filesystem %s: %+v", fs.Name, err)
	}

	// set the number of active mds instances
	if fs.Spec.MetadataServer.ActiveCount > 1 {
		if err = client.SetNumMDSRanks(context, fs.Namespace, fs.Name, fs.Spec.MetadataServer.ActiveCount); err != nil {
			logger.Warningf("failed setting active mds count to %d. %+v", fs.Spec.MetadataServer.ActiveCount, err)
		}
	}

	logger.Infof("start running mdses for filesystem %s", fs.Name)
	c := newCluster(clusterInfo, context, rookVersion, cephVersion, hostNetwork, fs, filesystem, ownerRefs)
	if err := c.start(); err != nil {
		return err
	}

	return nil
}

// deleteFileSystem deletes the filesystem and the metadata servers
func deleteFilesystem(context *clusterd.Context, fs cephv1.CephFilesystem) error {
	// The most important part of deletion is that the filesystem gets removed from Ceph
	if err := downFilesystem(context, fs.Namespace, fs.Name); err != nil {
		// If the fs isn't deleted from Ceph, leave the daemons so it can still be used.
		return fmt.Errorf("failed to down filesystem %s: %+v", fs.Name, err)
	}

	// Permanently remove the filesystem if it was created by rook
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

// newFS creates a new instance of the file (MDS) service
func newFS(name string, metadataPool *model.Pool, dataPools []*model.Pool, activeMDSCount int32) *Filesystem {

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

// doFilesystemCreate starts the Ceph file daemons and creates the filesystem in Ceph.
func (f *Filesystem) doFilesystemCreate(context *clusterd.Context, clusterName string) error {
	_, err := client.GetFilesystem(context, clusterName, f.Name)
	if err == nil {
		logger.Infof("filesystem %s already exists", f.Name)
		// Even if the fs already exists, the num active mdses may have changed
		if err := client.SetNumMDSRanks(context, clusterName, f.Name, f.activeMDSCount); err != nil {
			logger.Errorf(
				fmt.Sprintf("failed to set num mds ranks (max_mds) to %d for filesystem %s, still continuing. ", f.activeMDSCount, f.Name) +
					"this error is not critical, but mdses may not be as failure tolerant as desired. " +
					fmt.Sprintf("USER should verify that the number of active mdses is %d with 'ceph fs get %s'", f.activeMDSCount, f.Name) +
					fmt.Sprintf(": %+v", err),
			)
		}
		return nil
	}
	if len(f.dataPools) == 0 {
		return fmt.Errorf("at least one data pool must be specified")
	}

	fslist, err := client.ListFilesystems(context, clusterName)
	if err != nil {
		return fmt.Errorf("Unable to list existing filesystem: %+v", err)
	}
	if len(fslist) > 0 && !client.IsMultiFSEnabled() {
		return fmt.Errorf("Cannot create multiple filesystems. Enable %s env variable to create more than one", client.MultiFsEnv)
	}

	logger.Infof("Creating filesystem %s", f.Name)
	err = client.CreatePoolWithProfile(context, clusterName, *f.metadataPool, appName)
	if err != nil {
		return fmt.Errorf("failed to create metadata pool '%s': %+v", f.metadataPool.Name, err)
	}

	var dataPoolNames []string
	for _, pool := range f.dataPools {
		dataPoolNames = append(dataPoolNames, pool.Name)
		err = client.CreatePoolWithProfile(context, clusterName, *pool, appName)
		if err != nil {
			return fmt.Errorf("failed to create data pool %s: %+v", pool.Name, err)
		}
		if pool.Type == model.ErasureCoded {
			// An erasure coded data pool used for a filesystem must allow overwrites
			if err := client.SetPoolProperty(context, clusterName, pool.Name, "allow_ec_overwrites", "true"); err != nil {
				logger.Warningf("failed to set ec pool property: %+v", err)
			}
		}
	}

	// create the filesystem
	if err := client.CreateFilesystem(context, clusterName, f.Name, f.metadataPool.Name, dataPoolNames); err != nil {
		return err
	}

	logger.Infof("created filesystem%s on %d data pool(s) and metadata pool %s", f.Name, len(f.dataPools), f.metadataPool.Name)
	return nil
}

// downFilesystem marks the filesystem as down and the MDS' as failed
func downFilesystem(context *clusterd.Context, clusterName, filesystemName string) error {
	logger.Infof("Removing filesystem %s", filesystemName)

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

	logger.Infof("Removed filesystem %s", filesystemName)
	return nil
}

// prepareForDaemonUpgrade performs all actions necessary to ensure the filesystem is prepared
// to have its daemon(s) updated. This helps ensure there is no aberrant behavior during upgrades.
// If the mds is not prepared within the timeout window, an error will be reported.
// Ceph docs: http://docs.ceph.com/docs/master/cephfs/upgrading/
func prepareForDaemonUpgrade(
	context *clusterd.Context,
	clusterName, fsName string,
	timeout time.Duration,
) error {
	logger.Infof("preparing filesystem %s for daemon upgrade", fsName)
	// * Beginning of noted section 1
	// This section is necessary for upgrading to Mimic and to/past Luminous 12.2.3.
	//   See more:  https://ceph.com/releases/v13-2-0-mimic-released/
	//              http://docs.ceph.com/docs/mimic/cephfs/upgrading/
	// As of Oct. 2018, this is only necessary for Luminous and Mimic.
	if err := client.SetNumMDSRanks(context, clusterName, fsName, 1); err != nil {
		return fmt.Errorf("Could not Prepare filesystem %s for daemon upgrade: %+v", fsName, err)
	}
	if err := client.WaitForActiveRanks(context, clusterName, fsName, 1, false, timeout); err != nil {
		return err
	}
	// * End of Noted section 1

	logger.Infof("Filesystem %s successfully prepared for mds daemon upgrade", fsName)
	return nil
}

// finishedWithDaemonUpgrade performs all actions necessary to bring the filesystem back to its
// ideal state following an upgrade of its daemon(s).
func finishedWithDaemonUpgrade(
	context *clusterd.Context,
	clusterName, fsName string,
	activeMDSCount int32,
) error {
	logger.Debugf("restoring filesystem %s from daemon upgrade", fsName)
	logger.Debugf("bringing num active mds daemons for fs %s back to %d", fsName, activeMDSCount)
	// * Beginning of noted section 1
	// This section is necessary for upgrading to Mimic and to/past Luminous 12.2.3.
	//   See more:  https://ceph.com/releases/v13-2-0-mimic-released/
	//              http://docs.ceph.com/docs/mimic/cephfs/upgrading/
	// TODO: Unknown (Oct. 2018) if any parts can be removed once Rook no longer supports Mimic.
	if err := client.SetNumMDSRanks(context, clusterName, fsName, activeMDSCount); err != nil {
		return fmt.Errorf("Failed to restore filesystem %s following daemon upgrade: %+v", fsName, err)
	} // * End of noted section 1
	return nil
}
