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

	"github.com/rook/rook/pkg/operator/ceph/config/keyring"
	apps "k8s.io/api/apps/v1"
)

const (
	// don't list caps in keyring; allow OSD to get those from mons
	keyringTemplate = `[osd.%s]
key = %s
`
)

func (c *Cluster) generateKeyring(osdID int) (string, error) {
	deploymentName := fmt.Sprintf(osdAppNameFmt, osdID)
	osdIDStr := strconv.Itoa(osdID)

	user := fmt.Sprintf("osd.%s", osdIDStr)
	access := []string{"osd", "allow *", "mon", "allow profile osd"}

	s := keyring.GetSecretStore(c.context, c.Namespace, &c.ownerRef)

	key, err := s.GenerateKey(user, access)
	if err != nil {
		return "", err
	}

	keyring := fmt.Sprintf(keyringTemplate, osdIDStr, key)
	return keyring, s.CreateOrUpdate(deploymentName, keyring)
}

func (c *Cluster) associateKeyring(existingKeyring string, d *apps.Deployment) error {
	s := keyring.GetSecretStoreForDeployment(c.context, d)
	return s.CreateOrUpdate(d.GetName(), existingKeyring)
}
