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
	"time"

	"github.com/rook/rook/pkg/clusterd"
	"github.com/rook/rook/pkg/daemon/ceph/client"
	"github.com/rook/rook/pkg/daemon/ceph/model"
)

const (
	dataPoolSuffix     = "data"
	metadataPoolSuffix = "metadata"
	appName            = "cephfs"
)

// Filesystem represents an instance of a Ceph file system (CephFS)
type Filesystem struct {
	Name           string
	metadataPool   *model.Pool
	dataPools      []*model.Pool
	activeMDSCount int32
}

// NewFS creates a new instance of the file (MDS) service
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

// CreateFilesystem starts the Ceph file daemons and creates the filesystem in Ceph.
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
		return fmt.Errorf("Unable to list existing file systems: %+v", err)
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
			return fmt.Errorf("failed to create data pool %s: %+v", pool.Name, err)
		}
		if pool.Type == model.ErasureCoded {
			// An erasure coded data pool used for a file system must allow overwrites
			if err := client.SetPoolProperty(context, clusterName, pool.Name, "allow_ec_overwrites", "true"); err != nil {
				logger.Warningf("failed to set ec pool property: %+v", err)
			}
		}
	}

	// create the filesystem
	if err := client.CreateFilesystem(context, clusterName, f.Name, f.metadataPool.Name, dataPoolNames, f.activeMDSCount); err != nil {
		return err
	}

	logger.Infof("created file system %s on %d data pool(s) and metadata pool %s", f.Name, len(f.dataPools), f.metadataPool.Name)
	return nil
}

// DeleteFilesystem removes the filesystem in Ceph and removes the Ceph file daemons.
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

// PrepareForDaemonUpgrade performs all actions necessary to ensure the filesystem is prepared
// to have its daemon(s) updated. This helps ensure there is no aberrant behavior during upgrades.
// If the mds is not prepared within the timeout window, an error will be reported.
// Ceph docs: http://docs.ceph.com/docs/master/cephfs/upgrading/
func PrepareForDaemonUpgrade(
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

	ready := make(chan struct{})
	quit := make(chan struct{})
	defer close(ready)
	defer close(quit)
	go func() {
		waitForNumMDSRanks(context, clusterName, fsName, 1, ready, quit)
	}()
	defer func() { quit <- struct{}{} }() // Don't leak resources; stop inf wait loop before exiting

	logger.Infof("waiting %f minutes for number of active daemons for fs %s to drop to 1", float64(timeout/time.Minute), fsName)
	t := time.NewTimer(timeout)
	defer t.Stop() // clean up timer resources if exit before timer fires
	select {
	case <-ready:
		logger.Infof("number of active daemons for fs %s is now 1", fsName) // nothing more to do
	case <-t.C: // time.After could be used here, but its resources live until timer fires
		return fmt.Errorf(
			"timeout waiting for filesystem %s number active mds daemons to drop to 1", fsName)
	}
	// * End of Noted section 1

	logger.Infof("Filesystem %s successfully prepared for mds daemon upgrade", fsName)
	return nil
}

// wait in inf loop for the number of mds ranks to equal the desired number
func waitForNumMDSRanks(context *clusterd.Context, clusterName, fsName string, numRanks int,
	ready chan<- struct{}, quit <-chan struct{},
) {
	t := time.NewTicker(3 * time.Second)
	defer t.Stop()
	for {
		select {
		case <-quit:
			logger.Debugf("Quitting infinit wait-for-mds-ranks loop")
			return
		case <-t.C:
			fs, err := client.GetFilesystem(context, clusterName, fsName)
			if err != nil {
				logger.Errorf(
					"Error getting filesystem %s details while waiting for num mds ranks to become %d: %+v",
					fsName, numRanks, err)
			} else if fs.MDSMap.MaxMDS == numRanks && len(fs.MDSMap.Up) == numRanks {
				// Both max_mds and number of up MDS daemons must equal desired number of ranks to
				// prevent a false positive when Ceph has got the correct number of mdses up but is
				// trying to change the number of mdses up to an undesired number.
				logger.Debugf("mds ranks for filesystem %s successfully became %d", fsName, numRanks)
				ready <- struct{}{}
				// continue to inf loop after send ready; only return when get quit signal to
				// prevent deadlock
			}
		}
	}
}

// FinishedWithDaemonUpgrade performs all actions necessary to bring the filesystem back to its
// ideal state following an upgrade of its daemon(s).
func FinishedWithDaemonUpgrade(
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
