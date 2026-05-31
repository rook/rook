/*
Copyright 2016 The Rook Authors. All rights reserved.

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
	"path"

	"github.com/rook/rook/pkg/clusterd"
	cephclient "github.com/rook/rook/pkg/daemon/ceph/client"
)

const (
	bootstrapOsdKeyring = "bootstrap-osd/ceph.keyring"
)

// create a keyring for the bootstrap-osd client, it gets a limited set of privileges
func createOSDBootstrapKeyring(context *clusterd.Context, clusterInfo *cephclient.ClusterInfo, rootDir string) error {
	username := "client.bootstrap-osd"
	keyringPath := path.Join(rootDir, bootstrapOsdKeyring)
	access := []string{"mon", "allow profile bootstrap-osd"}
	keyringEval := func(key string) string {
		return fmt.Sprintf(bootstrapOSDKeyringTemplate, key)
	}

	return cephclient.CreateKeyring(context, clusterInfo, username, keyringPath, access, keyringEval)
}
