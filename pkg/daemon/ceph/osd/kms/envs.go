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
	"os"
	"path"
	"sort"
	"strings"

	"github.com/hashicorp/vault/api"
	"github.com/libopenstorage/secrets/vault"
	cephv1 "github.com/rook/rook/pkg/apis/ceph.rook.io/v1"
	"github.com/rook/rook/pkg/daemon/ceph/client"
	v1 "k8s.io/api/core/v1"
)

var (
	knownKMSPrefix = []string{"VAULT_"}
)

// VaultTokenEnvVarFromSecret returns the kms token secret value as an env var
func vaultTokenEnvVarFromSecret(tokenSecretName string) v1.EnvVar {
	return v1.EnvVar{
		Name: api.EnvVaultToken,
		ValueFrom: &v1.EnvVarSource{
			SecretKeyRef: &v1.SecretKeySelector{
				LocalObjectReference: v1.LocalObjectReference{
					Name: tokenSecretName,
				},
				Key: KMSTokenSecretNameKey,
			},
		},
	}
}

// vaultTLSEnvVarFromSecret translates TLS env var which are set to k8s secret name to their actual path on the fs once mounted as volume
// See: TLSSecretVolumeAndMount() for more details
func vaultTLSEnvVarFromSecret(kmsConfig map[string]string) []v1.EnvVar {
	vaultTLSEnvVar := []v1.EnvVar{}

	for _, tlsOption := range cephv1.VaultTLSConnectionDetails {
		tlsSecretName := GetParam(kmsConfig, tlsOption)
		if tlsSecretName != "" {
			vaultTLSEnvVar = append(vaultTLSEnvVar, v1.EnvVar{Name: tlsOption, Value: path.Join(EtcVaultDir, tlsSecretPath(tlsOption))})
		}
	}

	return vaultTLSEnvVar
}

// VaultConfigToEnvVar populates the kms config as env variables
func VaultConfigToEnvVar(spec cephv1.ClusterSpec) []v1.EnvVar {
	envs := []v1.EnvVar{}
	backendPath := GetParam(spec.Security.KeyManagementService.ConnectionDetails, vault.VaultBackendPathKey)
	// Set BACKEND_PATH to the API's default if not passed
	if backendPath == "" {
		spec.Security.KeyManagementService.ConnectionDetails[vault.VaultBackendPathKey] = vault.DefaultBackendPath
	}
	for k, v := range spec.Security.KeyManagementService.ConnectionDetails {
		// Skip TLS and token env var to avoid env being set multiple times
		toSkip := append(cephv1.VaultTLSConnectionDetails, api.EnvVaultToken)
		if client.StringInSlice(k, toSkip) {
			continue
		}
		envs = append(envs, v1.EnvVar{Name: k, Value: v})
	}

	// Add the VAULT_TOKEN
	envs = append(envs, vaultTokenEnvVarFromSecret(spec.Security.KeyManagementService.TokenSecretName))

	// Add TLS env if any
	envs = append(envs, vaultTLSEnvVarFromSecret(spec.Security.KeyManagementService.ConnectionDetails)...)

	logger.Debugf("kms envs are %v", envs)

	// Sort env vars since the input is a map which by nature is unsorted...
	return sortV1EnvVar(envs)
}

// ConfigEnvsToMapString returns all the env variables in map from a known KMS
func ConfigEnvsToMapString() map[string]string {
	envs := make(map[string]string)
	for _, e := range os.Environ() {
		pair := strings.SplitN(e, "=", 2)
		for _, knownKMS := range knownKMSPrefix {
			if strings.HasPrefix(pair[0], knownKMS) || pair[0] == Provider {
				envs[pair[0]] = os.Getenv(pair[0])
			}
		}
	}

	return envs
}

// sortV1EnvVar sorts a list of v1.EnvVar
func sortV1EnvVar(envs []v1.EnvVar) []v1.EnvVar {
	sort.SliceStable(envs, func(i, j int) bool {
		return envs[i].Name < envs[j].Name
	})

	return envs
}
