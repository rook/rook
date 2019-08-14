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

	cephv1 "github.com/rook/rook/pkg/apis/ceph.rook.io/v1"
	cephconfig "github.com/rook/rook/pkg/daemon/ceph/config"
)

const (
	userID = "admin"
)

func getNFSNodeID(n cephv1.CephNFS, name string) string {
	return fmt.Sprintf("%s.%s", n.Name, name)
}

func getGaneshaConfigObject(nodeID string) string {
	return fmt.Sprintf("conf-%s", nodeID)
}

func getRadosURL(n cephv1.CephNFS, nodeID string) string {
	url := fmt.Sprintf("rados://%s/", n.Spec.RADOS.Pool)

	if n.Spec.RADOS.Namespace != "" {
		url += n.Spec.RADOS.Namespace + "/"
	}

	url += getGaneshaConfigObject(nodeID)
	return url
}

func getGaneshaConfig(n cephv1.CephNFS, name string) string {
	nodeID := getNFSNodeID(n, name)
	url := getRadosURL(n, nodeID)
	return `
NFS_CORE_PARAM {
	Enable_NLM = false;
	Enable_RQUOTA = false;
	Protocols = 4;
}

CACHEINODE {
	Dir_Chunk = 0;
	NParts = 1;
	Cache_Size = 1;
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
	ceph_conf = '` + cephconfig.DefaultConfigFilePath() + `';
	userid = ` + userID + `;
	nodeid = ` + nodeID + `;
	pool = "` + n.Spec.RADOS.Pool + `";
	namespace = "` + n.Spec.RADOS.Namespace + `";
}

RADOS_URLS {
	ceph_conf = '` + cephconfig.DefaultConfigFilePath() + `';
	userid = ` + userID + `;
	watch_url = '` + url + `';
}

%url	` + url + `
`
}
