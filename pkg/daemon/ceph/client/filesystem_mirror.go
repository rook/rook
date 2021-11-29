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
	"encoding/json"
	"fmt"
	"strings"
	"syscall"

	"github.com/pkg/errors"
	cephv1 "github.com/rook/rook/pkg/apis/ceph.rook.io/v1"
	"github.com/rook/rook/pkg/clusterd"
	"github.com/rook/rook/pkg/util/exec"
)

type BootstrapPeerToken struct {
	Token string `json:"token"`
}

// RemoveFilesystemMirrorPeer add a mirror peer in the cephfs-mirror configuration
func RemoveFilesystemMirrorPeer(context *clusterd.Context, clusterInfo *ClusterInfo, peerUUID string) error {
	logger.Infof("removing cephfs-mirror peer %q", peerUUID)

	// Build command
	args := []string{"fs", "snapshot", "mirror", "peer_remove", peerUUID}
	cmd := NewCephCommand(context, clusterInfo, args)

	// Run command
	output, err := cmd.Run()
	if err != nil {
		return errors.Wrapf(err, "failed to remove cephfs-mirror peer for filesystem %q. %s", peerUUID, output)
	}

	logger.Infof("successfully removed cephfs-mirror peer %q", peerUUID)
	return nil
}

// EnableFilesystemSnapshotMirror enables filesystem snapshot mirroring
func EnableFilesystemSnapshotMirror(context *clusterd.Context, clusterInfo *ClusterInfo, filesystem string) error {
	logger.Infof("enabling ceph filesystem snapshot mirror for filesystem %q", filesystem)

	// Build command
	args := []string{"fs", "snapshot", "mirror", "enable", filesystem}
	cmd := NewCephCommand(context, clusterInfo, args)

	// Run command
	output, err := cmd.Run()
	if err != nil {
		return errors.Wrapf(err, "failed to enable ceph filesystem snapshot mirror for filesystem %q. %s", filesystem, output)
	}

	logger.Infof("successfully enabled ceph filesystem snapshot mirror for filesystem %q", filesystem)
	return nil
}

// DisableFilesystemSnapshotMirror enables filesystem snapshot mirroring
func DisableFilesystemSnapshotMirror(context *clusterd.Context, clusterInfo *ClusterInfo, filesystem string) error {
	logger.Infof("disabling ceph filesystem snapshot mirror for filesystem %q", filesystem)

	// Build command
	args := []string{"fs", "snapshot", "mirror", "disable", filesystem}
	cmd := NewCephCommand(context, clusterInfo, args)

	// Run command
	output, err := cmd.Run()
	if err != nil {
		if code, err := exec.ExtractExitCode(err); err == nil && code == int(syscall.ENOTSUP) {
			logger.Debug("filesystem mirroring is not enabled, nothing to disable")
			return nil
		}
		return errors.Wrapf(err, "failed to disable ceph filesystem snapshot mirror for filesystem %q. %s", filesystem, output)
	}

	logger.Infof("successfully disabled ceph filesystem snapshot mirror for filesystem %q", filesystem)
	return nil
}

func AddSnapshotSchedule(context *clusterd.Context, clusterInfo *ClusterInfo, path, interval, startTime, filesystem string) error {
	logger.Infof("adding snapshot schedule every %q to ceph filesystem %q on path %q", interval, filesystem, path)

	args := []string{"fs", "snap-schedule", "add", path, interval}
	if startTime != "" {
		args = append(args, startTime)
	}
	args = append(args, fmt.Sprintf("fs=%s", filesystem))
	cmd := NewCephCommand(context, clusterInfo, args)
	cmd.JsonOutput = false
	// Example command: "ceph fs snap-schedule add / 4d fs=myfs2"

	// CHANGE time for "2014-01-09T21:48:00" IF interval
	// Run command
	output, err := cmd.Run()
	if err != nil {
		if code, ok := exec.ExitStatus(err); ok && code != int(syscall.EEXIST) {
			return errors.Wrapf(err, "failed to add snapshot schedule every %q to ceph filesystem %q on path %q. %s", interval, filesystem, path, output)
		}
	}

	logger.Infof("successfully added snapshot schedule every %q to ceph filesystem %q on path %q", interval, filesystem, path)
	return nil
}

func AddSnapshotScheduleRetention(context *clusterd.Context, clusterInfo *ClusterInfo, path, duration, filesystem string) error {
	logger.Infof("adding snapshot schedule retention %s to ceph filesystem %q on path %q", duration, filesystem, path)

	// Example command: "ceph fs snap-schedule retention add / d 1 fs=myfs2"
	args := []string{"fs", "snap-schedule", "retention", "add", path, duration, fmt.Sprintf("fs=%s", filesystem)}
	cmd := NewCephCommand(context, clusterInfo, args)
	cmd.JsonOutput = false

	// Run command
	output, err := cmd.Run()
	if err != nil {
		if code, ok := exec.ExitStatus(err); ok && code == int(syscall.ENOENT) {
			logger.Warningf("snapshot schedule retention %s already exists for filesystem %q on path %q. %s", duration, filesystem, path, output)
		} else {
			return errors.Wrapf(err, "failed to add snapshot schedule retention %s to ceph filesystem %q on path %q. %s", duration, filesystem, path, output)
		}
	}

	logger.Infof("successfully added snapshot schedule retention %s to ceph filesystem %q on path %q", duration, filesystem, path)
	return nil
}

func GetSnapshotScheduleStatus(context *clusterd.Context, clusterInfo *ClusterInfo, filesystem string) ([]cephv1.FilesystemSnapshotSchedulesSpec, error) {
	logger.Infof("retrieving snapshot schedule status for ceph filesystem %q", filesystem)

	args := []string{"fs", "snap-schedule", "status", "/", "recursive=true", fmt.Sprintf("--fs=%s", filesystem)}
	cmd := NewCephCommand(context, clusterInfo, args)

	// Run command
	output, err := cmd.Run()
	if err != nil {
		return nil, errors.Wrapf(err, "failed to retrieve snapshot schedule status for ceph filesystem %q. %s", filesystem, output)
	}

	// Unmarshal JSON into Go struct
	var filesystemSnapshotSchedulesStatusSpec []cephv1.FilesystemSnapshotSchedulesSpec

	/* Replace new line since the command outputs a new line first and breaks the json parsing...
	[root@rook-ceph-operator-75c6d6bbfc-wqlnc /]# ceph --connect-timeout=15 --cluster=rook-ceph --conf=/var/lib/rook/rook-ceph/rook-ceph.config --name=client.admin --keyring=/var/lib/rook/rook-ceph/client.admin.keyring --format json fs snap-schedule status /

	[{"fs": "myfs", "subvol": null, "path": "/", "rel_path": "/", "schedule": "24h", "retention": {"h": 24}, "start": "2021-07-01T00:00:00", "created": "2021-07-01T12:19:12", "first": null, "last": null, "last_pruned": null, "created_count": 0, "pruned_count": 0, "active": true},{"fs": "myfs", "subvol": null, "path": "/", "rel_path": "/", "schedule": "25h", "retention": {"h": 24}, "start": "2021-07-01T00:00:00", "created": "2021-07-01T12:31:25", "first": null, "last": null, "last_pruned": null, "created_count": 0, "pruned_count": 0, "active": true}]
	*/
	if err := json.Unmarshal([]byte(strings.ReplaceAll(string(output), "\n", "")), &filesystemSnapshotSchedulesStatusSpec); err != nil {
		return nil, errors.Wrap(err, "failed to unmarshal filesystem mirror snapshot schedule status response")
	}

	logger.Infof("successfully retrieved snapshot schedule status for ceph filesystem %q", filesystem)
	return filesystemSnapshotSchedulesStatusSpec, nil
}

// ImportFSMirrorBootstrapPeer add a mirror peer in the cephfs-mirror configuration
func ImportFSMirrorBootstrapPeer(context *clusterd.Context, clusterInfo *ClusterInfo, fsName, token string) error {
	logger.Infof("importing cephfs bootstrap peer token for filesystem %q", fsName)

	// Build command
	args := []string{"fs", "snapshot", "mirror", "peer_bootstrap", "import", fsName, strings.TrimSpace(token)}
	cmd := NewCephCommand(context, clusterInfo, args)
	cmd.JsonOutput = false
	cmd.combinedOutput = true

	// Run command
	output, err := cmd.Run()
	if err != nil {
		return errors.Wrapf(err, "failed to import cephfs-mirror peer token for filesystem %q. %s", fsName, output)
	}

	logger.Infof("successfully imported cephfs-mirror peer for filesystem %q", fsName)
	return nil
}

// CreateFSMirrorBootstrapPeer add a mirror peer in the cephfs-mirror configuration
func CreateFSMirrorBootstrapPeer(context *clusterd.Context, clusterInfo *ClusterInfo, fsName string) ([]byte, error) {
	logger.Infof("create cephfs-mirror bootstrap peer token for filesystem %q", fsName)

	// Build command
	args := []string{"fs", "snapshot", "mirror", "peer_bootstrap", "create", fsName, "client.mirror", clusterInfo.FSID}
	cmd := NewCephCommand(context, clusterInfo, args)

	// Run command
	output, err := cmd.Run()
	if err != nil {
		return nil, errors.Wrapf(err, "failed to create cephfs-mirror peer token for filesystem %q. %s", fsName, output)
	}

	// Unmarshal JSON into Go struct
	var bootstrapPeerToken BootstrapPeerToken
	if err := json.Unmarshal(output, &bootstrapPeerToken); err != nil {
		return nil, errors.Wrapf(err, "failed to unmarshal cephfs-mirror peer token create response. %s", output)
	}

	logger.Infof("successfully created cephfs-mirror bootstrap peer token for filesystem %q", fsName)
	return []byte(bootstrapPeerToken.Token), nil
}

// GetFSMirrorDaemonStatus returns the mirroring status of a given filesystem
func GetFSMirrorDaemonStatus(context *clusterd.Context, clusterInfo *ClusterInfo, fsName string) ([]cephv1.FilesystemMirroringInfo, error) {
	// Using Debug level since this is called in a recurrent go routine
	logger.Debugf("retrieving filesystem mirror status for filesystem %q", fsName)

	// Build command
	args := []string{"fs", "snapshot", "mirror", "daemon", "status", fsName}
	cmd := NewCephCommand(context, clusterInfo, args)

	// Run command
	output, err := cmd.Run()
	if err != nil {
		return nil, errors.Wrapf(err, "failed to retrieve filesystem mirror status for filesystem %q. %s", fsName, output)
	}

	// Unmarshal JSON into Go struct
	var filesystemMirroringInfo []cephv1.FilesystemMirroringInfo
	if err := json.Unmarshal([]byte(output), &filesystemMirroringInfo); err != nil {
		return nil, errors.Wrapf(err, "failed to unmarshal filesystem mirror status response. %q.", string(output))
	}

	logger.Debugf("successfully retrieved filesystem mirror status for filesystem %q", fsName)
	return filesystemMirroringInfo, nil
}
