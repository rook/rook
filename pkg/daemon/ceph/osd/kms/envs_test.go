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
	"sort"
	"testing"

	cephv1 "github.com/rook/rook/pkg/apis/ceph.rook.io/v1"
	"github.com/stretchr/testify/assert"
	v1 "k8s.io/api/core/v1"
)

func TestVaultTLSEnvVarFromSecret(t *testing.T) {
	// No TLS
	spec := cephv1.ClusterSpec{Security: cephv1.SecuritySpec{KeyManagementService: cephv1.KeyManagementServiceSpec{TokenSecretName: "vault-token", ConnectionDetails: map[string]string{"KMS_PROVIDER": "vault", "VAULT_ADDR": "http://1.1.1.1:8200"}}}}
	envVars := VaultConfigToEnvVar(spec)
	areEnvVarsSorted := sort.SliceIsSorted(envVars, func(i, j int) bool {
		return envVars[i].Name < envVars[j].Name
	})
	assert.True(t, areEnvVarsSorted)
	assert.Equal(t, 4, len(envVars))
	assert.Contains(t, envVars, v1.EnvVar{Name: "KMS_PROVIDER", Value: "vault"})
	assert.Contains(t, envVars, v1.EnvVar{Name: "VAULT_ADDR", Value: "http://1.1.1.1:8200"})
	assert.Contains(t, envVars, v1.EnvVar{Name: "VAULT_TOKEN", ValueFrom: &v1.EnvVarSource{SecretKeyRef: &v1.SecretKeySelector{LocalObjectReference: v1.LocalObjectReference{Name: "vault-token"}, Key: "token"}}})
	assert.Contains(t, envVars, v1.EnvVar{Name: "VAULT_BACKEND_PATH", Value: "secret/"})

	// TLS
	spec = cephv1.ClusterSpec{Security: cephv1.SecuritySpec{KeyManagementService: cephv1.KeyManagementServiceSpec{TokenSecretName: "vault-token", ConnectionDetails: map[string]string{"KMS_PROVIDER": "vault", "VAULT_ADDR": "http://1.1.1.1:8200", "VAULT_CACERT": "vault-ca-cert-secret"}}}}
	envVars = VaultConfigToEnvVar(spec)
	areEnvVarsSorted = sort.SliceIsSorted(envVars, func(i, j int) bool {
		return envVars[i].Name < envVars[j].Name
	})
	assert.True(t, areEnvVarsSorted)
	assert.Equal(t, 5, len(envVars))
	assert.Contains(t, envVars, v1.EnvVar{Name: "KMS_PROVIDER", Value: "vault"})
	assert.Contains(t, envVars, v1.EnvVar{Name: "VAULT_ADDR", Value: "http://1.1.1.1:8200"})
	assert.Contains(t, envVars, v1.EnvVar{Name: "VAULT_CACERT", Value: "/etc/vault/vault.ca"})
	assert.Contains(t, envVars, v1.EnvVar{Name: "VAULT_TOKEN", ValueFrom: &v1.EnvVarSource{SecretKeyRef: &v1.SecretKeySelector{LocalObjectReference: v1.LocalObjectReference{Name: "vault-token"}, Key: "token"}}})

}

func TestConfigEnvsToMapString(t *testing.T) {
	// No VAULT envs
	envs := ConfigEnvsToMapString()
	assert.Equal(t, 0, len(envs))

	// Single KMS value
	os.Setenv("KMS_PROVIDER", "vault")
	defer os.Unsetenv("KMS_PROVIDER")
	envs = ConfigEnvsToMapString()
	assert.Equal(t, 1, len(envs))

	// Some more Vault KMS with one intruder
	os.Setenv("KMS_PROVIDER", "vault")
	defer os.Unsetenv("KMS_PROVIDER")
	os.Setenv("VAULT_ADDR", "1.1.1.1")
	defer os.Unsetenv("VAULT_ADDR")
	os.Setenv("VAULT_SKIP_VERIFY", "true")
	defer os.Unsetenv("VAULT_SKIP_VERIFY")
	os.Setenv("foo", "bar")
	defer os.Unsetenv("foo")
	envs = ConfigEnvsToMapString()
	assert.Equal(t, 3, len(envs))
}
