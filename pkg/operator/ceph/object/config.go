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
	"strings"

	cephconfig "github.com/rook/rook/pkg/operator/ceph/config"
	"github.com/rook/rook/pkg/operator/ceph/config/keyring"
	cephver "github.com/rook/rook/pkg/operator/ceph/version"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	keyringTemplate = `
[%s]
key = %s
caps mon = "allow rw"
caps osd = "allow rwx"
`

	certVolumeName = "rook-ceph-rgw-cert"
	certDir        = "/etc/ceph/private"
	certKeyName    = "cert"
	certFilename   = "rgw-cert.pem"
)

var (
	rgwFrontendName = "civetweb"
)

func rgwFrontend(v cephver.CephVersion) string {
	if v.IsAtLeastNautilus() {
		rgwFrontendName = "beast"
	}

	return rgwFrontendName
}

// TODO: these should be set in the mon's central kv store for mimic+
func (c *clusterConfig) defaultSettings() *cephconfig.Config {
	s := cephconfig.NewConfig()
	s.Section("global").
		Set("rgw log nonexistent bucket", "true").
		Set("rgw intent log object name utc", "true").
		Set("rgw enable usage log", "true").
		Set("rgw frontends", fmt.Sprintf("%s port=%s", rgwFrontend(c.clusterInfo.CephVersion), c.portString())).
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

func generateCephXUser(name string) string {
	user := strings.TrimPrefix(name, AppName)
	return "client" + strings.Replace(user, "-", ".", -1)
}

func (c *clusterConfig) generateKeyring(replicationControllerOwnerRef *metav1.OwnerReference) error {
	user := generateCephXUser(replicationControllerOwnerRef.Name)
	/* TODO: this says `osd allow rwx` while template says `osd allow *`; which is correct? */
	access := []string{"osd", "allow rwx", "mon", "allow rw"}
	s := keyring.GetSecretStore(c.context, c.store.Namespace, replicationControllerOwnerRef)

	key, err := s.GenerateKey(user, access)
	if err != nil {
		return err
	}

	keyring := fmt.Sprintf(keyringTemplate, user, key)
	return s.CreateOrUpdate(replicationControllerOwnerRef.Name, keyring)
}
