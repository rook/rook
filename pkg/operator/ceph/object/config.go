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

	requiredKeys := []string{"OS_PROJECT_DOMAIN_NAME",
		"OS_USER_DOMAIN_NAME",
		"OS_PROJECT_DOMAIN_NAME",
		"OS_USER_DOMAIN_NAME",
		"OS_PROJECT_NAME",
		"OS_USERNAME",
		"OS_PASSWORD"}

	data := make(map[string]string)
	for key, value := range secret.Data {
		data[key] = string(value[:])
	}

	for _, key := range requiredKeys {
		if value, ok := data[key]; !ok || value == "" {
			return nil, errors.New(fmt.Sprintf("Missing or empty %s", key))
		}
	}

	if authType, ok := data["OS_AUTH_TYPE"]; ok && authType != "password" {
		return nil, errors.New(fmt.Sprintf("OS_AUTHTYPE %s is not supported. Only OS_AUTH_TYPE password is supported!", authType))
	}

	if apiVersion, ok := data["OS_IDENTITY_API_VERSION"]; ok && apiVersion != "3" {
		return nil, errors.New(fmt.Sprintf("OS_IDENTITY_API_VERSION %s is not supported! Only OS_IDENTITY_API_VERSION 3 is supported!", apiVersion))
	}

	if data["OS_PROJECT_DOMAIN_NAME"] != data["OS_USER_DOMAIN_NAME"] {
		return nil, errors.New("The user domain name does not match the project domain name.")
	}

	cfg["rgw_keystone_api_version"] = data["OS_IDENTITY_API_VERSION"]
	cfg["rgw_keystone_admin_domain"] = data["OS_PROJECT_DOMAIN_NAME"]
	cfg["rgw_keystone_admin_project"] = data["OS_PROJECT_NAME"]
	cfg["rgw_keystone_admin_user"] = data["OS_USERNAME"]
	cfg["rgw_keystone_admin_password"] = data["OS_PASSWORD"]

	return cfg, nil
}

func (c *clusterConfig) setFlagsMonConfigStore(rgwConfig *rgwConfig) error {

	monStore := cephconfig.GetMonStore(c.context, c.clusterInfo)
	who := generateCephXUser(rgwConfig.ResourceName)

	configOptions, err := c.generateMonConfigOptions(rgwConfig)
	if err != nil {
		return err
	}

	if err := monStore.SetAll(who, configOptions); err != nil {
		return errors.Wrapf(err, "failed to set all RGW configs on %q", who)
	}

	return nil
}

func (c *clusterConfig) generateMonConfigOptions(rgwConfig *rgwConfig) (map[string]string, error) {
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

	configOptions, err := configureKeystoneAuthentication(rgwConfig, configOptions)
	if err != nil {
		return configOptions, err
	}

	if s3 := rgwConfig.Protocols.S3; s3 != nil {
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

	for flag, val := range c.store.Spec.Gateway.RgwConfig {
		if currVal, ok := configOptions[flag]; ok {
			// RGW might break with some user-specified config overrides; log clearly to help triage
			logger.Infof("overriding object store %q RGW config option %q (current value %q) with user-specified rgwConfig %q",
				fmt.Sprintf("%s/%s", c.store.Namespace, c.store.Name), flag, currVal, val)
		}
		configOptions[flag] = val
	}

	return configOptions, nil
}

func configureKeystoneAuthentication(rgwConfig *rgwConfig, configOptions map[string]string) (map[string]string, error) {

	keystone := rgwConfig.Auth.Keystone
	if keystone == nil {
		logger.Debug("Authentication with keystone is disabled")
		return configOptions, nil
	}

	logger.Info("Configuring authentication with keystone")

	configOptions["rgw_keystone_url"] = keystone.Url
	configOptions["rgw_keystone_accepted_roles"] = strings.Join(keystone.AcceptedRoles, ",")
	if keystone.ImplicitTenants != "" {
		lc := strings.ToLower(string(keystone.ImplicitTenants))

		// only four values are valid here (swift, s3, true and false)
		//
		// https://docs.ceph.com/en/latest/radosgw/keystone/#integrating-with-openstack-keystone
		if lc != "true" &&
			lc != "false" &&
			lc != "swift" &&
			lc != "s3" {

			return nil, errors.New(fmt.Sprintf("ImplicitTenantSetting can only be 'swift', 's3', 'true' or 'false', not %q", string(keystone.ImplicitTenants)))

		}

		configOptions["rgw_keystone_implicit_tenants"] = lc

	}

	if keystone.TokenCacheSize != nil {
		configOptions["rgw_keystone_token_cache_size"] = fmt.Sprintf("%d", *keystone.TokenCacheSize)
	}

	if rgwConfig.KeystoneSecret == nil {
		return nil, errors.New("Cannot find keystone secret")
	}

	var err error
	configOptions, err = mapKeystoneSecretToConfig(configOptions, rgwConfig.KeystoneSecret)
	if err != nil {
		return nil, errors.Wrap(err, fmt.Sprintf("error mapping keystone secret %s to config", rgwConfig.KeystoneSecret.Name))
	}

	return configOptions, nil
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
