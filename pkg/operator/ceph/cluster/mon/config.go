/*
Copyright 2017 The Rook Authors. All rights reserved.

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

package mon

import (
	"fmt"
	"path"
	"strings"

	"github.com/pkg/errors"
	"github.com/rook/rook/pkg/clusterd"
	cephclient "github.com/rook/rook/pkg/daemon/ceph/client"
	opcontroller "github.com/rook/rook/pkg/operator/ceph/controller"
	v1 "k8s.io/api/core/v1"
)

const (
	//#nosec G101 -- This is only a path name
	CephSecretMountPath = "/var/lib/rook-ceph-mon"
	//#nosec G101 -- This is only a filename
	CephSecretFilename = "secret.keyring"
	//#nosec G101 -- This is only a volume name
	cephSecretVolumeName = "ceph-admin-secret"

	// All mons share the same keyring
	keyringStoreName = "rook-ceph-mons"

	// The final string field is for the admin keyring
	keyringTemplate = `
[mon.]
	key = %s
	caps mon = "allow *"

%s`
)

func (c *Cluster) genMonSharedKeyring() string {
	return fmt.Sprintf(
		keyringTemplate,
		c.ClusterInfo.MonitorSecret,
		cephclient.CephKeyring(c.ClusterInfo.CephCred),
	)
}

// return mon data dir path relative to the dataDirHostPath given a mon's name
func dataDirRelativeHostPath(monName string) string {
	monHostDir := monName // support legacy case where the mon name is "mon#" and not a lettered ID
	if !strings.Contains(monName, "mon") {
		// if the mon name doesn't have "mon" in it, mon dir is "mon-<ID>"
		monHostDir = "mon-" + monName
	}
	// Keep existing behavior where Rook stores the mon's data in the "data" subdir
	return path.Join(monHostDir, "data")
}

// WriteConnectionConfig save monitor connection config to disk
func WriteConnectionConfig(context *clusterd.Context, clusterInfo *cephclient.ClusterInfo) error {
	// write the latest config to the config dir
	if _, err := cephclient.GenerateConnectionConfig(context, clusterInfo); err != nil {
		return errors.Wrap(err, "failed to write connection config")
	}

	return nil
}

// CephSecretVolume is a volume for the ceph admin secret
func CephSecretVolume() v1.Volume {
	return v1.Volume{
		Name: cephSecretVolumeName,
		VolumeSource: v1.VolumeSource{
			Secret: &v1.SecretVolumeSource{
				SecretName: AppName,
				Items:      []v1.KeyToPath{{Key: opcontroller.CephUserSecretKey, Path: CephSecretFilename}},
			},
		},
	}
}

// CephSecretVolumeMount is a mount for the ceph admin secret
func CephSecretVolumeMount() v1.VolumeMount {
	return v1.VolumeMount{
		Name:      cephSecretVolumeName,
		MountPath: CephSecretMountPath,
		ReadOnly:  true,
	}
}
