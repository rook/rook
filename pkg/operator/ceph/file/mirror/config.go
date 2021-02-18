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

package mirror

import (
	"fmt"

	"github.com/rook/rook/pkg/daemon/ceph/client"
	"github.com/rook/rook/pkg/operator/ceph/config"
	"github.com/rook/rook/pkg/operator/ceph/config/keyring"
	"github.com/rook/rook/pkg/operator/k8sutil"
)

const (
	keyringTemplate = `
[client.fs-mirror]
	key = %s
	caps mon = "allow r"
	caps mgr = "allow r"
	caps mds = "allow r"
	caps osd = "'allow rw tag cephfs metadata=*, allow r tag cephfs data=*'"

`
	user   = "client.fs-mirror"
	userID = "fs-mirror"
)

// daemonConfig for a single rbd-mirror
type daemonConfig struct {
	ResourceName string              // the name rook gives to mirror resources in k8s metadata
	DataPathMap  *config.DataPathMap // location to store data in container
	ownerInfo    *k8sutil.OwnerInfo
}

// PeerToken is the content of the peer token
type PeerToken struct {
	ClusterFSID string `json:"fsid"`
	ClientID    string `json:"client_id"`
	Key         string `json:"key"`
	MonHost     string `json:"mon_host"`
}

func (r *ReconcileFilesystemMirror) generateKeyring(clusterInfo *client.ClusterInfo, daemonConfig *daemonConfig) (string, error) {
	access := []string{
		"mon", "allow r",
		"mgr", "allow r",
		"mds", "allow r",
		"osd", "allow rw tag cephfs metadata=*, allow r tag cephfs data=*",
	}
	s := keyring.GetSecretStore(r.context, clusterInfo, daemonConfig.ownerInfo)

	key, err := s.GenerateKey(user, access)
	if err != nil {
		return "", err
	}

	keyring := fmt.Sprintf(keyringTemplate, key)
	return keyring, s.CreateOrUpdate(daemonConfig.ResourceName, keyring)
}
