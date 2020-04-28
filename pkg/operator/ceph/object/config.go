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

	"github.com/pkg/errors"
	cephconfig "github.com/rook/rook/pkg/operator/ceph/config"
	"github.com/rook/rook/pkg/operator/ceph/config/keyring"
)

const (
	keyringTemplate = `
[%s]
key = %s
caps mon = "allow rw"
caps osd = "allow rwx"
`

	certVolumeName            = "rook-ceph-rgw-cert"
	certDir                   = "/etc/ceph/private"
	certKeyName               = "cert"
	certFilename              = "rgw-cert.pem"
	rgwPortInternalPort int32 = 8080
)

var (
	rgwFrontendName = "beast"
)

func (c *clusterConfig) portString() string {
	var portString string

	port := c.store.Spec.Gateway.Port
	if port != 0 {
		if !c.clusterSpec.Network.IsHost() {
			port = rgwPortInternalPort
		}
		portString = fmt.Sprintf("port=%s", strconv.Itoa(int(port)))
	}
	if c.store.Spec.Gateway.SecurePort != 0 && c.store.Spec.Gateway.SSLCertificateRef != "" {
		certPath := path.Join(certDir, certFilename)
		// This is the beast backend
		// Config is: http://docs.ceph.com/docs/master/radosgw/frontends/#id3
		if port != 0 {
			portString = fmt.Sprintf("%s ssl_port=%d ssl_certificate=%s",
				portString, c.store.Spec.Gateway.SecurePort, certPath)
		} else {
			portString = fmt.Sprintf("ssl_port=%d ssl_certificate=%s",
				c.store.Spec.Gateway.SecurePort, certPath)
		}
	}
	return portString
}

func generateCephXUser(name string) string {
	user := strings.TrimPrefix(name, AppName)
	return "client.rgw" + strings.Replace(user, "-", ".", -1)
}

func (c *clusterConfig) generateKeyring(rgwConfig *rgwConfig) (string, error) {
	user := generateCephXUser(rgwConfig.ResourceName)
	/* TODO: this says `osd allow rwx` while template says `osd allow *`; which is correct? */
	access := []string{"osd", "allow rwx", "mon", "allow rw"}
	s := keyring.GetSecretStore(c.context, c.store.Namespace, c.ownerRef)

	key, err := s.GenerateKey(user, access)
	if err != nil {
		return "", err
	}

	keyring := fmt.Sprintf(keyringTemplate, user, key)
	return keyring, s.CreateOrUpdate(rgwConfig.ResourceName, keyring)
}

func (c *clusterConfig) setDefaultFlagsMonConfigStore(rgwName string) error {
	monStore := cephconfig.GetMonStore(c.context, c.store.Namespace)
	who := generateCephXUser(rgwName)
	configOptions := make(map[string]string)

	configOptions["rgw_log_nonexistent_bucket"] = "true"
	configOptions["rgw_log_object_name_utc"] = "true"
	configOptions["rgw_enable_usage_log"] = "true"
	configOptions["rgw_zone"] = c.store.Name
	configOptions["rgw_zonegroup"] = c.store.Name

	for flag, val := range configOptions {
		err := monStore.Set(who, flag, val)
		if err != nil {
			return errors.Wrapf(err, "failed to set %q to %q on %q", flag, val, who)
		}
	}

	return nil
}

func (c *clusterConfig) deleteFlagsMonConfigStore(rgwName string) error {
	monStore := cephconfig.GetMonStore(c.context, c.store.Namespace)
	who := generateCephXUser(rgwName)
	err := monStore.DeleteDaemon(who)
	if err != nil {
		return errors.Wrapf(err, "failed to delete rgw config for %q in mon configuration database", who)
	}

	logger.Infof("successfully deleted rgw config for %q in mon configuration database", who)
	return nil
}
