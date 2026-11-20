/*
Copyright 2026 The Rook Authors. All rights reserved.

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
	"path"
	"regexp"
	"strings"

	"github.com/google/uuid"
	"github.com/pkg/errors"
	"github.com/rook/rook/pkg/clusterd"
	cephclient "github.com/rook/rook/pkg/daemon/ceph/client"
	oposd "github.com/rook/rook/pkg/operator/ceph/cluster/osd"
	"github.com/rook/rook/pkg/util/sys"
)

// Encryption keys of host-based OSDs, kept in a KMS.
//
// The prepare job generates the key and passes it to "ceph-volume raw prepare", which sends it to
// the mons with "ceph osd new". The operator copies it from the mons into the KMS under the OSD
// fsid before it creates the OSD deployment, so the prepare job never touches the KMS.

// hostKMSEnabled reports whether encrypted host-based OSDs keep their keys in a KMS. The operator
// sets the variable only when a validated KMS is configured.
func hostKMSEnabled() bool {
	return !isOnPVC && os.Getenv(oposd.HostOSDKMSEnvVarName) == "true"
}

// setNewHostOSDKey generates the key of a new encrypted OSD and passes it to ceph-volume, which
// reads it from the environment only; "raw prepare --dmcrypt" fails without it
func setNewHostOSDKey() error {
	key, err := oposd.GenerateDmCryptKey()
	if err != nil {
		return errors.Wrap(err, "failed to generate the encryption key")
	}
	return errors.Wrap(os.Setenv(oposd.CephVolumeEncryptedKeyEnvVarName, key), "failed to set the encryption key for ceph-volume")
}

// mapperOfPreparedOSD returns the mapper of the encrypted OSD that "ceph-volume raw prepare"
// created, "/dev/mapper/ceph-<osd fsid>-<kernel name>-block-dmcrypt"
func mapperOfPreparedOSD(prepareOutput string) (string, error) {
	mapper := getEncryptedBlockPath(prepareOutput, oposd.DmcryptBlockType)
	if osdUUIDFromMapper(mapper) == "" {
		return "", errors.New("failed to find the OSD fsid in the ceph-volume output")
	}
	return mapper, nil
}

// openHostOSDs opens the closed encrypted raw OSDs of this cluster on this node with their key from
// the mons, so that "ceph-volume raw list" reports an OSD that got no deployment before a reboot.
// Purged OSDs whose disk was not wiped are left alone.
func openHostOSDs(context *clusterd.Context, clusterInfo *cephclient.ClusterInfo, clusterOSDs map[string]bool) {
	open := map[string]bool{}
	for _, disk := range context.Devices {
		if disk.Type != sys.CryptType {
			continue
		}
		for _, link := range strings.Fields(disk.DevLinks) {
			if strings.HasPrefix(link, "/dev/mapper/ceph-") && strings.HasSuffix(link, "-"+oposd.DmcryptBlockType) {
				open[osdUUIDFromMapper(link)] = true
			}
		}
	}
	for _, disk := range context.Devices {
		if disk.Filesystem != "crypto_LUKS" {
			continue
		}
		dump, err := dumpLUKS(context, disk.RealPath)
		if err != nil {
			logger.Warningf("failed to check device %q for an encrypted OSD. %v", disk.RealPath, err)
			continue
		}
		osdUUID := osdUUIDFromLUKS(dump)
		if osdUUID == "" || open[osdUUID] || !clusterOSDs[osdUUID] {
			continue
		}
		key, err := monKeyOfOSD(context, clusterInfo, osdUUID)
		if err != nil {
			logger.Warningf("not opening OSD %q on device %q. %v", osdUUID, disk.RealPath, err)
			continue
		}
		mapper := fmt.Sprintf("ceph-%s-%s-%s", osdUUID, path.Base(disk.RealPath), oposd.DmcryptBlockType)
		if err := openEncryptedDevice(context, disk.RealPath, mapper, key); err != nil {
			logger.Warningf("failed to open OSD %q. %v", osdUUID, err)
			continue
		}
		logger.Infof("opened OSD %q on device %q as %q", osdUUID, disk.RealPath, mapper)
	}
}

// monKeyOfOSD returns the encryption key that "ceph osd new" stored in the mons for the OSD
func monKeyOfOSD(context *clusterd.Context, clusterInfo *cephclient.ClusterInfo, osdUUID string) (string, error) {
	key, err := cephclient.NewCephCommand(context, clusterInfo, []string{"config-key", "get", fmt.Sprintf("dm-crypt/osd/%s/luks", osdUUID)}).Run()
	if err != nil {
		return "", errors.Wrapf(err, "failed to read the encryption key of OSD %q from the mons", osdUUID)
	}
	return string(key), nil
}

// clusterOSDUUIDs returns the fsids of the OSDs of this cluster
func clusterOSDUUIDs(context *clusterd.Context, clusterInfo *cephclient.ClusterInfo) (map[string]bool, error) {
	dump, err := cephclient.GetOSDDump(context, clusterInfo)
	if err != nil {
		return nil, err
	}
	uuids := map[string]bool{}
	for _, osd := range dump.OSDs {
		uuids[osd.UUID] = true
	}
	return uuids, nil
}

var luksSubsystemCephFSID = regexp.MustCompile(`(?m)^Subsystem:\s*ceph_fsid=(\S+)`)

// osdUUIDFromLUKS returns the OSD fsid that ceph-volume writes to the LUKS2 subsystem of raw mode
// OSDs, "ceph_fsid=<osd fsid>"
func osdUUIDFromLUKS(dump string) string {
	match := luksSubsystemCephFSID.FindStringSubmatch(dump)
	if match == nil {
		return ""
	}
	id, err := uuid.Parse(strings.TrimSpace(match[1]))
	if err != nil {
		return ""
	}
	return id.String()
}

// osdUUIDFromMapper returns the OSD fsid from the mapper that ceph-volume opens on prepare,
// "/dev/mapper/ceph-<osd fsid>-<kernel name>-block-dmcrypt"
func osdUUIDFromMapper(mapper string) string {
	name := strings.TrimPrefix(path.Base(mapper), "ceph-")
	if len(name) < 36 {
		return ""
	}
	id, err := uuid.Parse(name[:36])
	if err != nil {
		return ""
	}
	return id.String()
}
