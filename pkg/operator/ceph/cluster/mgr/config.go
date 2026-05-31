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

	"github.com/pkg/errors"
	"github.com/rook/rook/pkg/operator/ceph/config"
	"github.com/rook/rook/pkg/operator/ceph/config/keyring"
	"github.com/rook/rook/pkg/util/log"
	kerrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	// Ports below 1024 require extra privileges for binding inside a pod
	minPortWithoutPrivileges = 1024
	keyringTemplate          = `
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

// dashboardInternalPort gets the port to be used by the service targetPort
// and the container ports on the mgr pod.
// If the port is greater than 1024, for backward compatibility the port and
// targetPort should be the same value. If the port is less than 1024,
// the internal port must use a higher port number. In that case, the internal
// port will be the default port numbers and only the public port will be
// the desired port in the cluster CR.
func (c *Cluster) dashboardInternalPort() int {
	port := c.dashboardPublicPort()
	if port <= minPortWithoutPrivileges {
		// If the port is less than the allowed range, set it back to the default
		return c.dashboardDefaultPort()
	}
	return port
}

// dashboardPublicPort is the desired port to be exposed on the service
func (c *Cluster) dashboardPublicPort() int {
	if c.spec.Dashboard.Port == 0 {
		return c.dashboardDefaultPort()
	}
	// crd validates port >= 0
	return c.spec.Dashboard.Port
}

func (c *Cluster) dashboardDefaultPort() int {
	// default port for HTTP/HTTPS
	if c.spec.Dashboard.SSL {
		return dashboardPortHTTPS
	}
	return dashboardPortHTTP
}

func (c *Cluster) generateKeyring(m *mgrConfig) (string, error) {
	user := fmt.Sprintf("mgr.%s", m.DaemonID)
	access := []string{"mon", "allow profile mgr", "mds", "allow *", "osd", "allow *"}
	s := keyring.GetSecretStore(c.context, c.clusterInfo, c.clusterInfo.OwnerInfo)

	key, err := s.GenerateKey(user, access)
	if err != nil {
		return "", err
	}

	if c.shouldRotateCephxKeys {
		log.NamespacedInfo(c.clusterInfo.Namespace, logger, "rotating cephx key for mgr daemon %q in the namespace %q", m.ResourceName, c.clusterInfo.Namespace)
		newKey, err := s.RotateKey(user)
		if err != nil {
			return "", errors.Wrapf(err, "failed to rotate cephx key for mgr daemon %q", m.ResourceName)
		}
		key = newKey
	}

	// Delete legacy key store for upgrade from Rook v0.9.x to v1.0.x
	err = c.context.Clientset.CoreV1().Secrets(c.clusterInfo.Namespace).Delete(c.clusterInfo.Context, m.ResourceName, metav1.DeleteOptions{})
	if err != nil {
		if kerrors.IsNotFound(err) {
			log.NamespacedDebug(c.clusterInfo.Namespace, logger, "legacy mgr key %q is already removed", m.ResourceName)
		} else {
			log.NamespacedWarning(c.clusterInfo.Namespace, logger, "legacy mgr key %q could not be removed. %v", m.ResourceName, err)
		}
	}

	keyring := fmt.Sprintf(keyringTemplate, m.DaemonID, key)
	return s.CreateOrUpdate(m.ResourceName, keyring)
}
