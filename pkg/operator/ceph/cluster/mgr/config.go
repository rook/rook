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

package mgr

import (
	"fmt"

	"github.com/rook/rook/pkg/operator/ceph/config"
	"github.com/rook/rook/pkg/operator/ceph/config/keyring"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	keyringTemplate = `
[mgr.%s]
	key = %s
	caps mon = "allow profile mgr"
	caps mds = "allow *"
	caps osd = "allow *"
`
)

// mgrConfig for a single mgr
type mgrConfig struct {
	ResourceName string              // the name rook gives to mgr resources in k8s metadata
	DaemonID     string              // the ID of the Ceph daemon ("a", "b", ...)
	DataPathMap  *config.DataPathMap // location to store data in container
}

func (c *Cluster) dashboardPort() int {
	if c.spec.Dashboard.Port == 0 {
		// default port for HTTP/HTTPS
		if c.spec.Dashboard.SSL {
			return dashboardPortHTTPS
		} else {
			return dashboardPortHTTP
		}
	}
	// crd validates port >= 0
	return c.spec.Dashboard.Port
}

func (c *Cluster) generateKeyring(m *mgrConfig) (string, error) {
	user := fmt.Sprintf("mgr.%s", m.DaemonID)
	access := []string{"mon", "allow profile mgr", "mds", "allow *", "osd", "allow *"}
	s := keyring.GetSecretStore(c.context, c.clusterInfo, c.clusterInfo.OwnerInfo)

	key, err := s.GenerateKey(user, access)
	if err != nil {
		return "", err
	}

	// Delete legacy key store for upgrade from Rook v0.9.x to v1.0.x
	err = c.context.Clientset.CoreV1().Secrets(c.clusterInfo.Namespace).Delete(c.clusterInfo.Context, m.ResourceName, metav1.DeleteOptions{})
	if err != nil {
		if errors.IsNotFound(err) {
			logger.Debugf("legacy mgr key %q is already removed", m.ResourceName)
		} else {
			logger.Warningf("legacy mgr key %q could not be removed. %v", m.ResourceName, err)
		}
	}

	keyring := fmt.Sprintf(keyringTemplate, m.DaemonID, key)
	return keyring, s.CreateOrUpdate(m.ResourceName, keyring)
}
