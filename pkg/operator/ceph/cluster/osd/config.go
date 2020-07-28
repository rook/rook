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
	"fmt"
	"strconv"

	"github.com/pkg/errors"
	cephv1 "github.com/rook/rook/pkg/apis/ceph.rook.io/v1"
	opconfig "github.com/rook/rook/pkg/operator/ceph/config"
	"github.com/rook/rook/pkg/operator/ceph/config/keyring"
	"github.com/rook/rook/pkg/operator/k8sutil"
	v1 "k8s.io/api/core/v1"
)

const (
	// don't list caps in keyring; allow OSD to get those from mons
	keyringTemplate = `[osd.%s]
key = %s
`

	// OSDs on PVC using a certain storage class need to do some tuning
	osdRecoverySleep = "0.1"
	osdSnapTrimSleep = "2"
	osdDeleteSleep   = "2"
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

func (c *Cluster) osdRunFlagTuningOnPVC(osdID int) error {
	who := fmt.Sprintf("osd.%d", osdID)
	do := make(map[string]string)

	// Time in seconds to sleep before next recovery or backfill op
	do["osd_recovery_sleep"] = osdRecoverySleep
	// Time in seconds to sleep before next snap trim
	do["osd_snap_trim_sleep"] = osdSnapTrimSleep
	// Time in seconds to sleep before next removal transaction
	do["osd_delete_sleep"] = osdDeleteSleep

	monStore := opconfig.GetMonStore(c.context, c.clusterInfo)

	for flag, val := range do {
		err := monStore.Set(who, flag, val)
		if err != nil {
			return errors.Wrapf(err, "failed to set %q to %q on %q", flag, val, who)
		}
	}

	return nil
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

func (c *Cluster) skipVolumeForDirectory(path string) bool {
	// If attempting to add a directory at /var/lib/rook, we need to skip the volume and volume mount
	// since the dataDirHostPath is always mounting at /var/lib/rook
	return path == k8sutil.DataDir
}
