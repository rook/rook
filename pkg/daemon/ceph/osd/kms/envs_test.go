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
	"reflect"
	"testing"

	cephv1 "github.com/rook/rook/pkg/apis/ceph.rook.io/v1"
	"github.com/stretchr/testify/assert"
	v1 "k8s.io/api/core/v1"
)

func TestVaultTLSEnvVarFromSecret(t *testing.T) {
	type args struct {
		kmsConfig map[string]string
	}
	tests := []struct {
		name string
		args args
		want []v1.EnvVar
	}{
		{"no tls", args{kmsConfig: map[string]string{"foo": "bar"}}, []v1.EnvVar{}},
		{"ca cert tls", args{kmsConfig: map[string]string{"foo": "bar", "VAULT_CACERT": "vault-ca-cert-secret"}}, []v1.EnvVar{{Name: "VAULT_CACERT", Value: "/etc/vault/vault.ca"}}},
		{"ca cert tls and client ca/key", args{kmsConfig: map[string]string{
			"foo":               "bar",
			"VAULT_CACERT":      "vault-ca-cert-secret",
			"VAULT_CLIENT_CERT": "vault-client-cert-secret",
			"VAULT_CLIENT_KEY":  "vault-key-cert-secret",
		}},
			[]v1.EnvVar{
				{Name: "VAULT_CACERT", Value: "/etc/vault/vault.ca"},
				{Name: "VAULT_CLIENT_CERT", Value: "/etc/vault/vault.crt"},
				{Name: "VAULT_CLIENT_KEY", Value: "/etc/vault/vault.key"},
			}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := vaultTLSEnvVarFromSecret(tt.args.kmsConfig); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("vaultTLSEnvVarFromSecret() = %v, want %v", got, tt.want)
			}
		})
	}
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
		{"no tls", args{spec: cephv1.ClusterSpec{Security: cephv1.SecuritySpec{KeyManagementService: cephv1.KeyManagementServiceSpec{TokenSecretName: "vault-token", ConnectionDetails: map[string]string{"KMS_PROVIDER": "vault", "VAULT_ADDR": "http://1.1.1.1:8200"}}}}}, []v1.EnvVar{
			{Name: "KMS_PROVIDER", Value: "vault"},
			{Name: "VAULT_ADDR", Value: "http://1.1.1.1:8200"},
			{Name: "VAULT_TOKEN", ValueFrom: &v1.EnvVarSource{
				SecretKeyRef: &v1.SecretKeySelector{
					LocalObjectReference: v1.LocalObjectReference{
						Name: "vault-token",
					},
					Key: "token",
				},
			},
			},
		},
		},
		{" tls", args{spec: cephv1.ClusterSpec{Security: cephv1.SecuritySpec{KeyManagementService: cephv1.KeyManagementServiceSpec{TokenSecretName: "vault-token", ConnectionDetails: map[string]string{"KMS_PROVIDER": "vault", "VAULT_ADDR": "http://1.1.1.1:8200", "VAULT_CACERT": "vault-ca-cert-secret"}}}}}, []v1.EnvVar{
			{Name: "KMS_PROVIDER", Value: "vault"},
			{Name: "VAULT_ADDR", Value: "http://1.1.1.1:8200"},
			{Name: "VAULT_TOKEN", ValueFrom: &v1.EnvVarSource{
				SecretKeyRef: &v1.SecretKeySelector{
					LocalObjectReference: v1.LocalObjectReference{
						Name: "vault-token",
					},
					Key: "token",
				},
			},
			},
			{Name: "VAULT_CACERT", Value: "/etc/vault/vault.ca"},
		},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := VaultConfigToEnvVar(tt.args.spec); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("VaultConfigToEnvVar() = %v, want %v", got, tt.want)
			}
		})
	}
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
