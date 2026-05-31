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
	"path"
	"reflect"
	"testing"

	"github.com/libopenstorage/secrets"
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
	m := int32(0o444)
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
		{
			"single ca", args{config: map[string]string{"VAULT_CACERT": "vault-ca-secret"}, tokenSecretName: ""}, []v1.VolumeProjection{
				{Secret: &v1.SecretProjection{LocalObjectReference: v1.LocalObjectReference{Name: "vault-ca-secret"}, Items: []v1.KeyToPath{{Key: "cert", Path: "vault.ca", Mode: &m}}, Optional: nil}},
			},
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
		{
			"token file", args{tokenSecretName: "vault-token", config: map[string]string{"foo": "bar"}}, []v1.VolumeProjection{
				{Secret: &v1.SecretProjection{LocalObjectReference: v1.LocalObjectReference{Name: "vault-token"}, Items: []v1.KeyToPath{{Key: "token", Path: "vault.token", Mode: &m}}, Optional: nil}},
			},
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

func TestVaultVolumeAndMountWithCustomName(t *testing.T) {
	m := int32(0o444)
	type args struct {
		config          map[string]string
		tokenSecretName string
		customName      string
	}
	tests := []struct {
		name         string
		args         args
		wantVol      v1.Volume
		wantVolMount v1.VolumeMount
	}{
		{"empty without custom name", args{config: map[string]string{}, tokenSecretName: "", customName: ""}, v1.Volume{}, v1.VolumeMount{}},
		{"no kms related configs without custom name", args{config: map[string]string{"foo": "bar"}, tokenSecretName: "", customName: ""}, v1.Volume{Name: secrets.TypeVault, VolumeSource: v1.VolumeSource{Projected: &v1.ProjectedVolumeSource{Sources: []v1.VolumeProjection{}}}}, v1.VolumeMount{Name: secrets.TypeVault, ReadOnly: true, MountPath: EtcVaultDir}},
		{
			"only cert passed without custom name", args{config: map[string]string{"VAULT_CACERT": "vault-ca-secret"}, tokenSecretName: "", customName: ""}, v1.Volume{Name: secrets.TypeVault, VolumeSource: v1.VolumeSource{Projected: &v1.ProjectedVolumeSource{Sources: []v1.VolumeProjection{
				{Secret: &v1.SecretProjection{LocalObjectReference: v1.LocalObjectReference{Name: "vault-ca-secret"}, Items: []v1.KeyToPath{{Key: "cert", Path: "vault.ca", Mode: &m}}, Optional: nil}},
			}}}}, v1.VolumeMount{Name: secrets.TypeVault, ReadOnly: true, MountPath: EtcVaultDir},
		},
		{
			"only token passed without custom name", args{tokenSecretName: "vault-token", config: map[string]string{"foo": "bar"}, customName: ""}, v1.Volume{Name: secrets.TypeVault, VolumeSource: v1.VolumeSource{Projected: &v1.ProjectedVolumeSource{Sources: []v1.VolumeProjection{
				{Secret: &v1.SecretProjection{LocalObjectReference: v1.LocalObjectReference{Name: "vault-token"}, Items: []v1.KeyToPath{{Key: "token", Path: "vault.token", Mode: &m}}, Optional: nil}},
			}}}}, v1.VolumeMount{Name: secrets.TypeVault, ReadOnly: true, MountPath: EtcVaultDir},
		},
		{
			"both token and cert passed without custom name", args{tokenSecretName: "vault-token", config: map[string]string{"VAULT_CACERT": "vault-ca-secret"}, customName: ""}, v1.Volume{Name: secrets.TypeVault, VolumeSource: v1.VolumeSource{Projected: &v1.ProjectedVolumeSource{Sources: []v1.VolumeProjection{
				{Secret: &v1.SecretProjection{LocalObjectReference: v1.LocalObjectReference{Name: "vault-ca-secret"}, Items: []v1.KeyToPath{{Key: "cert", Path: "vault.ca", Mode: &m}}, Optional: nil}},
				{Secret: &v1.SecretProjection{LocalObjectReference: v1.LocalObjectReference{Name: "vault-token"}, Items: []v1.KeyToPath{{Key: "token", Path: "vault.token", Mode: &m}}, Optional: nil}},
			}}}}, v1.VolumeMount{Name: secrets.TypeVault, ReadOnly: true, MountPath: EtcVaultDir},
		},
		{"empty with custom name", args{config: map[string]string{}, tokenSecretName: "", customName: "custom"}, v1.Volume{}, v1.VolumeMount{}},
		{"no kms related configs with custom name", args{config: map[string]string{"foo": "bar"}, tokenSecretName: "", customName: "custom"}, v1.Volume{Name: secrets.TypeVault + "custom", VolumeSource: v1.VolumeSource{Projected: &v1.ProjectedVolumeSource{Sources: []v1.VolumeProjection{}}}}, v1.VolumeMount{Name: secrets.TypeVault + "custom", ReadOnly: true, MountPath: path.Join(EtcVaultDir, "custom")}},
		{
			"only cert passed with custom name", args{config: map[string]string{"VAULT_CACERT": "vault-ca-secret"}, tokenSecretName: "", customName: "custom"}, v1.Volume{Name: secrets.TypeVault + "custom", VolumeSource: v1.VolumeSource{Projected: &v1.ProjectedVolumeSource{Sources: []v1.VolumeProjection{
				{Secret: &v1.SecretProjection{LocalObjectReference: v1.LocalObjectReference{Name: "vault-ca-secret"}, Items: []v1.KeyToPath{{Key: "cert", Path: "vault.ca", Mode: &m}}, Optional: nil}},
			}}}}, v1.VolumeMount{Name: secrets.TypeVault + "custom", ReadOnly: true, MountPath: path.Join(EtcVaultDir, "custom")},
		},
		{
			"only token passed with custom name", args{tokenSecretName: "vault-token", config: map[string]string{"foo": "bar"}, customName: "custom"}, v1.Volume{Name: secrets.TypeVault + "custom", VolumeSource: v1.VolumeSource{Projected: &v1.ProjectedVolumeSource{Sources: []v1.VolumeProjection{
				{Secret: &v1.SecretProjection{LocalObjectReference: v1.LocalObjectReference{Name: "vault-token"}, Items: []v1.KeyToPath{{Key: "token", Path: "vault.token", Mode: &m}}, Optional: nil}},
			}}}}, v1.VolumeMount{Name: secrets.TypeVault + "custom", ReadOnly: true, MountPath: path.Join(EtcVaultDir, "custom")},
		},
		{
			"both token and cert passed with custom name", args{tokenSecretName: "vault-token", config: map[string]string{"VAULT_CACERT": "vault-ca-secret"}, customName: "custom"}, v1.Volume{Name: secrets.TypeVault + "custom", VolumeSource: v1.VolumeSource{Projected: &v1.ProjectedVolumeSource{Sources: []v1.VolumeProjection{
				{Secret: &v1.SecretProjection{LocalObjectReference: v1.LocalObjectReference{Name: "vault-ca-secret"}, Items: []v1.KeyToPath{{Key: "cert", Path: "vault.ca", Mode: &m}}, Optional: nil}},
				{Secret: &v1.SecretProjection{LocalObjectReference: v1.LocalObjectReference{Name: "vault-token"}, Items: []v1.KeyToPath{{Key: "token", Path: "vault.token", Mode: &m}}, Optional: nil}},
			}}}}, v1.VolumeMount{Name: secrets.TypeVault + "custom", ReadOnly: true, MountPath: path.Join(EtcVaultDir, "custom")},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotVol, gotVolMount := VaultVolumeAndMountWithCustomName(tt.args.config, tt.args.tokenSecretName, tt.args.customName)
			if !reflect.DeepEqual(gotVol, tt.wantVol) {
				t.Errorf("VaultVolumeAndMountWithCustomName() = %v, want %v", gotVol, tt.wantVol)
			}
			if !reflect.DeepEqual(gotVolMount, tt.wantVolMount) {
				t.Errorf("VaultVolumeAndMountWithCustomName() = %v, want %v", gotVolMount, tt.wantVolMount)
			}
		})
	}
}

func TestKMIPVolumeAndMount(t *testing.T) {
	mode := int32(0o444)
	tokenSecretName := "kmip-credentials"
	type args struct {
		tokenSecretName string
	}
	tests := []struct {
		name  string
		args  args
		want  v1.Volume
		want1 v1.VolumeMount
	}{
		{
			name: "",
			args: args{
				tokenSecretName: tokenSecretName,
			},
			want: v1.Volume{
				Name: TypeKMIP,
				VolumeSource: v1.VolumeSource{
					Projected: &v1.ProjectedVolumeSource{
						Sources: []v1.VolumeProjection{
							{
								Secret: &v1.SecretProjection{
									LocalObjectReference: v1.LocalObjectReference{
										Name: tokenSecretName,
									},
									Items: []v1.KeyToPath{
										{
											Key:  KmipCACert,
											Path: KmipCACertFileName,
											Mode: &mode,
										},
										{
											Key:  KmipClientCert,
											Path: KmipClientCertFileName,
											Mode: &mode,
										},
										{
											Key:  KmipClientKey,
											Path: KmipClientKeyFileName,
											Mode: &mode,
										},
									},
								},
							},
						},
					},
				},
			},
			want1: v1.VolumeMount{
				Name:      TypeKMIP,
				ReadOnly:  true,
				MountPath: EtcKmipDir,
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, got1 := KMIPVolumeAndMount(tt.args.tokenSecretName)
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("KMIPVolumeAndMount() got = %v, want %v", got, tt.want)
			}
			if !reflect.DeepEqual(got1, tt.want1) {
				t.Errorf("KMIPVolumeAndMount() got1 = %v, want %v", got1, tt.want1)
			}
		})
	}
}
