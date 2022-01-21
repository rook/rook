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

package v1

import (
	"strings"

	"github.com/hashicorp/vault/api"
	"github.com/libopenstorage/secrets"
	"github.com/libopenstorage/secrets/vault"
)

var (
	VaultTLSConnectionDetails = []string{api.EnvVaultCACert, api.EnvVaultClientCert, api.EnvVaultClientKey}
)

// IsEnabled return whether a KMS is configured
func (kms *KeyManagementServiceSpec) IsEnabled() bool {
	return len(kms.ConnectionDetails) != 0
}

// IsTokenAuthEnabled return whether KMS token auth is enabled
func (kms *KeyManagementServiceSpec) IsTokenAuthEnabled() bool {
	return kms.TokenSecretName != ""
}

// IsK8sAuthEnabled return whether KMS Kubernetes auth is enabled
func (kms *KeyManagementServiceSpec) IsK8sAuthEnabled() bool {
	return getParam(kms.ConnectionDetails, vault.AuthMethod) == vault.AuthMethodKubernetes && kms.TokenSecretName == ""
}

// IsVaultKMS return whether Vault KMS is configured
func (kms *KeyManagementServiceSpec) IsVaultKMS() bool {
	return getParam(kms.ConnectionDetails, "KMS_PROVIDER") == secrets.TypeVault
}

// IsIBMKeyProtectKMS return whether IBM Key Protect KMS is configured
func (kms *KeyManagementServiceSpec) IsIBMKeyProtectKMS() bool {
	return getParam(kms.ConnectionDetails, "KMS_PROVIDER") == "ibmkeyprotect"
}

// IsTLSEnabled return KMS TLS details are configured
func (kms *KeyManagementServiceSpec) IsTLSEnabled() bool {
	for _, tlsOption := range VaultTLSConnectionDetails {
		tlsSecretName := getParam(kms.ConnectionDetails, tlsOption)
		if tlsSecretName != "" {
			return true
		}
	}
	return false
}

// getParam returns the value of the KMS config option
func getParam(kmsConfig map[string]string, param string) string {
	if val, ok := kmsConfig[param]; ok && val != "" {
		return strings.TrimSpace(val)
	}
	return ""
}
