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

package keyring

import (
	"fmt"

	cephconfig "github.com/rook/rook/pkg/daemon/ceph/config"
)

const (
	adminKeyringResourceName          = "rook-ceph-admin"
	crashCollectorKeyringResourceName = "rook-ceph-crash-collector"

	adminKeyringTemplate = `
[client.admin]
	key = %s
	caps mds = "allow *"
	caps mon = "allow *"
	caps osd = "allow *"
	caps mgr = "allow *"
`
)

// An AdminStore is a specialized derivative of the SecretStore helper for storing the Ceph cluster
// admin keyring as a Kubernetes secret.
type AdminStore struct {
	secretStore *SecretStore
}

// Admin returns the special Admin keyring store type.
func (s *SecretStore) Admin() *AdminStore {
	return &AdminStore{secretStore: s}
}

// CreateOrUpdate creates or updates the admin keyring secret with cluster information.
func (a *AdminStore) CreateOrUpdate(c *cephconfig.ClusterInfo) error {
	keyring := fmt.Sprintf(adminKeyringTemplate, c.AdminSecret)
	return a.secretStore.CreateOrUpdate(adminKeyringResourceName, keyring)
}
