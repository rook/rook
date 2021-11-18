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
	"testing"

	v1 "k8s.io/api/core/v1"
)

func Test_tlsSecretPath(t *testing.T) {
	type args struct {
		tlsOption string
	}
	tests := []struct {
		name string
		args args
		want string
	}{
		{"certificate", args{tlsOption: "VAULT_CACERT"}, "vault.ca"},
		{"client-certificate", args{tlsOption: "VAULT_CLIENT_CERT"}, "vault.crt"},
		{"client-key", args{tlsOption: "VAULT_CLIENT_KEY"}, "vault.key"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tlsSecretPath(tt.args.tlsOption); got != tt.want {
				t.Errorf("tlsSecretPath() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestVaultSecretVolumeAndMount(t *testing.T) {
	m := int32(0444)
	type args struct {
		config          map[string]string
		tokenSecretName string
	}
	tests := []struct {
		name string
		args args
		want []v1.VolumeProjection
	}{
		{"empty", args{config: map[string]string{"foo": "bar"}, tokenSecretName: ""}, []v1.VolumeProjection{}},
		{"single ca", args{config: map[string]string{"VAULT_CACERT": "vault-ca-secret"}, tokenSecretName: ""}, []v1.VolumeProjection{
			{Secret: &v1.SecretProjection{LocalObjectReference: v1.LocalObjectReference{Name: "vault-ca-secret"}, Items: []v1.KeyToPath{{Key: "cert", Path: "vault.ca", Mode: &m}}, Optional: nil}}},
		},
		{"ca and client cert", args{config: map[string]string{"VAULT_CACERT": "vault-ca-secret", "VAULT_CLIENT_CERT": "vault-client-cert"}, tokenSecretName: ""}, []v1.VolumeProjection{
			{Secret: &v1.SecretProjection{LocalObjectReference: v1.LocalObjectReference{Name: "vault-ca-secret"}, Items: []v1.KeyToPath{{Key: "cert", Path: "vault.ca", Mode: &m}}, Optional: nil}},
			{Secret: &v1.SecretProjection{LocalObjectReference: v1.LocalObjectReference{Name: "vault-client-cert"}, Items: []v1.KeyToPath{{Key: "cert", Path: "vault.crt", Mode: &m}}, Optional: nil}},
		}},
		{"ca and client cert/key", args{config: map[string]string{"VAULT_CACERT": "vault-ca-secret", "VAULT_CLIENT_CERT": "vault-client-cert", "VAULT_CLIENT_KEY": "vault-client-key"}, tokenSecretName: ""}, []v1.VolumeProjection{
			{Secret: &v1.SecretProjection{LocalObjectReference: v1.LocalObjectReference{Name: "vault-ca-secret"}, Items: []v1.KeyToPath{{Key: "cert", Path: "vault.ca", Mode: &m}}, Optional: nil}},
			{Secret: &v1.SecretProjection{LocalObjectReference: v1.LocalObjectReference{Name: "vault-client-cert"}, Items: []v1.KeyToPath{{Key: "cert", Path: "vault.crt", Mode: &m}}, Optional: nil}},
			{Secret: &v1.SecretProjection{LocalObjectReference: v1.LocalObjectReference{Name: "vault-client-key"}, Items: []v1.KeyToPath{{Key: "key", Path: "vault.key", Mode: &m}}, Optional: nil}},
		}},
		{"token file", args{tokenSecretName: "vault-token", config: map[string]string{"foo": "bar"}}, []v1.VolumeProjection{
			{Secret: &v1.SecretProjection{LocalObjectReference: v1.LocalObjectReference{Name: "vault-token"}, Items: []v1.KeyToPath{{Key: "token", Path: "vault.token", Mode: &m}}, Optional: nil}}},
		},
		{"token and ca", args{tokenSecretName: "vault-token", config: map[string]string{"VAULT_CACERT": "vault-ca-secret"}}, []v1.VolumeProjection{
			{Secret: &v1.SecretProjection{LocalObjectReference: v1.LocalObjectReference{Name: "vault-ca-secret"}, Items: []v1.KeyToPath{{Key: "cert", Path: "vault.ca", Mode: &m}}, Optional: nil}},
			{Secret: &v1.SecretProjection{LocalObjectReference: v1.LocalObjectReference{Name: "vault-token"}, Items: []v1.KeyToPath{{Key: "token", Path: "vault.token", Mode: &m}}, Optional: nil}},
		}},
		{"token, ca, and client cert", args{tokenSecretName: "vault-token", config: map[string]string{"VAULT_CACERT": "vault-ca-secret", "VAULT_CLIENT_CERT": "vault-client-cert"}}, []v1.VolumeProjection{
			{Secret: &v1.SecretProjection{LocalObjectReference: v1.LocalObjectReference{Name: "vault-ca-secret"}, Items: []v1.KeyToPath{{Key: "cert", Path: "vault.ca", Mode: &m}}, Optional: nil}},
			{Secret: &v1.SecretProjection{LocalObjectReference: v1.LocalObjectReference{Name: "vault-client-cert"}, Items: []v1.KeyToPath{{Key: "cert", Path: "vault.crt", Mode: &m}}, Optional: nil}},
			{Secret: &v1.SecretProjection{LocalObjectReference: v1.LocalObjectReference{Name: "vault-token"}, Items: []v1.KeyToPath{{Key: "token", Path: "vault.token", Mode: &m}}, Optional: nil}},
		}},
		{"token, ca, and client cert/key", args{tokenSecretName: "vault-token", config: map[string]string{"VAULT_CACERT": "vault-ca-secret", "VAULT_CLIENT_CERT": "vault-client-cert", "VAULT_CLIENT_KEY": "vault-client-key"}}, []v1.VolumeProjection{
			{Secret: &v1.SecretProjection{LocalObjectReference: v1.LocalObjectReference{Name: "vault-ca-secret"}, Items: []v1.KeyToPath{{Key: "cert", Path: "vault.ca", Mode: &m}}, Optional: nil}},
			{Secret: &v1.SecretProjection{LocalObjectReference: v1.LocalObjectReference{Name: "vault-client-cert"}, Items: []v1.KeyToPath{{Key: "cert", Path: "vault.crt", Mode: &m}}, Optional: nil}},
			{Secret: &v1.SecretProjection{LocalObjectReference: v1.LocalObjectReference{Name: "vault-client-key"}, Items: []v1.KeyToPath{{Key: "key", Path: "vault.key", Mode: &m}}, Optional: nil}},
			{Secret: &v1.SecretProjection{LocalObjectReference: v1.LocalObjectReference{Name: "vault-token"}, Items: []v1.KeyToPath{{Key: "token", Path: "vault.token", Mode: &m}}, Optional: nil}},
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := VaultSecretVolumeAndMount(tt.args.config, tt.args.tokenSecretName); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("VaultSecretVolumeAndMount() = %v, want %v", got, tt.want)
			}
		})
	}
}
