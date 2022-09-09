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
	"reflect"
	"sort"
	"testing"

	cephv1 "github.com/rook/rook/pkg/apis/ceph.rook.io/v1"
	"github.com/stretchr/testify/assert"
	v1 "k8s.io/api/core/v1"
)

func TestVaultTLSEnvVarFromSecret(t *testing.T) {
	t.Run("vault - no tls", func(t *testing.T) {
		spec := cephv1.ClusterSpec{Security: cephv1.SecuritySpec{KeyManagementService: cephv1.KeyManagementServiceSpec{
			TokenSecretName:   "vault-token",
			ConnectionDetails: map[string]string{"KMS_PROVIDER": "vault", "VAULT_ADDR": "http://1.1.1.1:8200"}}},
		}
		envVars := ConfigToEnvVar(spec)
		areEnvVarsSorted := sort.SliceIsSorted(envVars, func(i, j int) bool {
			return envVars[i].Name < envVars[j].Name
		})
		assert.True(t, areEnvVarsSorted)
		assert.Equal(t, 4, len(envVars))
		assert.Contains(t, envVars, v1.EnvVar{Name: "KMS_PROVIDER", Value: "vault"})
		assert.Contains(t, envVars, v1.EnvVar{Name: "VAULT_ADDR", Value: "http://1.1.1.1:8200"})
		assert.Contains(t, envVars, v1.EnvVar{Name: "VAULT_TOKEN", ValueFrom: &v1.EnvVarSource{SecretKeyRef: &v1.SecretKeySelector{LocalObjectReference: v1.LocalObjectReference{Name: "vault-token"}, Key: "token"}}})
		assert.Contains(t, envVars, v1.EnvVar{Name: "VAULT_BACKEND_PATH", Value: "secret/"})
	})
	t.Run("vault tls", func(t *testing.T) {
		spec := cephv1.ClusterSpec{Security: cephv1.SecuritySpec{KeyManagementService: cephv1.KeyManagementServiceSpec{
			TokenSecretName:   "vault-token",
			ConnectionDetails: map[string]string{"KMS_PROVIDER": "vault", "VAULT_ADDR": "http://1.1.1.1:8200", "VAULT_CACERT": "vault-ca-cert-secret"}}},
		}
		envVars := ConfigToEnvVar(spec)
		areEnvVarsSorted := sort.SliceIsSorted(envVars, func(i, j int) bool {
			return envVars[i].Name < envVars[j].Name
		})
		assert.True(t, areEnvVarsSorted)
		assert.Equal(t, 5, len(envVars))
		assert.Contains(t, envVars, v1.EnvVar{Name: "KMS_PROVIDER", Value: "vault"})
		assert.Contains(t, envVars, v1.EnvVar{Name: "VAULT_ADDR", Value: "http://1.1.1.1:8200"})
		assert.Contains(t, envVars, v1.EnvVar{Name: "VAULT_CACERT", Value: "/etc/vault/vault.ca"})
		assert.Contains(t, envVars, v1.EnvVar{Name: "VAULT_TOKEN", ValueFrom: &v1.EnvVarSource{SecretKeyRef: &v1.SecretKeySelector{LocalObjectReference: v1.LocalObjectReference{Name: "vault-token"}, Key: "token"}}})
	})
	t.Run("ibm kp", func(t *testing.T) {
		spec := cephv1.ClusterSpec{Security: cephv1.SecuritySpec{KeyManagementService: cephv1.KeyManagementServiceSpec{
			TokenSecretName:   "ibm-kp-token",
			ConnectionDetails: map[string]string{"KMS_PROVIDER": TypeIBM, "IBM_KP_SERVICE_INSTANCE_ID": "1"}}},
		}
		envVars := ConfigToEnvVar(spec)
		areEnvVarsSorted := sort.SliceIsSorted(envVars, func(i, j int) bool {
			return envVars[i].Name < envVars[j].Name
		})
		assert.True(t, areEnvVarsSorted)
		assert.Equal(t, 3, len(envVars))
		assert.Contains(t, envVars, v1.EnvVar{Name: "KMS_PROVIDER", Value: TypeIBM})
		assert.Contains(t, envVars, v1.EnvVar{Name: "IBM_KP_SERVICE_INSTANCE_ID", Value: "1"})
		assert.Contains(t, envVars, v1.EnvVar{Name: "IBM_KP_SERVICE_API_KEY", ValueFrom: &v1.EnvVarSource{SecretKeyRef: &v1.SecretKeySelector{LocalObjectReference: v1.LocalObjectReference{Name: "ibm-kp-token"}, Key: "IBM_KP_SERVICE_API_KEY"}}})
	})
}

func TestConfigEnvsToMapString(t *testing.T) {
	// No VAULT envs
	envs := ConfigEnvsToMapString()
	assert.Equal(t, 0, len(envs))

	// Single KMS value
	t.Setenv("KMS_PROVIDER", "vault")
	envs = ConfigEnvsToMapString()
	assert.Equal(t, 1, len(envs))

	// Some more Vault KMS with one intruder
	t.Setenv("KMS_PROVIDER", "vault")
	t.Setenv("VAULT_ADDR", "1.1.1.1")
	t.Setenv("VAULT_SKIP_VERIFY", "true")
	t.Setenv("foo", "bar")
	envs = ConfigEnvsToMapString()
	assert.Equal(t, 3, len(envs))
	assert.True(t, envs["KMS_PROVIDER"] == "vault")
	clusterSpec := &cephv1.ClusterSpec{Security: cephv1.SecuritySpec{KeyManagementService: cephv1.KeyManagementServiceSpec{ConnectionDetails: envs}}}
	assert.True(t, clusterSpec.Security.KeyManagementService.IsEnabled())
}

func TestVaultConfigToEnvVar(t *testing.T) {
	type args struct {
		spec cephv1.ClusterSpec
	}
	tests := []struct {
		name string
		args args
		want []v1.EnvVar
	}{
		{
			"vault - no backend path",
			args{spec: cephv1.ClusterSpec{Security: cephv1.SecuritySpec{KeyManagementService: cephv1.KeyManagementServiceSpec{ConnectionDetails: map[string]string{"KMS_PROVIDER": "vault"}}}}},
			[]v1.EnvVar{
				{Name: "KMS_PROVIDER", Value: "vault"},
				{Name: "VAULT_BACKEND_PATH", Value: "secret/"},
			},
		},
		{
			"vault - with backend path",
			args{spec: cephv1.ClusterSpec{Security: cephv1.SecuritySpec{KeyManagementService: cephv1.KeyManagementServiceSpec{ConnectionDetails: map[string]string{"KMS_PROVIDER": "vault", "VAULT_BACKEND_PATH": "foo/"}}}}},
			[]v1.EnvVar{
				{Name: "KMS_PROVIDER", Value: "vault"},
				{Name: "VAULT_BACKEND_PATH", Value: "foo/"},
			},
		},
		{
			"vault - test with tls config",
			args{spec: cephv1.ClusterSpec{Security: cephv1.SecuritySpec{KeyManagementService: cephv1.KeyManagementServiceSpec{ConnectionDetails: map[string]string{"KMS_PROVIDER": "vault", "VAULT_CACERT": "my-secret-name"}}}}},
			[]v1.EnvVar{
				{Name: "KMS_PROVIDER", Value: "vault"},
				{Name: "VAULT_BACKEND_PATH", Value: "secret/"},
				{Name: "VAULT_CACERT", Value: "/etc/vault/vault.ca"},
			},
		},
		{
			"ibm kp - IBM_KP_SERVICE_API_KEY is removed from the details",
			args{spec: cephv1.ClusterSpec{Security: cephv1.SecuritySpec{KeyManagementService: cephv1.KeyManagementServiceSpec{ConnectionDetails: map[string]string{"KMS_PROVIDER": TypeIBM, "IBM_KP_SERVICE_API_KEY": "foo", "IBM_KP_SERVICE_INSTANCE_ID": "1"}, TokenSecretName: "ibm-kp-token"}}}},
			[]v1.EnvVar{
				{Name: "IBM_KP_SERVICE_API_KEY", ValueFrom: &v1.EnvVarSource{SecretKeyRef: &v1.SecretKeySelector{LocalObjectReference: v1.LocalObjectReference{Name: "ibm-kp-token"}, Key: "IBM_KP_SERVICE_API_KEY"}}},
				{Name: "IBM_KP_SERVICE_INSTANCE_ID", Value: "1"},
				{Name: "KMS_PROVIDER", Value: TypeIBM},
			},
		},
		{
			"kmip - token details is removed from the details",
			args{spec: cephv1.ClusterSpec{Security: cephv1.SecuritySpec{KeyManagementService: cephv1.KeyManagementServiceSpec{ConnectionDetails: map[string]string{"KMS_PROVIDER": TypeKMIP, "CLIENT_KEY": "foo", "CLIENT_CERT": "foo", "CA_CERT": "foo", "TLS_SERVER_NAME": "pykmip"}, TokenSecretName: "kmip-token"}}}},
			[]v1.EnvVar{
				{Name: "KMIP_KMS_PROVIDER", Value: TypeKMIP},
				{Name: "KMIP_TLS_SERVER_NAME", Value: "pykmip"},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := ConfigToEnvVar(tt.args.spec); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("VaultConfigToEnvVar() = %v, want %v", got, tt.want)
			}
		})
	}
}
