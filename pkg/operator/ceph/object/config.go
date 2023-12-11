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
		if !c.store.Spec.IsHostNetwork(c.clusterSpec) {
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

func mapKeystoneSecretToConfig(cfg map[string]string, secret *v1.Secret) (map[string]string, error) {

	data := make(map[string]string)
	for key, value := range secret.Data {

		logger.Debugf("keystone secret %s => %s", key, value)
		data[key] = string(value[:])

	}

	authType, ok := data["OS_AUTH_TYPE"]
	if ok {
		if authType != "password" {
			return nil, errors.New(fmt.Sprintf("OS_AUTHTYPE %s is not supported. Only OS_AUTH_TYPE password is supported!", authType))
		}
	}

	apiVersion, ok := data["OS_IDENTITY_API_VERSION"]
	if ok {
		if apiVersion != "3" {
			return nil, errors.New(fmt.Sprintf("OS_IDENTITY_API_VERSION %s is not supported! Only OS_IDENTITY_API_VERSION 3 is supported!", apiVersion))
		}
	}

	projectDomain, ok := data["OS_PROJECT_DOMAIN_NAME"]
	if !ok {
		return nil, errors.New("Missing OS_PROJECT_DOMAIN_NAME")
	}

	userDomain, ok := data["OS_USER_DOMAIN_NAME"]
	if !ok {
		return nil, errors.New("Missing OS_USER_DOMAIN_NAME")
	}

	if projectDomain != userDomain {
		return nil, errors.New("The user domain name does not match the project domain name.")
	}

	project, ok := data["OS_PROJECT_NAME"]
	if !ok {
		return nil, errors.New("No OS_PROJECT_NAME set.")
	}

	username, ok := data["OS_USERNAME"]
	if !ok {
		return nil, errors.New("No OS_USERNAME set.")
	}

	password, ok := data["OS_PASSWORD"]
	if !ok {
		return nil, errors.New("No OS_PASSWORD set.")
	}

	cfg["rgw_keystone_admin_domain"] = userDomain
	cfg["rgw_keystone_admin_project"] = project
	cfg["rgw_keystone_admin_user"] = username
	cfg["rgw_keystone_admin_password"] = password

	return cfg, nil
}

func (c *clusterConfig) setFlagsMonConfigStore(rgwConfig *rgwConfig) error {
	var err error

	monStore := cephconfig.GetMonStore(c.context, c.clusterInfo)
	who := generateCephXUser(rgwConfig.ResourceName)
	configOptions := make(map[string]string)

	configOptions["rgw_run_sync_thread"] = "true"
	if c.store.Spec.Gateway.DisableMultisiteSyncTraffic {
		configOptions["rgw_run_sync_thread"] = "false"
	}

	configOptions["rgw_log_nonexistent_bucket"] = "true"
	configOptions["rgw_log_object_name_utc"] = "true"
	configOptions["rgw_enable_usage_log"] = "true"
	configOptions["rgw_zone"] = rgwConfig.Zone
	configOptions["rgw_zonegroup"] = rgwConfig.ZoneGroup

	if ks := rgwConfig.Auth.Keystone; ks != nil {

		logger.Info("Configuring Authentication with keystone")

		configOptions["rgw_keystone_url"] = ks.Url
		configOptions["rgw_keystone_accepted_roles"] = strings.Join(ks.AcceptedRoles, ",")
		if ks.ImplicitTenants != "" {
			// XXX: where do we validate this?
			configOptions["rgw_keystone_implicit_tenants"] = string(ks.ImplicitTenants)
		}
		if ks.TokenCacheSize != nil {
			configOptions["rgw_keystone_token_cache_size"] = fmt.Sprintf("%d", *ks.TokenCacheSize)
		}
		if rgwConfig.KeystoneSecret == nil {
			return errors.New("Cannot find keystone secret")
		}

		configOptions, err = mapKeystoneSecretToConfig(configOptions, rgwConfig.KeystoneSecret)
		if err != nil {
			logger.Infof("error mapping keystone secret %s to config: %s", rgwConfig.KeystoneSecret.Name, err)
			return err
		}
	} else {
		logger.Info("Authentication with keystone disabled")
	}

	s3disabled := false
	if s3 := rgwConfig.Protocols.S3; s3 != nil {
		if s3.Enabled != nil && !*s3.Enabled {
			s3disabled = true
		}

		if s3.AuthUseKeystone != nil {
			configOptions["rgw_s3_auth_use_keystone"] = fmt.Sprintf("%t", *s3.AuthUseKeystone)
		}
	}

	if swift := rgwConfig.Protocols.Swift; swift != nil {
		if swift.AccountInUrl != nil {
			configOptions["rgw_swift_account_in_url"] = fmt.Sprintf("%t", *swift.AccountInUrl)
		}
		if swift.UrlPrefix != nil {
			configOptions["rgw_swift_url_prefix"] = *swift.UrlPrefix
		}
		if swift.VersioningEnabled != nil {
			configOptions["rgw_swift_versioning_enabled"] = fmt.Sprintf("%t", *swift.VersioningEnabled)
		}
	}

	if s3disabled {
		// XXX: how to handle enabled APIs? We only configure s3 and
		// swift in the resource, `admin` is required for the operator to
		// work, `swift_auth` is required to access swift without keystone
		// – not sure about the additional APIs

		// Swift was enabled so far already by default, so perhaps better
		// not change that if someon relies on it.

		configOptions["rgw_enabled_apis"] = "s3website, swift, swift_auth, admin, sts, iam, notifications"
	}

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
