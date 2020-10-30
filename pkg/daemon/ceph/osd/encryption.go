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
	"os"

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
	// The token if any is mounted in the provisioner pod as an env variable so the secrets lib will pick it up
	kmsConfig := kms.NewConfig(context, &v1.ClusterSpec{Security: v1.SecuritySpec{KeyManagementService: v1.KeyManagementServiceSpec{ConnectionDetails: kms.ConfigEnvsToMapString()}}}, clusterInfo)
	if kmsConfig.IsVault() {
		// Fetch the KEK
		kek, err := kmsConfig.GetSecret(os.Getenv(oposd.PVCNameEnvVarName))
		if err != nil {
			return errors.Wrapf(err, "failed to retrieve key encryption key from %q kms", kmsConfig.Provider)
		}

		// Set the KEK as an env variable for ceph-volume
		err = os.Setenv(oposd.CephVolumeEncryptedKeyEnvVarName, kek)
		if err != nil {
			return errors.Wrap(err, "failed to set key encryption key env variable for ceph-volume")
		}
	}

	return nil
}
