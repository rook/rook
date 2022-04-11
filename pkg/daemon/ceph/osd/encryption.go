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
	"fmt"
	"os"
	"regexp"
	"strings"

	"github.com/pkg/errors"
	v1 "github.com/rook/rook/pkg/apis/ceph.rook.io/v1"
	"github.com/rook/rook/pkg/clusterd"
	cephclient "github.com/rook/rook/pkg/daemon/ceph/client"
	"github.com/rook/rook/pkg/daemon/ceph/osd/kms"
	oposd "github.com/rook/rook/pkg/operator/ceph/cluster/osd"
)

const (
	cryptsetupBinary = "cryptsetup"
	dmsetupBinary    = "dmsetup"
)

var (
	luksLabelCephFSID = regexp.MustCompile("ceph_fsid=(.*)")
)

func closeEncryptedDevice(context *clusterd.Context, dmName string) error {
	args := []string{"--verbose", "luksClose", dmName}
	cryptsetupOut, err := context.Executor.ExecuteCommandWithCombinedOutput(cryptsetupBinary, args...)
	if err != nil {
		return errors.Wrapf(err, "failed to close encrypted device. %s", cryptsetupOut)
	}

	logger.Infof("dm version:\n%s", cryptsetupOut)
	return nil
}

func dmsetupVersion(context *clusterd.Context) error {
	args := []string{"version"}
	dmsetupOut, err := context.Executor.ExecuteCommandWithCombinedOutput(dmsetupBinary, args...)
	if err != nil {
		return errors.Wrapf(err, "failed to find device mapper version. %s", dmsetupOut)
	}

	logger.Info(dmsetupOut)
	return nil
}

func setKEKinEnv(context *clusterd.Context, clusterInfo *cephclient.ClusterInfo) error {
	// KMS details are passed by the Operator as env variables in the pod
	// The token if any is mounted in the provisioner pod as an env variable so the secrets lib will
	// pick it up
	clusterSpec := &v1.ClusterSpec{Security: v1.SecuritySpec{KeyManagementService: v1.KeyManagementServiceSpec{ConnectionDetails: kms.ConfigEnvsToMapString()}}}

	// If KMS is not enabled this code does not need to run since we attach the KEK value from the
	// Kubernetes Secret in the provision pod spec (mounted as an environment variable)
	if !clusterSpec.Security.KeyManagementService.IsEnabled() {
		logger.Debug("cluster-wide encryption is enabled with kubernetes secrets and the kek is attached to the provision env spec")
		return nil
	}

	// The ibm key protect library does not read any environment variables, so we must set the
	// service api key (coming from the secret mounted as environment variable) in the KMS
	// connection details. These details are used to build the client connection
	if clusterSpec.Security.KeyManagementService.IsIBMKeyProtectKMS() {
		ibmServiceApiKey := os.Getenv(kms.IbmKeyProtectServiceApiKey)
		if ibmServiceApiKey == "" {
			return errors.Errorf("ibm key protect %q environment variable is not set", kms.IbmKeyProtectServiceApiKey)
		}
		clusterSpec.Security.KeyManagementService.ConnectionDetails[kms.IbmKeyProtectServiceApiKey] = ibmServiceApiKey
	}

	kmsConfig := kms.NewConfig(context, clusterSpec, clusterInfo)

	// Fetch the KEK
	kek, err := kmsConfig.GetSecret(os.Getenv(oposd.PVCNameEnvVarName))
	if err != nil {
		return errors.Wrapf(err, "failed to retrieve key encryption key from %q kms", kmsConfig.Provider)
	}

	if kek == "" {
		return errors.New("key encryption key is empty")
	}

	// Set the KEK as an env variable for ceph-volume
	err = os.Setenv(oposd.CephVolumeEncryptedKeyEnvVarName, kek)
	if err != nil {
		return errors.Wrap(err, "failed to set key encryption key env variable for ceph-volume")
	}

	logger.Debug("successfully set kek to env variable")
	return nil
}

func setLUKSLabelAndSubsystem(context *clusterd.Context, clusterInfo *cephclient.ClusterInfo, disk string) error {
	// The PVC info is a nice to have
	pvcName := os.Getenv(oposd.PVCNameEnvVarName)
	if pvcName == "" {
		return errors.Errorf("failed to find %q environment variable", oposd.PVCNameEnvVarName)
	}
	subsystem := fmt.Sprintf("ceph_fsid=%s", clusterInfo.FSID)
	label := fmt.Sprintf("pvc_name=%s", pvcName)

	logger.Infof("setting LUKS subsystem to %q and label to %q to disk %q", subsystem, label, disk)
	// 48 characters limit for both label and subsystem
	args := []string{"config", disk, "--subsystem", subsystem, "--label", label}
	output, err := context.Executor.ExecuteCommandWithCombinedOutput(cryptsetupBinary, args...)
	if err != nil {
		return errors.Wrapf(err, "failed to set subsystem %q and label %q to encrypted device %q. is your distro built with LUKS1 as a default?. %s", subsystem, label, disk, output)
	}

	logger.Infof("successfully set LUKS subsystem to %q and label to %q to disk %q", subsystem, label, disk)
	return nil
}

func dumpLUKS(context *clusterd.Context, disk string) (string, error) {
	args := []string{"luksDump", disk}
	cryptsetupOut, err := context.Executor.ExecuteCommandWithCombinedOutput(cryptsetupBinary, args...)
	if err != nil {
		return "", errors.Wrapf(err, "failed to dump LUKS header for disk %q. %s", disk, cryptsetupOut)
	}

	return cryptsetupOut, nil
}

func isCephEncryptedBlock(context *clusterd.Context, currentClusterFSID string, disk string) bool {
	metadata, err := dumpLUKS(context, disk)
	if err != nil {
		logger.Errorf("failed to determine if the encrypted block %q is from our cluster. %v", disk, err)
		return false
	}

	// Now we parse the CLI output
	// JSON output is only available with cryptsetup 2.4.x - https://gitlab.com/cryptsetup/cryptsetup/-/issues/511
	ceph_fsid := luksLabelCephFSID.FindString(metadata)
	if ceph_fsid == "" {
		logger.Error("failed to find ceph_fsid in the LUKS header, the encrypted disk is not from a ceph cluster")
		return false
	}

	// is it an OSD from our cluster?
	currentDiskCephFSID := strings.SplitAfter(ceph_fsid, "=")[1]
	if currentDiskCephFSID != currentClusterFSID {
		logger.Errorf("encrypted disk %q is part of a different ceph cluster %q", disk, currentDiskCephFSID)
		return false
	}

	return true

}
