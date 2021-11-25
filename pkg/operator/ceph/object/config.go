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
	"time"

	"github.com/pkg/errors"
	cephconfig "github.com/rook/rook/pkg/operator/ceph/config"
	"github.com/rook/rook/pkg/operator/ceph/config/keyring"
	v1 "k8s.io/api/core/v1"
)

const (
	keyringTemplate = `
[%s]
key = %s
caps mon = "allow rw"
caps osd = "allow rwx"
`

	caBundleVolumeName              = "rook-ceph-custom-ca-bundle"
	caBundleUpdatedVolumeName       = "rook-ceph-ca-bundle-updated"
	caBundleTrustedDir              = "/etc/pki/ca-trust/"
	caBundleSourceCustomDir         = caBundleTrustedDir + "source/anchors/"
	caBundleExtractedDir            = caBundleTrustedDir + "extracted/"
	caBundleKeyName                 = "cabundle"
	caBundleFileName                = "custom-ca-bundle.crt"
	certVolumeName                  = "rook-ceph-rgw-cert"
	certDir                         = "/etc/ceph/private"
	certKeyName                     = "cert"
	certFilename                    = "rgw-cert.pem"
	certKeyFileName                 = "rgw-key.pem"
	rgwPortInternalPort       int32 = 8080
	ServiceServingCertCAFile        = "/var/run/secrets/kubernetes.io/serviceaccount/service-ca.crt"
	HttpTimeOut                     = time.Second * 15
	rgwVaultVolumeName              = "rgw-vault-volume"
	rgwVaultDirName                 = "/etc/vault/rgw/"
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
	if c.store.Spec.IsTLSEnabled() {
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
		secretType, _ := c.rgwTLSSecretType(c.store.Spec.Gateway.SSLCertificateRef)
		if c.store.Spec.GetServiceServingCert() != "" || secretType == v1.SecretTypeTLS {
			privateKey := path.Join(certDir, certKeyFileName)
			portString = fmt.Sprintf("%s ssl_private_key=%s", portString, privateKey)
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
	s := keyring.GetSecretStore(c.context, c.clusterInfo, c.ownerInfo)

	key, err := s.GenerateKey(user, access)
	if err != nil {
		return "", err
	}

	keyring := fmt.Sprintf(keyringTemplate, user, key)
	return keyring, s.CreateOrUpdate(rgwConfig.ResourceName, keyring)
}

func (c *clusterConfig) setDefaultFlagsMonConfigStore(rgwConfig *rgwConfig) error {
	monStore := cephconfig.GetMonStore(c.context, c.clusterInfo)
	who := generateCephXUser(rgwConfig.ResourceName)
	configOptions := make(map[string]string)

	configOptions["rgw_log_nonexistent_bucket"] = "true"
	configOptions["rgw_log_object_name_utc"] = "true"
	configOptions["rgw_enable_usage_log"] = "true"
	configOptions["rgw_zone"] = rgwConfig.Zone
	configOptions["rgw_zonegroup"] = rgwConfig.ZoneGroup

	for flag, val := range configOptions {
		err := monStore.Set(who, flag, val)
		if err != nil {
			return errors.Wrapf(err, "failed to set %q to %q on %q", flag, val, who)
		}
	}

	return nil
}

func (c *clusterConfig) deleteFlagsMonConfigStore(rgwName string) error {
	monStore := cephconfig.GetMonStore(c.context, c.clusterInfo)
	who := generateCephXUser(rgwName)
	err := monStore.DeleteDaemon(who)
	if err != nil {
		return errors.Wrapf(err, "failed to delete rgw config for %q in mon configuration database", who)
	}

	logger.Infof("successfully deleted rgw config for %q in mon configuration database", who)
	return nil
}
