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
	clusterInfo *cephclient.ClusterInfo
}

// NewConfig returns the selected KMS
func NewConfig(context *clusterd.Context, clusterSpec *cephv1.ClusterSpec, clusterInfo *cephclient.ClusterInfo) *Config {
	config := &Config{
		context:     context,
		clusterInfo: clusterInfo,
		clusterSpec: clusterSpec,
	}

	Provider := clusterSpec.Security.KeyManagementService.ConnectionDetails[Provider]
	switch Provider {
	case "":
		config.Provider = secrets.TypeK8s
	case secrets.TypeVault:
		config.Provider = secrets.TypeVault
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
		v, err := InitVault(c.context, c.clusterInfo.Namespace, c.clusterSpec.Security.KeyManagementService.ConnectionDetails)
		if err != nil {
			return errors.Wrap(err, "failed to init vault kms")
		}
		k := buildKeyContext(c.clusterSpec.Security.KeyManagementService.ConnectionDetails)
		err = put(v, GenerateOSDEncryptionSecretName(secretName), secretValue, k)
		if err != nil {
			return errors.Wrap(err, "failed to put secret in vault")
		}
	}

	return nil
}

// GetSecret returns an encrypted key from a KMS
func (c *Config) GetSecret(secretName string) (string, error) {
	var value string
	if c.IsVault() {
		// Store the secret in Vault
		v, err := InitVault(c.context, c.clusterInfo.Namespace, c.clusterSpec.Security.KeyManagementService.ConnectionDetails)
		if err != nil {
			return "", errors.Wrap(err, "failed to get secret in vault")
		}

		k := buildKeyContext(c.clusterSpec.Security.KeyManagementService.ConnectionDetails)
		value, err = get(v, GenerateOSDEncryptionSecretName(secretName), k)
		if err != nil {
			return "", errors.Wrap(err, "failed to get secret in vault")
		}
	}

	return value, nil
}

// DeleteSecret deletes an encrypted key from a KMS
func (c *Config) DeleteSecret(secretName string) error {
	if c.IsVault() {
		// Store the secret in Vault
		v, err := InitVault(c.context, c.clusterInfo.Namespace, c.clusterSpec.Security.KeyManagementService.ConnectionDetails)
		if err != nil {
			return errors.Wrap(err, "failed to delete secret in vault")
		}

		k := buildKeyContext(c.clusterSpec.Security.KeyManagementService.ConnectionDetails)

		// Force removal of all the versions of the secret on K/V version 2
		k[secrets.DestroySecret] = "true"

		err = delete(v, GenerateOSDEncryptionSecretName(secretName), k)
		if err != nil {
			return errors.Wrap(err, "failed to delete secret in vault")
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
func ValidateConnectionDetails(clusterdContext *clusterd.Context, securitySpec *cephv1.SecuritySpec, ns string) error {
	ctx := context.TODO()
	// A token must be specified
	if !securitySpec.KeyManagementService.IsTokenAuthEnabled() {
		return errors.New("failed to validate kms configuration (missing token in spec)")
	}

	// KMS provider must be specified
	provider := GetParam(securitySpec.KeyManagementService.ConnectionDetails, Provider)

	// Validate potential token Secret presence
	if securitySpec.KeyManagementService.IsTokenAuthEnabled() {
		kmsToken, err := clusterdContext.Clientset.CoreV1().Secrets(ns).Get(ctx, securitySpec.KeyManagementService.TokenSecretName, metav1.GetOptions{})
		if err != nil {
			return errors.Wrapf(err, "failed to fetch kms token secret %q", securitySpec.KeyManagementService.TokenSecretName)
		}

		// Check for empty token
		token, ok := kmsToken.Data[KMSTokenSecretNameKey]
		if !ok || len(token) == 0 {
			return errors.Errorf("failed to read k8s kms secret %q key %q (not found or empty)", KMSTokenSecretNameKey, securitySpec.KeyManagementService.TokenSecretName)
		}

		switch provider {
		case "vault":
			// Set the env variable
			err = os.Setenv(api.EnvVaultToken, string(token))
			if err != nil {
				return errors.Wrap(err, "failed to set vault kms token to an env var")
			}
		}
	}

	// Lookup mandatory connection details
	for _, config := range kmsMandatoryConnectionDetails {
		if GetParam(securitySpec.KeyManagementService.ConnectionDetails, config) == "" {
			return errors.Errorf("failed to validate kms config %q. cannot be empty", config)
		}
	}

	// Validate KMS provider connection details
	switch provider {
	case "vault":
		err := validateVaultConnectionDetails(clusterdContext, ns, securitySpec.KeyManagementService.ConnectionDetails)
		if err != nil {
			return errors.Wrap(err, "failed to validate vault connection details")
		}

		secretEngine := securitySpec.KeyManagementService.ConnectionDetails[VaultSecretEngineKey]
		switch secretEngine {
		case VaultKVSecretEngineKey:
			// Append Backend Version if not already present
			if GetParam(securitySpec.KeyManagementService.ConnectionDetails, vault.VaultBackendKey) == "" {
				backendVersion, err := BackendVersion(clusterdContext, ns, securitySpec.KeyManagementService.ConnectionDetails)
				if err != nil {
					return errors.Wrap(err, "failed to get backend version")
				}
				securitySpec.KeyManagementService.ConnectionDetails[vault.VaultBackendKey] = backendVersion
			}
		}
	default:
		return errors.Errorf("failed to validate kms provider connection details (provider %q not supported)", provider)
	}

	return nil
}

// SetTokenToEnvVar sets a KMS token as an env variable
func SetTokenToEnvVar(clusterdContext *clusterd.Context, tokenSecretName, provider, namespace string) error {
	ctx := context.TODO()
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
