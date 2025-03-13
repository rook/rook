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

	"github.com/hashicorp/vault/api"
	"github.com/libopenstorage/secrets"
	"github.com/libopenstorage/secrets/vault"
	"github.com/pkg/errors"
	cephv1 "github.com/rook/rook/pkg/apis/ceph.rook.io/v1"
	"github.com/rook/rook/pkg/clusterd"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	// EtcVaultDir is vault config dir
	EtcVaultDir = "/etc/vault"
	// VaultSecretEngineKey is the type of secret engine used (kv, transit)
	VaultSecretEngineKey = "VAULT_SECRET_ENGINE"
	// VaultKVSecretEngineKey is a kv secret engine type
	VaultKVSecretEngineKey = "kv"
	// VaultTransitSecretEngineKey is a transit secret engine type
	VaultTransitSecretEngineKey = "transit"
)

var vaultMandatoryConnectionDetails = []string{api.EnvVaultAddress}

// Used for unit tests mocking too as well as production code
var (
	createTmpFile      = os.CreateTemp
	getRemoveCertFiles = getRemoveCertFilesFunc
)

type removeCertFilesFunction func()

/* VAULT API INTERNAL VALUES
// Refer to https://pkg.golangclub.com/github.com/hashicorp/vault/api?tab=doc#pkg-constants
   const EnvVaultAddress = "VAULT_ADDR"
   const EnvVaultAgentAddr = "VAULT_AGENT_ADDR"
   const EnvVaultCACert = "VAULT_CACERT"
   const EnvVaultCAPath = "VAULT_CAPATH"
   const EnvVaultClientCert = "VAULT_CLIENT_CERT"
   const EnvVaultClientKey = "VAULT_CLIENT_KEY"
   const EnvVaultClientTimeout = "VAULT_CLIENT_TIMEOUT"
   const EnvVaultSRVLookup = "VAULT_SRV_LOOKUP"
   const EnvVaultSkipVerify = "VAULT_SKIP_VERIFY"
   const EnvVaultNamespace = "VAULT_NAMESPACE"
   const EnvVaultTLSServerName = "VAULT_TLS_SERVER_NAME"
   const EnvVaultWrapTTL = "VAULT_WRAP_TTL"
   const EnvVaultMaxRetries = "VAULT_MAX_RETRIES"
   const EnvVaultToken = "VAULT_TOKEN"
   const EnvVaultMFA = "VAULT_MFA"
   const EnvRateLimit = "VAULT_RATE_LIMIT"
*/

// InitVault inits the secret store
func InitVault(ctx context.Context, context *clusterd.Context, namespace string, config map[string]string) (secrets.Secrets, error) {
	c := make(map[string]interface{})

	// So that we don't alter the content of c.config for later iterations
	// We just want to swap the name of the TLS config secret name --> file name for the kms lib
	oriConfig := make(map[string]string)
	for k, v := range config {
		oriConfig[k] = v
	}

	// Populate TLS config
	newConfigWithTLS, removeCertFiles, err := configTLS(ctx, context, namespace, oriConfig)
	if err != nil {
		return nil, errors.Wrap(err, "failed to initialize vault tls configuration")
	}
	defer removeCertFiles()

	// Populate TLS config
	for key, value := range newConfigWithTLS {
		c[key] = string(value)
	}

	// Initialize Vault
	v, err := vault.New(c)
	if err != nil {
		return nil, errors.Wrap(err, "failed to initialize vault secret store")
	}

	return v, nil
}

// configTLS returns a map of TLS config that map physical files for the TLS library to load
// Also it returns a function to remove the temporary files (certs, keys)
// The signature has named result parameters to help building 'defer' statements especially for the
// content of removeCertFiles which needs to be populated by the files to remove if no errors and be
// nil on errors
func configTLS(ctx context.Context, clusterdContext *clusterd.Context, namespace string, config map[string]string) (newConfig map[string]string, removeCertFiles removeCertFilesFunction, retErr error) {
	var filesToRemove []*os.File

	defer func() {
		// Build the function that the caller should use to remove the temp files here
		// create it when this function is returning based on the currently-recorded files
		removeCertFiles = getRemoveCertFiles(filesToRemove)
		if retErr != nil {
			// If we encountered an error, remove the temp files
			removeCertFiles()

			// Also return an empty function to remove the temp files
			// It's fine to use nil here since the defer from the calling functions is only
			// triggered after evaluating any error, if on error the defer is not triggered since we
			// have returned already
			removeCertFiles = nil
		}
	}()

	for _, tlsOption := range cephv1.VaultTLSConnectionDetails {
		tlsSecretName := GetParam(config, tlsOption)
		if tlsSecretName == "" {
			continue
		}
		// If the string already has the correct path /etc/vault, we are in provisioner code and all the envs have been populated by the op already
		if !strings.Contains(tlsSecretName, EtcVaultDir) {
			secret, err := clusterdContext.Clientset.CoreV1().Secrets(namespace).Get(ctx, tlsSecretName, v1.GetOptions{})
			if err != nil {
				return nil, removeCertFiles, errors.Wrapf(err, "failed to fetch tls k8s secret %q", tlsSecretName)
			}
			// Generate a temp file
			file, err := createTmpFile("", "")
			if err != nil {
				return nil, removeCertFiles, errors.Wrapf(err, "failed to generate temp file for k8s secret %q content", tlsSecretName)
			}

			// Write into a file
			err = os.WriteFile(file.Name(), secret.Data[tlsSecretKeyToCheck(tlsOption)], 0o400)
			if err != nil {
				return nil, removeCertFiles, errors.Wrapf(err, "failed to write k8s secret %q content to a file", tlsSecretName)
			}

			logger.Debugf("replacing %q current content %q with %q", tlsOption, config[tlsOption], file.Name())

			// Update the env var with the path
			config[tlsOption] = file.Name()

			// Add the file to the list of files to remove
			filesToRemove = append(filesToRemove, file)
		} else {
			logger.Debugf("value of tlsOption %q tlsSecretName is already correct %q", tlsOption, tlsSecretName)
		}
	}

	return config, removeCertFiles, nil
}

func getRemoveCertFilesFunc(filesToRemove []*os.File) removeCertFilesFunction {
	return removeCertFilesFunction(func() {
		for _, file := range filesToRemove {
			logger.Debugf("closing %q", file.Name())
			err := file.Close()
			if err != nil {
				logger.Errorf("failed to close file %q. %v", file.Name(), err)
			}
			logger.Debugf("closed %q", file.Name())
			logger.Debugf("removing %q", file.Name())
			err = os.Remove(file.Name())
			if err != nil {
				logger.Errorf("failed to remove file %q. %v", file.Name(), err)
			}
			logger.Debugf("removed %q", file.Name())
		}
	})
}

func buildVaultKeyContext(config map[string]string) map[string]string {
	// Key context is just the Vault namespace, available in the enterprise version only
	keyContext := map[string]string{secrets.KeyVaultNamespace: config[api.EnvVaultNamespace]}
	vaultNamespace, ok := config[api.EnvVaultNamespace]
	if !ok || vaultNamespace == "" {
		keyContext = map[string]string{}
	}

	return keyContext
}

// IsVault determines whether the configured KMS is Vault
func (c *Config) IsVault() bool {
	return c.Provider == secrets.TypeVault
}

func validateVaultConnectionDetails(ctx context.Context, clusterdContext *clusterd.Context, ns string, kmsConfig map[string]string) error {
	for _, option := range vaultMandatoryConnectionDetails {
		if GetParam(kmsConfig, option) == "" {
			return errors.Errorf("failed to find connection details %q", option)
		}
	}

	// We do not support a directory with multiple CA since we fetch a k8s Secret and read its content
	// So we operate with a single CA only
	if GetParam(kmsConfig, api.EnvVaultCAPath) != "" {
		return errors.Errorf("failed to validate TLS connection details. %q is not supported. use %q instead", api.EnvVaultCAPath, api.EnvVaultCACert)
	}

	// Validate potential TLS configuration
	for _, tlsOption := range cephv1.VaultTLSConnectionDetails {
		tlsSecretName := GetParam(kmsConfig, tlsOption)
		if tlsSecretName != "" {
			// Fetch the secret
			s, err := clusterdContext.Clientset.CoreV1().Secrets(ns).Get(ctx, tlsSecretName, v1.GetOptions{})
			if err != nil {
				return errors.Wrapf(err, "failed to find TLS connection details k8s secret %q", tlsSecretName)
			}

			// Check the Secret key and its content
			keyToCheck := tlsSecretKeyToCheck(tlsOption)
			cert, ok := s.Data[keyToCheck]
			if !ok || len(cert) == 0 {
				return errors.Errorf("failed to find TLS connection key %q for %q in k8s secret %q", keyToCheck, tlsOption, tlsSecretName)
			}
		}
	}

	return nil
}

func tlsSecretKeyToCheck(tlsOption string) string {
	if tlsOption == api.EnvVaultCACert || tlsOption == api.EnvVaultClientCert {
		return vaultCACertSecretKeyName
	} else if tlsOption == api.EnvVaultClientKey {
		return vaultKeySecretKeyName
	}

	return ""
}
