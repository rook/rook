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
	"fmt"
	"strconv"
	"strings"

	"github.com/pkg/errors"
	cephv1 "github.com/rook/rook/pkg/apis/ceph.rook.io/v1"
	"github.com/rook/rook/pkg/clusterd"
	"github.com/rook/rook/pkg/util/exec"
	"k8s.io/apimachinery/pkg/types"
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

// PinCephFSSubVolumeGroup pin the cephfs subvolume group
func PinCephFSSubVolumeGroup(context *clusterd.Context, clusterInfo *ClusterInfo, volName string, cephFilesystemSubVolumeGroup *cephv1.CephFilesystemSubVolumeGroup, cephFilesystemSubVolumeGroupName string) error {
	// namespace is the namespace of the svg CR, name is the svg name spec otherwise svg CR name
	namespaceName := types.NamespacedName{Namespace: cephFilesystemSubVolumeGroup.Namespace, Name: cephFilesystemSubVolumeGroupName}
	logger.Infof("validating pinning configuration of cephfs subvolume group %v of filesystem %q", namespaceName, volName)
	err := validatePinningValues(cephFilesystemSubVolumeGroup.Spec.Pinning)
	if err != nil {
		return errors.Wrapf(err, "failed to pin subvolume group %q", cephFilesystemSubVolumeGroupName)
	}

	logger.Infof("pinning cephfs subvolume group %v of filesystem %q", namespaceName, volName)
	args := []string{"fs", "subvolumegroup", "pin", volName, cephFilesystemSubVolumeGroupName}
	if cephFilesystemSubVolumeGroup.Spec.Pinning.Distributed != nil {
		setting := strconv.Itoa(*cephFilesystemSubVolumeGroup.Spec.Pinning.Distributed)
		args = append(args, "distributed", setting)
	} else if cephFilesystemSubVolumeGroup.Spec.Pinning.Export != nil {
		setting := strconv.Itoa(*cephFilesystemSubVolumeGroup.Spec.Pinning.Export)
		args = append(args, "export", setting)
	} else if cephFilesystemSubVolumeGroup.Spec.Pinning.Random != nil {
		setting := strconv.FormatFloat(*cephFilesystemSubVolumeGroup.Spec.Pinning.Random, 'f', -1, 64)
		args = append(args, "random", setting)
	} else {
		// set by default value
		args = append(args, "distributed", "1")
	}
	logger.Infof("subvolume group pinning args %v", args)

	cmd := NewCephCommand(context, clusterInfo, args)
	cmd.JsonOutput = false
	output, err := cmd.Run()
	if err != nil {
		return errors.Wrapf(err, "failed to pin subvolume group %q. %s", cephFilesystemSubVolumeGroupName, output)
	}

	logger.Infof("successfully pinned cephfs subvolume group %v", namespaceName)
	return nil
}

func validatePinningValues(pinning cephv1.CephFilesystemSubVolumeGroupSpecPinning) error {
	numNils := 0
	var err error
	if pinning.Export != nil {
		numNils++
		if *pinning.Export > 256 {
			err = errors.Errorf("validate pinning type failed, Export: value too big %d", *pinning.Export)
		} else if *pinning.Export < -1 {
			err = errors.Errorf("validate pinning type failed, Export: negative value %d not allowed except -1", *pinning.Export)
		}
	}
	if pinning.Distributed != nil {
		numNils++
		if !(*pinning.Distributed == 1) && !(*pinning.Distributed == 0) {
			err = errors.Errorf("validate pinning type failed, Distributed: unknown value %d", *pinning.Distributed)
		}
	}
	if pinning.Random != nil {
		numNils++
		if (*pinning.Random < 0) || (*pinning.Random > 1.0) {
			err = errors.Errorf("validate pinning type failed, Random: value %.2f is not between 0.0 and 1.1 (inclusive)", *pinning.Random)
		}
	}
	if numNils > 1 {
		return fmt.Errorf("only one can be set")
	}
	if numNils == 0 {
		return nil // pinning disabled
	}
	return err
}

func GetOMAPKey(context *clusterd.Context, clusterInfo *ClusterInfo, omapObj, poolName, namespace string) (string, error) {
	args := []string{"getomapval", omapObj, "csi.volname", "-p", poolName, "--namespace", namespace, "/dev/stdout"}
	cmd := NewRadosCommand(context, clusterInfo, args)
	buf, err := cmd.RunWithTimeout(exec.CephCommandsTimeout)
	if err != nil {
		return "", errors.Wrapf(err, "failed to list omapKeys for omapObj %q", omapObj)
	}

	// Todo: is there a way to avoid this parsing?
	respStr := string(buf)
	var pvcName string
	if len(respStr) != 0 {
		resp := strings.Split(respStr, "\n")
		if len(resp) == 2 {
			pvcName = resp[1]
		}
	}

	if pvcName == "" {
		return "", nil
	}

	omapKey := fmt.Sprintf("ceph.volume.%s", pvcName)
	return omapKey, nil
}

func DeleteOmapValue(context *clusterd.Context, clusterInfo *ClusterInfo, omapValue, poolName, namespace string) error {
	args := []string{"rm", omapValue, "-p", poolName, "--namespace", namespace}
	cmd := NewRadosCommand(context, clusterInfo, args)
	_, err := cmd.RunWithTimeout(exec.CephCommandsTimeout)
	if err != nil {
		return errors.Wrapf(err, "failed to delete omap value %q in pool %q", omapValue, poolName)
	}
	logger.Infof("successfully deleted omap value %q for pool %q", omapValue, poolName)
	return nil
}

func DeleteOmapKey(context *clusterd.Context, clusterInfo *ClusterInfo, omapKey, poolName, namespace string) error {
	args := []string{"rmomapkey", "csi.volumes.default", omapKey, "-p", poolName, "--namespace", namespace}
	cmd := NewRadosCommand(context, clusterInfo, args)
	_, err := cmd.RunWithTimeout(exec.CephCommandsTimeout)
	if err != nil {
		return errors.Wrapf(err, "failed to delete omapKey %q in pool %q", omapKey, poolName)
	}
	logger.Infof("successfully deleted omap key %q for pool %q", omapKey, poolName)
	return nil
}

func DeleteSubVolume(context *clusterd.Context, clusterInfo *ClusterInfo, fs, subvol, svg string) error {
	args := []string{"fs", "subvolume", "rm", fs, subvol, svg, "--force"}
	cmd := NewCephCommand(context, clusterInfo, args)
	_, err := cmd.RunWithTimeout(exec.CephCommandsTimeout)
	if err != nil {
		return errors.Wrapf(err, "failed to delete subvolume %q in filesystem %q", subvol, fs)
	}
	return nil
}

func DeleteSubvolumeSnapshot(context *clusterd.Context, clusterInfo *ClusterInfo, fs, subvol, svg, snap string) error {
	args := []string{"fs", "subvolume", "snapshot", "rm", fs, subvol, snap, "--group_name", svg}
	cmd := NewCephCommand(context, clusterInfo, args)
	_, err := cmd.RunWithTimeout(exec.CephCommandsTimeout)
	if err != nil {
		return errors.Wrapf(err, "failed to delete subvolume %q in filesystem %q", subvol, fs)
	}
	return nil
}

func CancelSnapshotClone(context *clusterd.Context, clusterInfo *ClusterInfo, fs, svg, clone string) error {
	args := []string{"fs", "clone", "cancel", fs, clone, "--group_name", svg}
	cmd := NewCephCommand(context, clusterInfo, args)
	_, err := cmd.RunWithTimeout(exec.CephCommandsTimeout)
	if err != nil {
		return errors.Wrapf(err, "failed to cancel clone %q in filesystem %q in group %q", clone, fs, svg)
	}
	return nil
}
