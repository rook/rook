/*
Copyright 2021 The Rook Authors. All rights reserved.

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
	"github.com/pkg/errors"
	"github.com/rook/rook/pkg/clusterd"
)

// CreateCephFSSubVolumeGroup create a CephFS subvolume group.
// volName is the name of the Ceph FS volume, the same as the CephFilesystem CR name.
func CreateCephFSSubVolumeGroup(context *clusterd.Context, clusterInfo *ClusterInfo, volName, groupName string) error {
	logger.Infof("creating cephfs %q subvolume group %q", volName, groupName)
	//  [--pool_layout <data_pool_name>] [--uid <uid>] [--gid <gid>] [--mode <octal_mode>]
	args := []string{"fs", "subvolumegroup", "create", volName, groupName}
	cmd := NewCephCommand(context, clusterInfo, args)
	cmd.JsonOutput = false
	output, err := cmd.Run()
	if err != nil {
		return errors.Wrapf(err, "failed to create subvolume group %q. %s", volName, output)
	}

	logger.Infof("successfully created cephfs %q subvolume group %q", volName, groupName)
	return nil
}

// DeleteCephFSSubVolumeGroup delete a CephFS subvolume group.
func DeleteCephFSSubVolumeGroup(context *clusterd.Context, clusterInfo *ClusterInfo, volName, groupName string) error {
	logger.Infof("deleting cephfs %q subvolume group %q", volName, groupName)
	//  --force?
	args := []string{"fs", "subvolumegroup", "rm", volName, groupName}
	cmd := NewCephCommand(context, clusterInfo, args)
	cmd.JsonOutput = false
	output, err := cmd.Run()
	if err != nil {
		logger.Debugf("failed to delete subvolume group %q. %s. %v", volName, output, err)
		// Intentionally don't wrap the error so the caller can inspect the return code
		return err
	}

	logger.Infof("successfully deleted cephfs %q subvolume group %q", volName, groupName)
	return nil
}
