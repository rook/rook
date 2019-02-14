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

package object

import (
	"fmt"
	"path"
	"strconv"

	cephconfig "github.com/rook/rook/pkg/operator/ceph/config"
	"github.com/rook/rook/pkg/operator/ceph/config/keyring"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	keyringTemplate = `
[client.radosgw.gateway]
key = %s
caps mon = "allow rw"
caps osd = "allow rwx"
`

	certVolumeName = "rook-ceph-rgw-cert"
	certDir        = "/etc/ceph/private"
	certKeyName    = "cert"
	certFilename   = "rgw-cert.pem"
)

// TODO: these should be set in the mon's central kv store for mimic+
func (c *clusterConfig) defaultSettings() *cephconfig.Config {
	s := cephconfig.NewConfig()
	s.Section("global").
		Set("rgw log nonexistent bucket", "true").
		Set("rgw intent log object name utc", "true").
		Set("rgw enable usage log", "true").
		Set("rgw frontends", fmt.Sprintf("civetweb port=%s", c.portString())).
		Set("rgw zone", c.store.Name).
		Set("rgw zonegroup", c.store.Name)
	return s
}

func (c *clusterConfig) portString() string {
	var portString string
	port := c.store.Spec.Gateway.Port
	if port != 0 {
		portString = strconv.Itoa(int(port))
	}
	if c.store.Spec.Gateway.SecurePort != 0 && c.store.Spec.Gateway.SSLCertificateRef != "" {
		var separator string
		if port != 0 {
			separator = "+"
		}
		certPath := path.Join(certDir, certFilename)
		// with ssl enabled, the port number must end with the letter s.
		// e.g., "443s ssl_certificate=/etc/ceph/private/keyandcert.pem"
		portString = fmt.Sprintf("%s%s%ds ssl_certificate=%s",
			portString, separator, c.store.Spec.Gateway.SecurePort, certPath)
	}
	return portString
}

func (c *clusterConfig) generateKeyring(replicationControllerOwnerRef *metav1.OwnerReference) error {
	user := "client.radosgw.gateway"
	/* TODO: this says `osd allow rwx` while template says `osd allow *`; which is correct? */
	access := []string{"osd", "allow rwx", "mon", "allow rw"}
	s := keyring.GetSecretStore(c.context, c.store.Namespace, replicationControllerOwnerRef)

	key, err := s.GenerateKey(c.instanceName(), user, access)
	if err != nil {
		return err
	}

	// Delete legacy key store for upgrade from Rook v0.9.x to v1.0.x
	err = c.context.Clientset.CoreV1().Secrets(c.store.Namespace).Delete(c.instanceName(), &metav1.DeleteOptions{})
	if err != nil {
		if errors.IsNotFound(err) {
			logger.Debugf("legacy rgw key %s is already removed", c.instanceName())
		} else {
			logger.Warningf("legacy rgw key %s could not be removed. %+v", c.instanceName(), err)
		}
	}

	keyring := fmt.Sprintf(keyringTemplate, key)
	return s.CreateOrUpdate(c.instanceName(), keyring)
}
