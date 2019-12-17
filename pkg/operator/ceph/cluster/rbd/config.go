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

package rbd

import (
	"fmt"

	"github.com/rook/rook/pkg/operator/ceph/config"
	"github.com/rook/rook/pkg/operator/ceph/config/keyring"
	apps "k8s.io/api/apps/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	keyringTemplate = `
[client.rbd-mirror.%s]
	key = %s
	caps mon = "profile rbd-mirror"
	caps osd = "profile rbd"
`
)

// daemonConfig for a single rbd-mirror
type daemonConfig struct {
	ResourceName string              // the name rook gives to mirror resources in k8s metadata
	DaemonID     string              // the ID of the Ceph daemon ("a", "b", ...)
	DataPathMap  *config.DataPathMap // location to store data in container
}

func (m *Mirroring) generateKeyring(daemonConfig *daemonConfig) (string, error) {
	user := fullDaemonName(daemonConfig.DaemonID)
	access := []string{"mon", "profile rbd-mirror", "osd", "profile rbd"}
	s := keyring.GetSecretStore(m.context, m.Namespace, &m.ownerRef)

	key, err := s.GenerateKey(user, access)
	if err != nil {
		return "", err
	}

	// Delete legacy key store for upgrade from Rook v0.9.x to v1.0.x
	err = m.context.Clientset.CoreV1().Secrets(m.Namespace).Delete(daemonConfig.ResourceName, &metav1.DeleteOptions{})
	if err != nil {
		if errors.IsNotFound(err) {
			logger.Debugf("legacy rbd-mirror key %q is already removed", daemonConfig.ResourceName)
		} else {
			logger.Warningf("legacy rbd-mirror key %q could not be removed. %v", daemonConfig.ResourceName, err)
		}
	}

	keyring := fmt.Sprintf(keyringTemplate, daemonConfig.DaemonID, key)
	return keyring, s.CreateOrUpdate(daemonConfig.ResourceName, keyring)
}

func (m *Mirroring) associateKeyring(existingKeyring string, d *apps.Deployment) error {
	s := keyring.GetSecretStoreForDeployment(m.context, d)
	return s.CreateOrUpdate(d.GetName(), existingKeyring)
}

func fullDaemonName(daemonID string) string {
	return fmt.Sprintf("client.rbd-mirror.%s", daemonID)
}
