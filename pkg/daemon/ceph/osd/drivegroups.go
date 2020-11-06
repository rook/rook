/*
Copyright 2020 The Rook Authors. All rights reserved.

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

package osd

import (
	"github.com/pkg/errors"
	"github.com/rook/rook/pkg/clusterd"
)

const (
	cvDriveGroupsCommand = "drive-group"
)

func (a *OsdAgent) configureDriveGroups(context *clusterd.Context) error {
	if len(a.driveGroups) == 0 {
		logger.Debug("No Drive Groups configured.")
		return nil
	}

	if !a.driveGroupsAreSupported(context) {
		return errors.New("failed to configure OSDs via Drive Groups. " +
			"Rook will fail OSD creation; update Ceph to a newer version or remove Drive Groups from the CephCluster resource to correct this. " +
			"the current version of ceph-volume does not support creating OSDs via Drive Groups; this is only available in later versions of Ceph Octopus (v15).")
	}

	// Create OSD bootstrap keyring
	err := createOSDBootstrapKeyring(context, a.clusterInfo, cephConfigDir)
	if err != nil {
		return errors.Wrapf(err, "failed to generate OSD bootstrap keyring")
	}

	for group, spec := range a.driveGroups {
		logger.Infof("configuring Drive Group %q: %+v", group, spec)
		_, err := callCephVolume(context, true, cvDriveGroupsCommand, "--spec", spec)
		if err != nil {
			return errors.Wrapf(err, "failed to configure Drive Group %q", group)
		}
	}
	return nil
}

func (a *OsdAgent) driveGroupsAreSupported(context *clusterd.Context) bool {
	// Call `ceph-volume drive-group` command with no args:
	// if supported, will return 0 (and help message which we ignore), else returns nonzero (error)
	_, err := callCephVolume(context, true, cvDriveGroupsCommand)
	return err == nil
}
