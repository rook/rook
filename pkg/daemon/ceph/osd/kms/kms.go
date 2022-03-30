/*
Copyright 2020 The Rook Authors. All rights reserved.

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

package kms

import (
	"context"
	"os"
	"strings"

	kp "github.com/IBM/keyprotect-go-client"
	"github.com/coreos/pkg/capnslog"
	"github.com/hashicorp/vault/api"
	"github.com/libopenstorage/secrets"
	"github.com/libopenstorage/secrets/vault"
	"github.com/pkg/errors"
	cephv1 "github.com/rook/rook/pkg/apis/ceph.rook.io/v1"
	"github.com/rook/rook/pkg/clusterd"
	cephclient "github.com/rook/rook/pkg/daemon/ceph/client"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	// Provider is the config name for the KMS provider type
	Provider = "KMS_PROVIDER"
)

var (
	logger                        = capnslog.NewPackageLogger("github.com/rook/rook", "op-kms")
	kmsMandatoryConnectionDetails = []string{Provider}
)

// Config is the generic configuration for the KMS
type Config struct {
	Provider    string
	context     *clusterd.Context
	clusterSpec *cephv1.ClusterSpec
	ClusterInfo *cephclient.ClusterInfo
}

// NewConfig returns the selected KMS
func NewConfig(context *clusterd.Context, clusterSpec *cephv1.ClusterSpec, clusterInfo *cephclient.ClusterInfo) *Config {
	config := &Config{
		context:     context,
		ClusterInfo: clusterInfo,
		clusterSpec: clusterSpec,
	}

	Provider := clusterSpec.Security.KeyManagementService.ConnectionDetails[Provider]
	switch Provider {
	case "":
		config.Provider = secrets.TypeK8s
	case secrets.TypeVault:
		config.Provider = secrets.TypeVault
	case TypeIBM:
		config.Provider = TypeIBM
	default:
		logger.Errorf("unsupported kms type %q", Provider)
	}

	return config
}

// PutSecret writes an encrypted key in a KMS
func (c *Config) PutSecret(secretName, secretValue string) error {
	// If Kubernetes Secret KMS is selected (default)
	if c.IsK8s() {
		// Store the secret in Kubernetes Secrets
		err := c.storeSecretInKubernetes(secretName, secretValue)
		if err != nil {
			return errors.Wrap(err, "failed to store secret in kubernetes secret")
		}
	}
	if c.IsVault() {
		// Store the secret in Vault
		v, err := InitVault(c.ClusterInfo.Context, c.context, c.ClusterInfo.Namespace, c.clusterSpec.Security.KeyManagementService.ConnectionDetails)
		if err != nil {
			return errors.Wrap(err, "failed to init vault kms")
		}
		k := buildVaultKeyContext(c.clusterSpec.Security.KeyManagementService.ConnectionDetails)
		err = put(v, GenerateOSDEncryptionSecretName(secretName), secretValue, k)
		if err != nil {
			return errors.Wrap(err, "failed to put secret in vault")
		}
	}
	if c.IsIBMKeyProtect() {
		kpClient, err := InitKeyProtect(c.clusterSpec.Security.KeyManagementService.ConnectionDetails)
		if err != nil {
			return errors.Wrap(err, "failed to init ibm key protect")
		}

		// Create the key is not present
		keyAlias := []string{secretName}
		_, err = kpClient.CreateImportedKeyWithAliases(c.ClusterInfo.Context, secretName, nil, secretValue, "", "", true, keyAlias)
		if err != nil {
			if strings.Contains(err.Error(), "KEY_ALIAS_NOT_UNIQUE_ERR") {
				logger.Debugf("key %q already exists. %v", secretName, err)
				return nil
			}

			return errors.Wrap(err, "failed to put secret in ibm key protect")
		}
	}

	return nil
}

// GetSecret returns an encrypted key from a KMS
func (c *Config) GetSecret(secretName string) (string, error) {
	var value string
	if c.IsVault() {
		// Store the secret in Vault
		v, err := InitVault(c.ClusterInfo.Context, c.context, c.ClusterInfo.Namespace, c.clusterSpec.Security.KeyManagementService.ConnectionDetails)
		if err != nil {
			return "", errors.Wrap(err, "failed to init vault")
		}

		k := buildVaultKeyContext(c.clusterSpec.Security.KeyManagementService.ConnectionDetails)
		value, err = get(v, GenerateOSDEncryptionSecretName(secretName), k)
		if err != nil {
			return "", errors.Wrap(err, "failed to get secret from vault")
		}
	}
	if c.IsIBMKeyProtect() {
		kpClient, err := InitKeyProtect(c.clusterSpec.Security.KeyManagementService.ConnectionDetails)
		if err != nil {
			return "", errors.Wrap(err, "failed to init ibm key protect")
		}
		keyObject, err := kpClient.GetKey(c.ClusterInfo.Context, secretName)
		if err != nil {
			return "", errors.Wrap(err, "failed to get secret from ibm key protect")
		}
		value = string(keyObject.Payload)
	}

	return value, nil
}

// DeleteSecret deletes an encrypted key from a KMS
func (c *Config) DeleteSecret(secretName string) error {
	if c.IsVault() {
		// Store the secret in Vault
		v, err := InitVault(c.ClusterInfo.Context, c.context, c.ClusterInfo.Namespace, c.clusterSpec.Security.KeyManagementService.ConnectionDetails)
		if err != nil {
			return errors.Wrap(err, "failed to delete secret in vault")
		}

		k := buildVaultKeyContext(c.clusterSpec.Security.KeyManagementService.ConnectionDetails)

		// Force removal of all the versions of the secret on K/V version 2
		k[secrets.DestroySecret] = "true"

		err = deleteSecret(v, GenerateOSDEncryptionSecretName(secretName), k)
		if err != nil {
			return errors.Wrap(err, "failed to delete secret in vault")
		}
	}
	if c.IsIBMKeyProtect() {
		kpClient, err := InitKeyProtect(c.clusterSpec.Security.KeyManagementService.ConnectionDetails)
		if err != nil {
			return errors.Wrap(err, "failed to init ibm key protect")
		}

		// We use context.TODO() since the clusterInfo context has been cancelled by the CephCluster's
		// deletion event
		ctx := context.TODO()

		// Fetch the key to get the ID
		key, err := kpClient.GetKey(ctx, secretName)
		if err != nil {
			return errors.Wrap(err, "failed to get secret in ibm key protect")
		}

		// DeleteKey does not support deleting secret with the alias name so we must use the ID
		// After you delete a key, the key transitions to the Destroyed state. Any data encrypted by
		// keys in this state is no longer accessible. Metadata that is associated with the key,
		// such as the key's deletion date, is kept in the Key Protect database. Destroyed keys can
		// be recovered after up to 30 days or their expiration date, whichever is sooner. After 30
		// days, keys can no longer be recovered, and become eligible to be purged after 90 days, a
		// process that shreds the key material and makes its metadata inaccessible.
		_, err = kpClient.DeleteKey(ctx, key.ID, kp.ReturnRepresentation, []kp.CallOpt{kp.ForceOpt{Force: true}}...)
		if err != nil {
			return errors.Wrap(err, "failed to delete secret in ibm key protect")
		}
	}

	return nil
}

// GetParam returns the value of the KMS config option
func GetParam(kmsConfig map[string]string, param string) string {
	if val, ok := kmsConfig[param]; ok && val != "" {
		return strings.TrimSpace(val)
	}
	return ""
}

// ValidateConnectionDetails validates mandatory KMS connection details
func ValidateConnectionDetails(ctx context.Context, clusterdContext *clusterd.Context, securitySpec *cephv1.SecuritySpec, ns string) error {
	// Lookup mandatory connection details
	for _, config := range kmsMandatoryConnectionDetails {
		if GetParam(securitySpec.KeyManagementService.ConnectionDetails, config) == "" {
			return errors.Errorf("failed to validate kms config %q. cannot be empty", config)
		}
	}

	// A token must be specified if token-auth is used
	if !securitySpec.KeyManagementService.IsK8sAuthEnabled() && securitySpec.KeyManagementService.TokenSecretName == "" {
		if !securitySpec.KeyManagementService.IsTokenAuthEnabled() {
			return errors.New("failed to validate kms configuration (missing token in spec)")
		}
	}

	// KMS provider must be specified
	provider := GetParam(securitySpec.KeyManagementService.ConnectionDetails, Provider)

	// Validate potential token Secret presence
	if securitySpec.KeyManagementService.IsTokenAuthEnabled() {
		kmsToken, err := clusterdContext.Clientset.CoreV1().Secrets(ns).Get(ctx, securitySpec.KeyManagementService.TokenSecretName, metav1.GetOptions{})
		if err != nil {
			return errors.Wrapf(err, "failed to fetch kms token secret %q", securitySpec.KeyManagementService.TokenSecretName)
		}

		switch provider {
		case secrets.TypeVault:
			// Check for empty token
			token, ok := kmsToken.Data[KMSTokenSecretNameKey]
			if !ok || len(token) == 0 {
				return errors.Errorf("failed to read k8s kms secret %q key %q (not found or empty)", KMSTokenSecretNameKey, securitySpec.KeyManagementService.TokenSecretName)
			}

			// Set the env variable
			err = os.Setenv(api.EnvVaultToken, string(token))
			if err != nil {
				return errors.Wrap(err, "failed to set vault kms token to an env var")
			}

		case TypeIBM:
			for _, config := range kmsIBMKeyProtectMandatoryTokenDetails {
				v, ok := kmsToken.Data[config]
				if !ok || len(v) == 0 {
					return errors.Errorf("failed to read k8s kms secret %q key %q (not found or empty)", config, securitySpec.KeyManagementService.TokenSecretName)
				}
				// Append the token secret details to the connection details
				securitySpec.KeyManagementService.ConnectionDetails[config] = strings.TrimSuffix(strings.TrimSpace(string(v)), "\n")
			}
		}
	}

	// Validate KMS provider connection details for each provider
	switch provider {
	case secrets.TypeVault:
		err := validateVaultConnectionDetails(ctx, clusterdContext, ns, securitySpec.KeyManagementService.ConnectionDetails)
		if err != nil {
			return errors.Wrap(err, "failed to validate vault connection details")
		}

		secretEngine := securitySpec.KeyManagementService.ConnectionDetails[VaultSecretEngineKey]
		switch secretEngine {
		case VaultKVSecretEngineKey:
			// Append Backend Version if not already present
			if GetParam(securitySpec.KeyManagementService.ConnectionDetails, vault.VaultBackendKey) == "" {
				backendVersion, err := BackendVersion(ctx, clusterdContext, ns, securitySpec.KeyManagementService.ConnectionDetails)
				if err != nil {
					return errors.Wrap(err, "failed to get backend version")
				}
				securitySpec.KeyManagementService.ConnectionDetails[vault.VaultBackendKey] = backendVersion
			}
		}

	case TypeIBM:
		for _, config := range kmsIBMKeyProtectMandatoryConnectionDetails {
			if GetParam(securitySpec.KeyManagementService.ConnectionDetails, config) == "" {
				return errors.Errorf("failed to validate kms config %q. cannot be empty", config)
			}
		}

	default:
		return errors.Errorf("failed to validate kms provider connection details (provider %q not supported)", provider)
	}

	return nil
}

// SetTokenToEnvVar sets a KMS token as an env variable
func SetTokenToEnvVar(ctx context.Context, clusterdContext *clusterd.Context, tokenSecretName, provider, namespace string) error {
	// Get the secret containing the kms token
	kmsToken, err := clusterdContext.Clientset.CoreV1().Secrets(namespace).Get(ctx, tokenSecretName, metav1.GetOptions{})
	if err != nil {
		return errors.Wrapf(err, "failed to fetch kms token secret %q", tokenSecretName)
	}

	// We set the token as an env variable, the secrets lib will pick it up
	var key, value string
	switch provider {
	case secrets.TypeVault:
		key = api.EnvVaultToken
		value = string(kmsToken.Data[KMSTokenSecretNameKey])
	default:
		logger.Debugf("unknown provider %q return nil", provider)
		return nil
	}

	// Set the env variable
	err = os.Setenv(key, value)
	if err != nil {
		return errors.Wrap(err, "failed to set kms token to an env var")
	}

	return nil
}
