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

package mds

import (
	"fmt"

	apps "k8s.io/api/apps/v1"

	"github.com/rook/rook/pkg/operator/ceph/config/keyring"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	keyringTemplate = `
[mds.%s]
key = %s
caps mon = "allow profile mds"
caps osd = "allow *"
caps mds = "allow"
`
)

func (c *Cluster) generateKeyring(m *mdsConfig) (string, error) {
	user := fmt.Sprintf("mds.%s", m.DaemonID)
	access := []string{"osd", "allow *", "mds", "allow", "mon", "allow profile mds"}

	// At present
	s := keyring.GetSecretStore(c.context, c.fs.Namespace, &c.ownerRef)

	key, err := s.GenerateKey(user, access)
	if err != nil {
		return "", err
	}

	// Delete legacy key store for upgrade from Rook v0.9.x to v1.0.x
	err = c.context.Clientset.CoreV1().Secrets(c.fs.Namespace).Delete(m.ResourceName, &metav1.DeleteOptions{})
	if err != nil {
		if errors.IsNotFound(err) {
			logger.Debugf("legacy mds key %s is already removed", m.ResourceName)
		} else {
			logger.Warningf("legacy mds key %s could not be removed: %+v", m.ResourceName, err)
		}
	}

	keyring := fmt.Sprintf(keyringTemplate, m.DaemonID, key)
	return keyring, s.CreateOrUpdate(m.ResourceName, keyring)
}

func (c *Cluster) associateKeyring(existingKeyring string, d *apps.Deployment) error {
	s := keyring.GetSecretStoreForDeployment(c.context, d)
	return s.CreateOrUpdate(d.GetName(), existingKeyring)
}
