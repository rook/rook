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

package client

import (
	"encoding/json"
	"fmt"
	"os"
	"strconv"
	"time"

	"github.com/rook/rook/pkg/clusterd"
	cephver "github.com/rook/rook/pkg/operator/ceph/version"
	"k8s.io/apimachinery/pkg/util/wait"
)

const (
	// MultiFsEnv defines the name of the Rook environment variable which controls if Rook is
	// allowed to create multiple Ceph filesystems.
	MultiFsEnv = "ROOK_ALLOW_MULTIPLE_FILESYSTEMS"
)

// CephFilesystem is a representation of the json structure returned by 'ceph fs ls'
type CephFilesystem struct {
	Name           string   `json:"name"`
	MetadataPool   string   `json:"metadata_pool"`
	MetadataPoolID int      `json:"metadata_pool_id"`
	DataPools      []string `json:"data_pools"`
	DataPoolIDs    []int    `json:"data_pool_ids"`
}

// CephFilesystemDetails is a representation of the main json structure returned by 'ceph fs get'
type CephFilesystemDetails struct {
	ID     int    `json:"id"`
	MDSMap MDSMap `json:"mdsmap"`
}

// MDSMap is a representation of the mds map sub-structure returned by 'ceph fs get'
type MDSMap struct {
	FilesystemName string             `json:"fs_name"`
	Enabled        bool               `json:"enabled"`
	Root           int                `json:"root"`
	TableServer    int                `json:"tableserver"`
	MaxMDS         int                `json:"max_mds"`
	In             []int              `json:"in"`
	Up             map[string]int     `json:"up"`
	MetadataPool   int                `json:"metadata_pool"`
	DataPools      []int              `json:"data_pools"`
	Failed         []int              `json:"failed"`
	Damaged        []int              `json:"damaged"`
	Stopped        []int              `json:"stopped"`
	Info           map[string]MDSInfo `json:"info"`
}

// MDSInfo is a representation of the individual mds daemon sub-sub-structure returned by 'ceph fs get'
type MDSInfo struct {
	GID     int    `json:"gid"`
	Name    string `json:"name"`
	Rank    int    `json:"rank"`
	State   string `json:"state"`
	Address string `json:"addr"`
}

// ListFilesystems lists all filesystems provided by the Ceph cluster.
func ListFilesystems(context *clusterd.Context, clusterName string) ([]CephFilesystem, error) {
	args := []string{"fs", "ls"}
	buf, err := ExecuteCephCommand(context, clusterName, args)
	if err != nil {
		return nil, fmt.Errorf("failed to list filesystems: %+v", err)
	}

	var filesystems []CephFilesystem
	err = json.Unmarshal(buf, &filesystems)
	if err != nil {
		return nil, fmt.Errorf("unmarshal failed: %+v.  raw buffer response: %s", err, string(buf))
	}

	return filesystems, nil
}

// GetFilesystem gets detailed status information about a Ceph filesystem.
func GetFilesystem(context *clusterd.Context, clusterName string, fsName string) (*CephFilesystemDetails, error) {
	args := []string{"fs", "get", fsName}
	buf, err := ExecuteCephCommand(context, clusterName, args)
	if err != nil {
		return nil, fmt.Errorf("failed to get file system %s: %+v", fsName, err)
	}

	var fs CephFilesystemDetails
	err = json.Unmarshal(buf, &fs)
	if err != nil {
		return nil, fmt.Errorf("unmarshal failed: %+v.  raw buffer response: %s", err, string(buf))
	}

	return &fs, nil
}

// AllowStandbyReplay gets detailed status information about a Ceph filesystem.
func AllowStandbyReplay(context *clusterd.Context, clusterName string, fsName string, allowStandbyReplay bool) error {
	args := []string{"fs", "set", fsName, "allow_standby_replay", strconv.FormatBool(allowStandbyReplay)}
	_, err := ExecuteCephCommand(context, clusterName, args)
	if err != nil {
		return fmt.Errorf("failed to set allow_standby_replay to filesystem %s: %+v", fsName, err)
	}

	return nil
}

// CreateFilesystem performs software configuration steps for Ceph to provide a new filesystem.
func CreateFilesystem(context *clusterd.Context, clusterName, name, metadataPool string, dataPools []string) error {
	if len(dataPools) == 0 {
		return fmt.Errorf("at least one data pool is required")
	}

	args := []string{}
	var err error

	if IsMultiFSEnabled() {
		// enable multiple file systems in case this is not the first
		args = []string{"fs", "flag", "set", "enable_multiple", "true", confirmFlag}
		_, err = ExecuteCephCommand(context, clusterName, args)
		if err != nil {
			// continue if this fails
			logger.Warning("failed enabling multiple file systems. %+v", err)
		}
	}

	// create the filesystem
	args = []string{"fs", "new", name, metadataPool, dataPools[0]}
	_, err = ExecuteCephCommand(context, clusterName, args)
	if err != nil {
		return fmt.Errorf("failed enabling ceph fs %s. %+v", name, err)
	}

	// add each additional pool
	for i := 1; i < len(dataPools); i++ {
		poolName := dataPools[i]
		args = []string{"fs", "add_data_pool", name, poolName}
		_, err = ExecuteCephCommand(context, clusterName, args)
		if err != nil {
			logger.Errorf("failed to add pool %s to file system %s. %+v", poolName, name, err)
		}
	}

	return nil
}

// IsMultiFSEnabled returns true if ROOK_ALLOW_MULTIPLE_FILESYSTEMS is set to "true", allowing
// Rook to create multiple Ceph filesystems. False if Rook is not allowed to do so.
func IsMultiFSEnabled() bool {
	t := os.Getenv(MultiFsEnv)
	if t == "true" {
		return true
	}
	return false
}

// SetNumMDSRanks sets the number of mds ranks (max_mds) for a Ceph filesystem.
func SetNumMDSRanks(context *clusterd.Context, cephVersion cephver.CephVersion, clusterName, fsName string, activeMDSCount int32) error {
	// Noted sections 1 and 2 are necessary for reducing max_mds in Luminous.
	//   See more:   [1] http://docs.ceph.com/docs/luminous/cephfs/upgrading/
	//               [2] https://tracker.ceph.com/issues/23172

	// * Noted section 1 - See note at top of function
	fsAtStart, errAtStart := GetFilesystem(context, clusterName, fsName)
	// collect information now, but don't check error yet
	// * End of Noted section 1

	// Always tell Ceph to set the new max_mds value
	args := []string{"fs", "set", fsName, "max_mds", strconv.Itoa(int(activeMDSCount))}
	if _, err := ExecuteCephCommand(context, clusterName, args); err != nil {
		return fmt.Errorf("failed to set filesystem %s num mds ranks (max_mds) to %d: %v",
			fsName, activeMDSCount, err)
	}

	if cephVersion.IsAtLeastMimic() {
		return nil
	}

	// ** Noted section 2 - See note at top of function
	// Now check the error to see if we can even determine whether we should reduce or not
	if errAtStart != nil {
		return fmt.Errorf(`failed to get filesystem %s info needed to ensure mds rank can be changed correctly,
if Ceph version is Luminous (12.y.z) and num active mdses (max_mds) was lowered, USER should deactivate extra active mdses manually: %v`,
			fsName, errAtStart)
	}
	if int(activeMDSCount) > fsAtStart.MDSMap.MaxMDS {
		return nil // No need to deactivate mdses if we are raising max_mds
	}
	logger.Debugf("deactivating some running mdses for filesystem %s", fsName)
	// Deactivate all mdses except desired number (N); arbitrarily choose first N to live
	fs, err := GetFilesystem(context, clusterName, fsName)
	if err != nil {
		logger.Warningf(
			fmt.Sprintf("Failed to get filesystem %s info needed to deactivate running mdses. ", fsName) +
				"using slightly stale info, this could (rarely) result in momentary loss of filesystem availability" +
				fmt.Sprintf(": %v", err),
		)
		fs = fsAtStart // <-- Do the best we can to disable mdses that were active when we started
		// Effects of stale info should be non-destructive, & unlikely the info is actually bad
	}
	// Deactivate any mdses with a higher rank than the desired max rank
	// Ceph only allows mdses to be deactivated in reverse order starting with the highest rank
	for gid := int(len(fs.MDSMap.In)) - 1; gid >= int(activeMDSCount); gid-- {
		if err := deactivateMdsWithRetry(context, gid, clusterName, fsName); err != nil {
			logger.Warningf("in luminous this is non-ideal but not necessarily critical: %v", err)
		}
	}
	// ** End of noted section 2

	return nil
}

func deactivateMdsWithRetry(context *clusterd.Context, mdsGid int, namespace, fsName string) error {
	retries := 10
	retrySleep := 5 * time.Second
	var err error
	for i := 1; i <= retries; i++ {
		args := []string{"mds", "deactivate", fmt.Sprintf("%s:%d", fsName, mdsGid)}
		if _, err = ExecuteCephCommand(context, namespace, args); err == nil {
			logger.Infof("successfully disabled mds with rank %d on attempt %d", mdsGid, i)
			return nil
		}
		time.Sleep(retrySleep)
	}
	// report most recent error with additional err info
	return fmt.Errorf("failed to deactivate mds w/ gid %d for filesystem %s: %+v", mdsGid, fsName, err)
}

// WaitForActiveRanks waits for the filesystem's number of active ranks to equal the desired count.
// It times out with an error if the number of active ranks does not become desired in time.
// Param 'moreIsOkay' will allow success condition if num of ranks is more than active count given.
func WaitForActiveRanks(
	context *clusterd.Context,
	clusterName, fsName string,
	desiredActiveRanks int32, moreIsOkay bool,
	timeout time.Duration,
) error {
	countText := fmt.Sprintf("%d", desiredActiveRanks)
	if moreIsOkay {
		// If it's okay to have more active ranks than desired, indicate so in log messages
		countText = fmt.Sprintf("%d or more", desiredActiveRanks)
	}
	logger.Infof("waiting %.2f second(s) for number of active mds daemons for fs %s to become %s",
		float64(timeout/time.Second), fsName, countText)
	err := wait.Poll(3*time.Second, timeout, func() (bool, error) {
		fs, err := GetFilesystem(context, clusterName, fsName)
		if err != nil {
			logger.Errorf(
				"Error getting filesystem %s details while waiting for num mds ranks to become %d: %+v",
				fsName, desiredActiveRanks, err)
		} else if fs.MDSMap.MaxMDS == int(desiredActiveRanks) &&
			activeRanksSuccess(len(fs.MDSMap.Up), int(desiredActiveRanks), moreIsOkay) {
			// Both max_mds and number of up MDS daemons must equal desired number of ranks to
			// prevent a false positive when Ceph has got the correct number of mdses up but is
			// trying to change the number of mdses up to an undesired number.
			logger.Debugf("mds ranks for filesystem %s successfully became %d", fsName, desiredActiveRanks)
			return true, nil
			// continue to inf loop after send ready; only return when get quit signal to
			// prevent deadlock
		}
		return false, nil
	})
	if err != nil {
		return fmt.Errorf("timeout waiting for number active mds daemons for filesystem %s to become %s",
			fsName, countText)
	}
	return nil
}

func activeRanksSuccess(upCount, desiredRanks int, moreIsOkay bool) bool {
	if moreIsOkay {
		return upCount >= desiredRanks
	}
	return upCount == desiredRanks
}

// MarkFilesystemAsDown marks a Ceph filesystem as down.
func MarkFilesystemAsDown(context *clusterd.Context, clusterName string, fsName string) error {
	args := []string{"fs", "set", fsName, "cluster_down", "true"}
	_, err := ExecuteCephCommand(context, clusterName, args)
	if err != nil {
		return fmt.Errorf("failed to set file system %s to cluster_down: %+v", fsName, err)
	}
	return nil
}

// FailMDS instructs Ceph to fail an mds daemon.
func FailMDS(context *clusterd.Context, clusterName string, gid int) error {
	args := []string{"mds", "fail", strconv.Itoa(gid)}
	_, err := ExecuteCephCommand(context, clusterName, args)
	if err != nil {
		return fmt.Errorf("failed to fail mds %d: %+v", gid, err)
	}
	return nil
}

// FailFilesystem efficiently brings down the filesystem by marking the filesystem as down
// and failing the MDSes using a single Ceph command. This works only from nautilus version
// of Ceph onwards.
func FailFilesystem(context *clusterd.Context, clusterName, fsName string) error {
	args := []string{"fs", "fail", fsName}
	_, err := ExecuteCephCommand(context, clusterName, args)
	if err != nil {
		return fmt.Errorf("failed to fail filesystem %s: %+v", fsName, err)
	}
	return nil
}

// RemoveFilesystem performs software configuration steps to remove a Ceph filesystem and its
// backing pools.
func RemoveFilesystem(context *clusterd.Context, clusterName, fsName string) error {
	fs, err := GetFilesystem(context, clusterName, fsName)
	if err != nil {
		return fmt.Errorf("filesystem %s not found. %+v", fsName, err)
	}

	args := []string{"fs", "rm", fsName, confirmFlag}
	_, err = ExecuteCephCommand(context, clusterName, args)
	if err != nil {
		return fmt.Errorf("Failed to delete ceph fs %s. err=%+v", fsName, err)
	}

	err = deleteFSPools(context, clusterName, fs)
	if err != nil {
		return fmt.Errorf("failed to delete fs %s pools. %+v", fsName, err)
	}
	return nil
}

func deleteFSPools(context *clusterd.Context, clusterName string, fs *CephFilesystemDetails) error {
	poolNames, err := GetPoolNamesByID(context, clusterName)
	if err != nil {
		return fmt.Errorf("failed to get pool names. %+v", err)
	}

	// delete the metadata pool
	var lastErr error
	if err := deleteFSPool(context, clusterName, poolNames, fs.MDSMap.MetadataPool); err != nil {
		lastErr = err
	}

	// delete the data pools
	for _, poolID := range fs.MDSMap.DataPools {
		if err := deleteFSPool(context, clusterName, poolNames, poolID); err != nil {
			lastErr = err
		}
	}
	return lastErr
}

func deleteFSPool(context *clusterd.Context, clusterName string, poolNames map[int]string, id int) error {
	name, ok := poolNames[id]
	if !ok {
		return fmt.Errorf("pool %d not found", id)
	}
	return DeletePool(context, clusterName, name)
}
