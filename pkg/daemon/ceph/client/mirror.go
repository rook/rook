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
	"encoding/json"
	"fmt"
	"io/ioutil"
	"os"

	"github.com/pkg/errors"
	cephv1 "github.com/rook/rook/pkg/apis/ceph.rook.io/v1"
	"github.com/rook/rook/pkg/clusterd"
)

// PoolMirroringStatus is the mirroring status of a given pool
type PoolMirroringStatus struct {
	Summary SummarySpec `json:"summary"`
}

// SummarySpec is the summary output of the command
type SummarySpec struct {
	Health       string      `json:"health"`
	DaemonHealth string      `json:"daemon_health"`
	ImageHealth  string      `json:"image_health"`
	States       interface{} `json:"states"`
}

// PoolMirroringInfo is the mirroring info of a given pool
type PoolMirroringInfo struct {
	Mode     string      `json:"mode"`
	SiteName string      `json:"site_name"`
	Peers    []PeersSpec `json:"peers"`
}

// PeersSpec contains peer details
type PeersSpec struct {
	UUID       string `json:"uuid"`
	Direction  string `json:"direction"`
	SiteName   string `json:"site_name"`
	MirrorUUID string `json:"mirror_uuid"`
	ClientName string `json:"client_name"`
}

// SnapshotScheduleStatus is the mirroring status of a given pool
type SnapshotScheduleStatus struct {
	ScheduleTime string `json:"schedule_time"`
	Image        string `json:"image"`
}

// SnapshotSchedule is a schedule
type SnapshotSchedule struct {
	Interval  string `json:"interval"`
	StartTime string `json:"start_time"`
}

// ImportRBDMirrorBootstrapPeer add a mirror peer in the rbd-mirror configuration
func ImportRBDMirrorBootstrapPeer(context *clusterd.Context, clusterInfo *ClusterInfo, poolName, direction string, token []byte) error {
	logger.Infof("add rbd-mirror bootstrap peer token for pool %q", poolName)

	// Token file
	tokenFilePath := fmt.Sprintf("/tmp/rbd-mirror-token-%s", poolName)

	// Write token into a file
	err := ioutil.WriteFile(tokenFilePath, token, 0400)
	if err != nil {
		return errors.Wrapf(err, "failed to write token to file %q", tokenFilePath)
	}

	// Remove token once we exit, we don't need it anymore
	defer func() error {
		err := os.Remove(tokenFilePath)
		return err
	}() //nolint, we don't want to return here

	// Build command
	args := []string{"mirror", "pool", "peer", "bootstrap", "import", poolName, tokenFilePath}
	if direction != "" {
		args = append(args, "--direction", direction)
	}
	cmd := NewRBDCommand(context, clusterInfo, args)

	// Run command
	output, err := cmd.Run()
	if err != nil {
		return errors.Wrapf(err, "failed to add rbd-mirror peer token for pool %q. %s", poolName, output)
	}

	return nil
}

// CreateRBDMirrorBootstrapPeer add a mirror peer in the rbd-mirror configuration
func CreateRBDMirrorBootstrapPeer(context *clusterd.Context, clusterInfo *ClusterInfo, poolName string) ([]byte, error) {
	logger.Infof("create rbd-mirror bootstrap peer token for pool %q", poolName)

	// Build command
	args := []string{"mirror", "pool", "peer", "bootstrap", "create", poolName, "--site-name", fmt.Sprintf("%s-%s", clusterInfo.FSID, clusterInfo.Namespace)}
	cmd := NewRBDCommand(context, clusterInfo, args)

	// Run command
	output, err := cmd.Run()
	if err != nil {
		return nil, errors.Wrapf(err, "failed to create rbd-mirror peer token  for pool %q. %s", poolName, output)
	}

	return output, nil
}

// enablePoolMirroring turns on mirroring on that pool by specifying the mirroring type
func enablePoolMirroring(context *clusterd.Context, clusterInfo *ClusterInfo, pool cephv1.PoolSpec, poolName string) error {
	logger.Infof("enabling mirroring type %q for pool %q", pool.Mirroring.Mode, poolName)

	// Build command
	args := []string{"mirror", "pool", "enable", poolName, pool.Mirroring.Mode}
	cmd := NewRBDCommand(context, clusterInfo, args)

	// Run command
	output, err := cmd.Run()
	if err != nil {
		return errors.Wrapf(err, "failed to enable mirroring type %q for pool %q. %s", pool.Mirroring.Mode, poolName, output)
	}

	return nil
}

// GetPoolMirroringStatus prints the pool mirroring status
func GetPoolMirroringStatus(context *clusterd.Context, clusterInfo *ClusterInfo, poolName string) (*PoolMirroringStatus, error) {
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

	var poolMirroringStatus PoolMirroringStatus
	if err := json.Unmarshal([]byte(buf), &poolMirroringStatus); err != nil {
		return nil, errors.Wrap(err, "failed to unmarshal mirror pool status response")
	}

	return &poolMirroringStatus, nil
}

// GetPoolMirroringInfo  prints the pool mirroring information
func GetPoolMirroringInfo(context *clusterd.Context, clusterInfo *ClusterInfo, poolName string) (*PoolMirroringInfo, error) {
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
	var poolMirroringInfo PoolMirroringInfo
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
func removeSnapshotSchedule(context *clusterd.Context, clusterInfo *ClusterInfo, snapScheduleResponse SnapshotSchedule, poolName string) error {
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

func enableSnapshotSchedules(context *clusterd.Context, clusterInfo *ClusterInfo, poolSpec cephv1.PoolSpec, poolName string) error {
	logger.Info("resetting current snapshot schedules")
	// Reset any existing schedules
	err := removeSnapshotSchedules(context, clusterInfo, poolSpec, poolName)
	if err != nil {
		logger.Errorf("failed to remove snapshot schedules. %v", err)
	}

	// Enable all the snap schedules
	for _, snapSchedule := range poolSpec.Mirroring.SnapshotSchedules {
		err := enableSnapshotSchedule(context, clusterInfo, snapSchedule, poolName)
		if err != nil {
			return errors.Wrap(err, "failed to enable snapshot schedule")
		}
	}

	return nil
}

// removeSnapshotSchedules removes all the existing snapshot schedules
func removeSnapshotSchedules(context *clusterd.Context, clusterInfo *ClusterInfo, poolSpec cephv1.PoolSpec, poolName string) error {
	// Get the list of existing snapshot schedule
	existingSnapshotSchedules, err := listSnapshotSchedules(context, clusterInfo, poolName)
	if err != nil {
		return errors.Wrap(err, "failed to list snapshot schedule(s)")
	}

	// Remove each schedule
	for _, existingSnapshotSchedule := range existingSnapshotSchedules {
		err := removeSnapshotSchedule(context, clusterInfo, existingSnapshotSchedule, poolName)
		if err != nil {
			return errors.Wrapf(err, "failed to remove snapshot schedule %v", existingSnapshotSchedule)
		}
	}

	return nil
}

// GetSnapshotScheduleStatus configures the snapshots schedule on a mirrored pool
func GetSnapshotScheduleStatus(context *clusterd.Context, clusterInfo *ClusterInfo, poolName string) ([]SnapshotScheduleStatus, error) {
	// Build command
	args := []string{"mirror", "snapshot", "schedule", "status", "--pool", poolName}
	cmd := NewRBDCommand(context, clusterInfo, args)
	cmd.JsonOutput = true

	// Run command
	buf, err := cmd.Run()
	if err != nil {
		return nil, errors.Wrapf(err, "failed to retrieve snapshot schedule status on pool %q. %s", poolName, string(buf))
	}

	// Unmarshal JSON into Go struct
	var snapshotScheduleStatus []SnapshotScheduleStatus
	if err := json.Unmarshal([]byte(buf), &snapshotScheduleStatus); err != nil {
		return nil, errors.Wrap(err, "failed to unmarshal mirror snapshot schedule status response")
	}

	logger.Debugf("successfully retrieved snapshot schedule status for pool %q", poolName)
	return snapshotScheduleStatus, nil
}

// listSnapshotSchedules configures the snapshots schedule on a mirrored pool
func listSnapshotSchedules(context *clusterd.Context, clusterInfo *ClusterInfo, poolName string) ([]SnapshotSchedule, error) {
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
	var snapshotSchedules []SnapshotSchedule
	if err := json.Unmarshal([]byte(buf), &snapshotSchedules); err != nil {
		return nil, errors.Wrap(err, "failed to unmarshal mirror snapshot schedule list response")
	}

	logger.Debugf("successfully listed snapshot schedules for pool %q", poolName)
	return snapshotSchedules, nil
}
