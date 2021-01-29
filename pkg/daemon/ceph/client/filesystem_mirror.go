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

// AddFilesystemMirrorPeer add a mirror peer in the cephfs-mirror configuration
func AddFilesystemMirrorPeer(context *clusterd.Context, clusterInfo *ClusterInfo, filesystem, peer, remoteFilesystem string) error {
	logger.Infof("adding cephfs-mirror peer for filesystem %q", filesystem)

	// Build command
	args := []string{"fs", "snapshot", "mirror", "peer_add", filesystem, peer, remoteFilesystem}
	cmd := NewCephCommand(context, clusterInfo, args)

	// Run command
	output, err := cmd.Run()
	if err != nil {
		return errors.Wrapf(err, "failed to add cephfs-mirror peer for filesystem %q. %s", filesystem, output)
	}

	return nil
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
		return errors.Wrapf(err, "failed to disable ceph filesystem snapshot mirror for filesystem %q. %s", filesystem, output)
	}

	return nil
}

// AuthorizeFilesystemMirror add a mirror peer in the rbd-mirror configuration
// Create a user for each file system peer (on the secondary/remote cluster).
// This user needs to have full capabilities on the MDS (to take snapshots) and the OSDs
func AuthorizeFilesystemMirror(context *clusterd.Context, clusterInfo *ClusterInfo, filesystem, client string) ([]byte, error) {
	logger.Infof("disabling ceph filesystem mirror for filesystem %q", filesystem)

	// Build command, do not get confused by the "/ rwps"
	// The "/" is the fs path
	// The "rwps" are the permissions:
	// 		r: read
	//		w: write
	//		p: layout or quota
	//		s: snapshots
	args := []string{"fs", "authorize", filesystem, client, "/", "rwps"}
	cmd := NewCephCommand(context, clusterInfo, args)

	// Run command
	output, err := cmd.Run()
	if err != nil {
		return nil, errors.Wrapf(err, "failed to authorize ceph client %q for filesystem %q. %s", client, filesystem, output)
	}

	return output, nil
}
