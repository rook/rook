/*
Copyright 2018 The Rook Authors. All rights reserved.

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

package nfs

import (
	"fmt"

	"github.com/pkg/errors"
	cephv1 "github.com/rook/rook/pkg/apis/ceph.rook.io/v1"
	cephclient "github.com/rook/rook/pkg/daemon/ceph/client"
	"github.com/rook/rook/pkg/operator/ceph/config/keyring"
	cephver "github.com/rook/rook/pkg/operator/ceph/version"
	"github.com/rook/rook/pkg/operator/k8sutil"
)

const (
	keyringTemplate = `
[%s]
        key = %s
        caps mon = "allow r"
        caps osd = "%s"
`
)

func getNFSUserID(nodeID string) string {
	return fmt.Sprintf("nfs-ganesha.%s", nodeID)
}

func getNFSClientID(n *cephv1.CephNFS, name string) string {
	return fmt.Sprintf("client.%s", getNFSUserID(getNFSNodeID(n, name)))
}

func getNFSNodeID(n *cephv1.CephNFS, name string) string {
	return fmt.Sprintf("%s.%s", n.Name, name)
}

func getGaneshaConfigObject(n *cephv1.CephNFS, version cephver.CephVersion, name string) string {
	/* Exports created with Dashboard will not be affected by change in config object name.
	 * As it looks for ganesha config object just by 'conf-'. Exports cannot be created by using
	 * volume/nfs plugin in Octopus version. Because the ceph rook module is broken.
	 */
	if version.IsAtLeastOctopus() {
		return fmt.Sprintf("conf-nfs.%s", n.Name)
	}
	return fmt.Sprintf("conf-%s", getNFSNodeID(n, name))
}

func getRadosURL(n *cephv1.CephNFS, version cephver.CephVersion, name string) string {
	url := fmt.Sprintf("rados://%s/", n.Spec.RADOS.Pool)

	if n.Spec.RADOS.Namespace != "" {
		url += n.Spec.RADOS.Namespace + "/"
	}

	url += getGaneshaConfigObject(n, version, name)
	return url
}

func (r *ReconcileCephNFS) generateKeyring(n *cephv1.CephNFS, name string) error {
	osdCaps := fmt.Sprintf("allow rw pool=%s", n.Spec.RADOS.Pool)
	if n.Spec.RADOS.Namespace != "" {
		osdCaps = fmt.Sprintf("%s namespace=%s", osdCaps, n.Spec.RADOS.Namespace)
	}

	caps := []string{"mon", "allow r", "osd", osdCaps}
	user := getNFSClientID(n, name)

	ownerInfo := k8sutil.NewOwnerInfo(n, r.scheme)
	s := keyring.GetSecretStore(r.context, r.clusterInfo, ownerInfo)

	key, err := s.GenerateKey(user, caps)
	if err != nil {
		return errors.Wrapf(err, "failed to create user %s", user)
	}

	keyring := fmt.Sprintf(keyringTemplate, user, key, osdCaps)
	return s.CreateOrUpdate(instanceName(n, name), keyring)
}

func getGaneshaConfig(n *cephv1.CephNFS, version cephver.CephVersion, name string) string {
	nodeID := getNFSNodeID(n, name)
	userID := getNFSUserID(nodeID)
	url := getRadosURL(n, version, name)
	return `
NFS_CORE_PARAM {
	Enable_NLM = false;
	Enable_RQUOTA = false;
	Protocols = 4;
}

MDCACHE {
	Dir_Chunk = 0;
}

EXPORT_DEFAULTS {
	Attr_Expiration_Time = 0;
}

NFSv4 {
	Delegations = false;
	RecoveryBackend = 'rados_cluster';
	Minor_Versions = 1, 2;
}

RADOS_KV {
	ceph_conf = '` + cephclient.DefaultConfigFilePath() + `';
	userid = ` + userID + `;
	nodeid = ` + nodeID + `;
	pool = "` + n.Spec.RADOS.Pool + `";
	namespace = "` + n.Spec.RADOS.Namespace + `";
}

RADOS_URLS {
	ceph_conf = '` + cephclient.DefaultConfigFilePath() + `';
	userid = ` + userID + `;
	watch_url = '` + url + `';
}

%url	` + url + `
`
}
