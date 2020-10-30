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
	"strconv"

	"github.com/pkg/errors"
	cephv1 "github.com/rook/rook/pkg/apis/ceph.rook.io/v1"
	"github.com/rook/rook/pkg/operator/ceph/cluster/mgr"
	opconfig "github.com/rook/rook/pkg/operator/ceph/config"
	"github.com/rook/rook/pkg/operator/ceph/config/keyring"
	v1 "k8s.io/api/core/v1"
)

const (
	// don't list caps in keyring; allow OSD to get those from mons
	keyringTemplate = `[osd.%s]
key = %s
`

	// OsdEncryptionSecretNameKeyName is the key name of the Secret that contains the OSD encryption key
	// #nosec G101 since this is not leaking any hardcoded credentials, it's just the secret key name
	OsdEncryptionSecretNameKeyName = "dmcrypt-key"
	dmCryptKeySize                 = 128
)

func (c *Cluster) generateKeyring(osdID int) (string, error) {
	deploymentName := fmt.Sprintf(osdAppNameFmt, osdID)
	osdIDStr := strconv.Itoa(osdID)

	user := fmt.Sprintf("osd.%s", osdIDStr)
	access := []string{"osd", "allow *", "mon", "allow profile osd"}

	s := keyring.GetSecretStore(c.context, c.clusterInfo, &c.clusterInfo.OwnerRef)

	key, err := s.GenerateKey(user, access)
	if err != nil {
		return "", err
	}

	keyring := fmt.Sprintf(keyringTemplate, osdIDStr, key)
	return keyring, s.CreateOrUpdate(deploymentName, keyring)
}

// PrivilegedContext returns a privileged Pod security context
func PrivilegedContext() *v1.SecurityContext {
	privileged := true

	return &v1.SecurityContext{
		Privileged: &privileged,
	}
}

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

func encryptionDMName(pvcName, blockType string) string {
	return fmt.Sprintf("%s-%s", pvcName, blockType)
}

func encryptionDMPath(pvcName, blockType string) string {
	return path.Join("/dev/mapper", encryptionDMName(pvcName, blockType))
}

func encryptionBlockDestinationCopy(mountPath, blockType string) string {
	return path.Join(mountPath, blockType) + "-tmp"
}

func generateDmCryptKey() (string, error) {
	key, err := mgr.GenerateRandomBytes(dmCryptKeySize)
	if err != nil {
		return "", errors.Wrap(err, "failed to generate random bytes")
	}

	return base64.StdEncoding.EncodeToString(key), nil
}

func (c *Cluster) isCephVolumeRawModeSupported() bool {
	if c.clusterInfo.CephVersion.IsAtLeast(cephVolumeRawEncryptionModeMinNautilusCephVersion) && !c.clusterInfo.CephVersion.IsOctopus() {
		return true
	}
	if c.clusterInfo.CephVersion.IsAtLeast(cephVolumeRawEncryptionModeMinOctopusCephVersion) {
		return true
	}

	return false
}
