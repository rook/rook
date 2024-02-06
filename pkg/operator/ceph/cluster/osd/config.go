/*
Copyright 2019 The Rook Authors. All rights reserved.

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
	"encoding/base64"
	"fmt"
	"path"

	"github.com/pkg/errors"
	cephv1 "github.com/rook/rook/pkg/apis/ceph.rook.io/v1"
	"github.com/rook/rook/pkg/operator/ceph/cluster/mgr"
	opconfig "github.com/rook/rook/pkg/operator/ceph/config"
)

const (
	dmCryptKeySize = 128
)

func osdOnSDNFlag(network cephv1.NetworkSpec) []string {
	var args []string
	// OSD fails to find the right IP to bind to when running on SDN
	// for more details: https://github.com/rook/rook/issues/3140
	if !network.IsHost() {
		args = append(args, "--ms-learn-addr-from-peer=false")
	}

	return args
}

func encryptionKeyPath() string {
	return path.Join(opconfig.EtcCephDir, encryptionKeyFileName)
}

func EncryptionDMName(pvcName, blockType string) string {
	return fmt.Sprintf("%s-%s", pvcName, blockType)
}

func EncryptionDMPath(pvcName, blockType string) string {
	return path.Join("/dev/mapper", EncryptionDMName(pvcName, blockType))
}

func encryptionBlockDestinationCopy(mountPath, blockType string) string {
	return path.Join(mountPath, blockType) + "-tmp"
}

func GenerateDmCryptKey() (string, error) {
	key, err := mgr.GenerateRandomBytes(dmCryptKeySize)
	if err != nil {
		return "", errors.Wrap(err, "failed to generate random bytes")
	}

	return base64.StdEncoding.EncodeToString(key), nil
}
