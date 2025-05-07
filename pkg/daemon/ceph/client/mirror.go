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

package client

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/pkg/errors"
	cephv1 "github.com/rook/rook/pkg/apis/ceph.rook.io/v1"
	"github.com/rook/rook/pkg/clusterd"
	"github.com/rook/rook/pkg/operator/ceph/version"
	cephver "github.com/rook/rook/pkg/operator/ceph/version"
	"github.com/rook/rook/pkg/util/exec"
	"k8s.io/apimachinery/pkg/util/sets"
)

// PeerToken is the content of the peer token
type PeerToken struct {
	ClusterFSID string `json:"fsid"`
	ClientID    string `json:"client_id"`
	Key         string `json:"key"`
	MonHost     string `json:"mon_host"`
	// These fields are added by Rook and NOT part of the output of client.CreateRBDMirrorBootstrapPeer()
	Namespace string `json:"namespace"`
}

type MirroredImages struct {
	// Images is the list of mirrored images on a pool
	Images *[]Images
}

type Images struct {
	// Name of the pool image
	Name string
}

const (
	mirrorModeDisabled = "disabled"
	mirrorModeInitOnly = "init-only"
)

var (
	rbdMirrorPeerCaps                     = []string{"mon", "profile rbd-mirror-peer", "osd", "profile rbd"}
	rbdMirrorPeerKeyringID                = "rbd-mirror-peer"
	radosNamespaceMirroringMinimumVersion = cephver.CephVersion{Major: 20, Minor: 0, Extra: 0}
)

// ImportRBDMirrorBootstrapPeer add a mirror peer in the rbd-mirror configuration
func ImportRBDMirrorBootstrapPeer(context *clusterd.Context, clusterInfo *ClusterInfo, poolName string, direction string, token []byte) error {
	logger.Infof("add rbd-mirror bootstrap peer token for pool %q", poolName)

	// Token file
	tokenFilePattern := fmt.Sprintf("rbd-mirror-token-%s", poolName)
	tokenFilePath, err := os.CreateTemp("/tmp", tokenFilePattern)
	if err != nil {
		return errors.Wrapf(err, "failed to create temporary token file for pool %q", poolName)
	}

	// Write token into a file
	err = os.WriteFile(tokenFilePath.Name(), token, 0o400)
	if err != nil {
		return errors.Wrapf(err, "failed to write token to file %q", tokenFilePath.Name())
	}

	// Remove token once we exit, we don't need it anymore
	defer func() error {
		err := os.Remove(tokenFilePath.Name())
		return err
	}() //nolint // we don't want to return here

	// Build command
	args := []string{"mirror", "pool", "peer", "bootstrap", "import", poolName, tokenFilePath.Name()}
	if direction != "" {
		args = append(args, "--direction", direction)
	}
	cmd := NewRBDCommand(context, clusterInfo, args)

	// Run command
	output, err := cmd.RunWithTimeout(exec.CephCommandsTimeout)
	if err != nil {
		return errors.Wrapf(err, "failed to add rbd-mirror peer token for pool %q. %s", poolName, output)
	}

	logger.Infof("successfully added rbd-mirror peer token for pool %q", poolName)
	return nil
}

// CreateRBDMirrorBootstrapPeer add a mirror peer in the rbd-mirror configuration
func CreateRBDMirrorBootstrapPeer(context *clusterd.Context, clusterInfo *ClusterInfo, poolName string) ([]byte, error) {
	logger.Infof("create rbd-mirror bootstrap peer token for pool %q", poolName)

	// Build command
	args := []string{"mirror", "pool", "peer", "bootstrap", "create", poolName}
	cmd := NewRBDCommand(context, clusterInfo, args)

	// Run command
	output, err := cmd.Run()
	if err != nil {
		return nil, errors.Wrapf(err, "failed to create rbd-mirror peer token for pool %q. %s", poolName, output)
	}

	logger.Infof("successfully created rbd-mirror bootstrap peer token for pool %q", poolName)
	return output, nil
}

// enablePoolMirroring turns on mirroring on that pool by specifying the mirroring type
func enablePoolMirroring(context *clusterd.Context, clusterInfo *ClusterInfo, pool cephv1.NamedPoolSpec) error {
	logger.Infof("enabling mirroring type %q for pool %q", pool.Mirroring.Mode, pool.Name)

	if pool.Mirroring.Mode == mirrorModeInitOnly && !clusterInfo.CephVersion.IsAtLeastTentacle() {
		return fmt.Errorf("ceph version %q does not support mirroring mode %s, minimum ceph version required is %q", clusterInfo.CephVersion.String(), pool.Mirroring.Mode, version.Tentacle.String())
	}

	mirrorInfo, err := GetPoolMirroringInfo(context, clusterInfo, pool.Name)
	if err != nil {
		return errors.Wrapf(err, "failed to get mirroring info for the pool %q", pool.Name)
	}

	if pool.Mirroring.Mode == mirrorInfo.Mode {
		logger.Debugf("mirroring is already enabled on the pool %s with mode %s", pool.Name, mirrorInfo.Mode)
		return nil
	}

	// Build command
	args := []string{"mirror", "pool", "enable", pool.Name, pool.Mirroring.Mode}
	cmd := NewRBDCommand(context, clusterInfo, args)

	// Run command
	output, err := cmd.Run()
	if err != nil {
		return errors.Wrapf(err, "failed to enable mirroring type %q for pool %q. %s", pool.Mirroring.Mode, pool.Name, output)
	}

	return nil
}

// disablePoolMirroring turns off mirroring on a pool
func DisablePoolMirroring(context *clusterd.Context, clusterInfo *ClusterInfo, poolName string) error {
	logger.Infof("disabling mirroring for pool %q", poolName)

	// Build command
	args := []string{"mirror", "pool", "disable", poolName}
	cmd := NewRBDCommand(context, clusterInfo, args)

	// Run command
	output, err := cmd.Run()
	if err != nil {
		return errors.Wrapf(err, "failed to disable mirroring for pool %q. %s", poolName, output)
	}

	return nil
}

func RemoveClusterPeer(context *clusterd.Context, clusterInfo *ClusterInfo, poolName, peerUUID string) error {
	logger.Infof("removing cluster peer with UUID %q for the pool %q", peerUUID, poolName)

	// Build command
	args := []string{"mirror", "pool", "peer", "remove", poolName, peerUUID}
	cmd := NewRBDCommand(context, clusterInfo, args)

	// Run command
	output, err := cmd.Run()
	if err != nil {
		return errors.Wrapf(err, "failed to remove cluster peer with UUID %q for the pool %q. %s", peerUUID, poolName, output)
	}

	return nil
}

// GetPoolMirroringStatus prints the pool mirroring status
func GetPoolMirroringStatus(context *clusterd.Context, clusterInfo *ClusterInfo, poolName string) (*cephv1.MirroringStatus, error) {
	logger.Debugf("retrieving mirroring pool %q status", poolName)

	// Build command
	args := []string{"mirror", "pool", "status", poolName}
	cmd := NewRBDCommand(context, clusterInfo, args)
	cmd.JsonOutput = true

	// Run command
	buf, err := cmd.Run()
	if err != nil {
		return nil, errors.Wrapf(err, "failed to retrieve mirroring pool %q status", poolName)
	}

	var poolMirroringStatus cephv1.MirroringStatus
	if err := json.Unmarshal([]byte(buf), &poolMirroringStatus); err != nil {
		return nil, errors.Wrap(err, "failed to unmarshal mirror pool status response")
	}

	return &poolMirroringStatus, nil
}

// GetMirroredPoolImages returns a list of mirrored images for a given pool
func GetMirroredPoolImages(context *clusterd.Context, clusterInfo *ClusterInfo, poolName string) (*MirroredImages, error) {
	logger.Debugf("retrieving mirrored images for pool %q", poolName)

	// Build command
	args := []string{"mirror", "pool", "status", "--verbose", poolName}
	cmd := NewRBDCommand(context, clusterInfo, args)
	cmd.JsonOutput = true

	// Run command
	buf, err := cmd.Run()
	if err != nil {
		return nil, errors.Wrapf(err, "failed to retrieve mirroring pool %q status", poolName)
	}

	var mirroredImages MirroredImages
	if err := json.Unmarshal([]byte(buf), &mirroredImages); err != nil {
		return nil, errors.Wrap(err, "failed to unmarshal mirror pool status response")
	}

	return &mirroredImages, nil
}

// GetPoolMirroringInfo  prints the pool mirroring information
// `poolName` is the name of the pool or the pool/radosNamespace
func GetPoolMirroringInfo(context *clusterd.Context, clusterInfo *ClusterInfo, poolName string) (*cephv1.MirroringInfo, error) {
	logger.Debugf("retrieving mirroring pool %q info", poolName)

	// Build command
	args := []string{"mirror", "pool", "info", poolName}
	cmd := NewRBDCommand(context, clusterInfo, args)
	cmd.JsonOutput = true

	// Run command
	buf, err := cmd.Run()
	if err != nil {
		return nil, errors.Wrapf(err, "failed to retrieve mirroring pool %q info. %s", poolName, string(buf))
	}

	// Unmarshal JSON into Go struct
	var poolMirroringInfo cephv1.MirroringInfo
	if err := json.Unmarshal(buf, &poolMirroringInfo); err != nil {
		return nil, errors.Wrap(err, "failed to unmarshal mirror pool info response")
	}

	return &poolMirroringInfo, nil
}

// enableSnapshotSchedule configures the snapshots schedule on a mirrored pool
func enableSnapshotSchedule(context *clusterd.Context, clusterInfo *ClusterInfo, snapSpec cephv1.SnapshotScheduleSpec, poolName string) error {
	logger.Infof("enabling snapshot schedule for pool %q", poolName)

	// Build command
	args := []string{"mirror", "snapshot", "schedule", "add", "--pool", poolName, snapSpec.Interval}

	// If a start time is defined let's add it
	if snapSpec.StartTime != "" {
		args = append(args, snapSpec.StartTime)
	}
	cmd := NewRBDCommand(context, clusterInfo, args)

	// Run command
	buf, err := cmd.Run()
	if err != nil {
		return errors.Wrapf(err, "failed to enable snapshot schedule on pool %q. %s", poolName, string(buf))
	}

	logger.Infof("successfully enabled snapshot schedule for pool %q every %q", poolName, snapSpec.Interval)
	return nil
}

// removeSnapshotSchedule removes the snapshots schedule on a mirrored pool
func removeSnapshotSchedule(context *clusterd.Context, clusterInfo *ClusterInfo, snapScheduleResponse cephv1.SnapshotSchedule, poolName string) error {
	logger.Debugf("removing snapshot schedule for pool %q (before adding new ones)", poolName)

	// Build command
	args := []string{"mirror", "snapshot", "schedule", "remove", "--pool", poolName, snapScheduleResponse.Interval}

	// If a start time is defined let's add it
	if snapScheduleResponse.StartTime != "" {
		args = append(args, snapScheduleResponse.StartTime)
	}
	cmd := NewRBDCommand(context, clusterInfo, args)

	// Run command
	buf, err := cmd.Run()
	if err != nil {
		return errors.Wrapf(err, "failed to remove snapshot schedule on pool %q. %s", poolName, string(buf))
	}

	logger.Infof("successfully removed snapshot schedule %q for pool %q", poolName, snapScheduleResponse.Interval)
	return nil
}

// `poolName` is the name of the pool or the pool/radosNamespace
func EnableSnapshotSchedules(context *clusterd.Context, clusterInfo *ClusterInfo, poolName string, snapshotSchedules []cephv1.SnapshotScheduleSpec) error {
	logger.Info("resetting current snapshot schedules in cluster namespace %q", clusterInfo.Namespace)
	// Reset any existing schedules
	err := removeSnapshotSchedules(context, clusterInfo, poolName)
	if err != nil {
		logger.Errorf("failed to remove snapshot schedules. %v", err)
	}

	// Enable all the snap schedules
	for _, snapSchedule := range snapshotSchedules {
		err := enableSnapshotSchedule(context, clusterInfo, snapSchedule, poolName)
		if err != nil {
			return errors.Wrap(err, "failed to enable snapshot schedule")
		}
	}

	return nil
}

// removeSnapshotSchedules removes all the existing snapshot schedules
func removeSnapshotSchedules(context *clusterd.Context, clusterInfo *ClusterInfo, pool string) error {
	// Get the list of existing snapshot schedule
	existingSnapshotSchedules, err := listSnapshotSchedules(context, clusterInfo, pool)
	if err != nil {
		return errors.Wrap(err, "failed to list snapshot schedule(s)")
	}

	// Remove each schedule
	for _, existingSnapshotSchedule := range existingSnapshotSchedules {
		err := removeSnapshotSchedule(context, clusterInfo, existingSnapshotSchedule, pool)
		if err != nil {
			return errors.Wrapf(err, "failed to remove snapshot schedule %v", existingSnapshotSchedule)
		}
	}

	return nil
}

// listSnapshotSchedules configures the snapshots schedule on a mirrored pool
func listSnapshotSchedules(context *clusterd.Context, clusterInfo *ClusterInfo, poolName string) ([]cephv1.SnapshotSchedule, error) {
	// Build command
	args := []string{"mirror", "snapshot", "schedule", "ls", "--pool", poolName}
	cmd := NewRBDCommand(context, clusterInfo, args)
	cmd.JsonOutput = true

	// Run command
	buf, err := cmd.Run()
	if err != nil {
		return nil, errors.Wrapf(err, "failed to retrieve snapshot schedules on pool %q. %s", poolName, string(buf))
	}

	// Unmarshal JSON into Go struct
	var snapshotSchedules []cephv1.SnapshotSchedule
	if err := json.Unmarshal([]byte(buf), &snapshotSchedules); err != nil {
		return nil, errors.Wrap(err, "failed to unmarshal mirror snapshot schedule list response")
	}

	logger.Debugf("successfully listed snapshot schedules for pool %q", poolName)
	return snapshotSchedules, nil
}

// ListSnapshotSchedulesRecursively configures the snapshots schedule on a mirrored pool
func ListSnapshotSchedulesRecursively(context *clusterd.Context, clusterInfo *ClusterInfo, poolName string) ([]cephv1.SnapshotSchedulesSpec, error) {
	// Build command
	args := []string{"mirror", "snapshot", "schedule", "ls", "--pool", poolName, "--recursive"}
	cmd := NewRBDCommand(context, clusterInfo, args)
	cmd.JsonOutput = true

	// Run command
	buf, err := cmd.Run()
	if err != nil {
		return nil, errors.Wrapf(err, "failed to retrieve snapshot schedules recursively on pool %q. %s", poolName, string(buf))
	}

	// Unmarshal JSON into Go struct
	var snapshotSchedulesRecursive []cephv1.SnapshotSchedulesSpec
	if err := json.Unmarshal([]byte(buf), &snapshotSchedulesRecursive); err != nil {
		return nil, errors.Wrap(err, "failed to unmarshal mirror snapshot schedule list recursive response")
	}

	logger.Debugf("successfully recursively listed snapshot schedules for pool %q", poolName)
	return snapshotSchedulesRecursive, nil
}

// EnableRBDRadosNamespaceMirroring enables rbd mirroring on a rados namespace.
func EnableRBDRadosNamespaceMirroring(context *clusterd.Context, clusterInfo *ClusterInfo, poolAndRadosNamespaceName string, remoteNamespace *string, mode string) error {
	logger.Infof("enable mirroring in rados namespace %s in k8s namespace %q", poolAndRadosNamespaceName, clusterInfo.Namespace)

	// remove the check when the min supported version is 20.0.0
	if !clusterInfo.CephVersion.IsAtLeast(radosNamespaceMirroringMinimumVersion) {
		return errors.Errorf("ceph version %q does not support mirroring in rados namespace %q with --remote-namespace flag, supported version are v20 and above.", clusterInfo.CephVersion.String(), poolAndRadosNamespaceName)
	}

	if remoteNamespace != nil && *remoteNamespace == cephv1.ImplicitNamespaceKey {
		*remoteNamespace = cephv1.ImplicitNamespaceVal
	}

	args := []string{"mirror", "pool", "enable", poolAndRadosNamespaceName, mode}
	if remoteNamespace != nil {
		args = []string{"mirror", "pool", "enable", poolAndRadosNamespaceName, mode, "--remote-namespace", *remoteNamespace}
	}

	cmd := NewRBDCommand(context, clusterInfo, args)
	cmd.JsonOutput = false
	output, err := cmd.Run()
	if err != nil {
		return errors.Wrapf(err, "failed to enable mirroring in rados namespace %s with mode %s. %s", poolAndRadosNamespaceName, mode, output)
	}

	logger.Infof("successfully enabled mirroring in rados namespace %s in k8s namespace %q", poolAndRadosNamespaceName, clusterInfo.Namespace)
	return nil
}

func DisableRBDRadosNamespaceMirroring(context *clusterd.Context, clusterInfo *ClusterInfo, poolAndRadosNamespaceName string) error {
	logger.Infof("disable mirroring in rados namespace %s in k8s namespace %q", poolAndRadosNamespaceName, clusterInfo.Namespace)
	args := []string{"mirror", "pool", "disable", poolAndRadosNamespaceName}
	cmd := NewRBDCommand(context, clusterInfo, args)
	cmd.JsonOutput = false
	output, err := cmd.Run()
	if err != nil {
		return errors.Wrapf(err, "failed to disable mirroring in rados namespace %s. %s", poolAndRadosNamespaceName, output)
	}

	logger.Infof("successfully disabled mirroring in rados namespace %s in k8s namespace %q", poolAndRadosNamespaceName, clusterInfo.Namespace)
	return nil
}

/*
	CreateRBDMirrorBootstrapPeerWithoutPool creates a bootstrap peer for the current cluster

It creates the cephx user for the remote cluster to use with all the necessary details
This function is handy on scenarios where no pools have been created yet but replication communication is required (connecting peers)
It essentially sits above CreateRBDMirrorBootstrapPeer()
and is a cluster-wide option in the scenario where all the pools will be mirrored to the same remote cluster

So the scenario looks like:

 1. Create the cephx ID on the source cluster

 2. Enable a source pool for mirroring - at any time, we just don't know when
    rbd --cluster site-a mirror pool enable image-pool image

 3. Copy the key details over to the other cluster (non-ceph workflow)

 4. Enable destination pool for mirroring
    rbd --cluster site-b mirror pool enable image-pool image

 5. Add the peer details to the destination pool

 6. Repeat the steps flipping source and destination to enable
    bi-directional mirroring
*/
func CreateRBDMirrorBootstrapPeerWithoutPool(context *clusterd.Context, clusterInfo *ClusterInfo) ([]byte, error) {
	fullClientName := getQualifiedUser(rbdMirrorPeerKeyringID)
	logger.Infof("create rbd-mirror bootstrap peer token %q", fullClientName)
	key, err := AuthGetOrCreateKey(context, clusterInfo, fullClientName, rbdMirrorPeerCaps)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to create rbd-mirror peer key %q", fullClientName)
	}
	logger.Infof("successfully created rbd-mirror bootstrap peer token for cluster %q", clusterInfo.NamespacedName().Name)

	mons := sets.New[string]()
	for _, mon := range clusterInfo.AllMonitors() {
		mons.Insert(mon.Endpoint)
	}

	peerToken := PeerToken{
		ClusterFSID: clusterInfo.FSID,
		ClientID:    rbdMirrorPeerKeyringID,
		Key:         key,
		MonHost:     strings.Join(sets.List(mons), ","),
		Namespace:   clusterInfo.Namespace,
	}

	// Marshal the Go type back to JSON
	decodedTokenBackToJSON, err := json.Marshal(peerToken)
	if err != nil {
		return nil, errors.Wrap(err, "failed to encode peer token to json")
	}

	// Return the base64 encoded token
	return []byte(base64.StdEncoding.EncodeToString(decodedTokenBackToJSON)), nil
}
