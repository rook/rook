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
	"github.com/hashicorp/vault/api"
	"github.com/libopenstorage/secrets"
	cephv1 "github.com/rook/rook/pkg/apis/ceph.rook.io/v1"
	v1 "k8s.io/api/core/v1"
)

const (
	// Key name of the Secret containing the cert and client key
	vaultCACertSecretKeyName = "cert"
	vaultKeySecretKeyName    = "key"

	// File names of the Secret value when mapping on the filesystem
	VaultCAFileName   = "vault.ca"
	VaultCertFileName = "vault.crt"
	VaultKeyFileName  = "vault.key"

	// File name for token file
	VaultFileName = "vault.token"
)

// VaultSecretVolumeAndMount return the volume and matching volume mount for mounting the vault secrets into /etc/vault
func VaultSecretVolumeAndMount(kmsVaultConfigFiles map[string]string, tokenSecretName string) []v1.VolumeProjection {
	// Projection list
	secretVolumeProjections := []v1.VolumeProjection{}

	// File mode, anybody can read, this is a must-have since the container runs as "rook" and the
	// secret is mounted as root. There is no non-ugly way to change this behavior and it's
	// probably as less safe as doing this mode.
	mode := int32(0444)

	// Vault TLS Secrets
	for _, tlsOption := range cephv1.VaultTLSConnectionDetails {
		tlsSecretName := GetParam(kmsVaultConfigFiles, tlsOption)
		if tlsSecretName != "" {
			projectionSecret := &v1.SecretProjection{Items: []v1.KeyToPath{{Key: tlsSecretKeyToCheck(tlsOption), Path: tlsSecretPath(tlsOption), Mode: &mode}}}
			projectionSecret.Name = tlsSecretName
			secretProjection := v1.VolumeProjection{Secret: projectionSecret}
			secretVolumeProjections = append(secretVolumeProjections, secretProjection)
		}
	}
	if tokenSecretName != "" {
		projectionSecret := &v1.SecretProjection{Items: []v1.KeyToPath{{Key: KMSTokenSecretNameKey, Path: VaultFileName, Mode: &mode}}}
		projectionSecret.Name = tokenSecretName
		secretProjection := v1.VolumeProjection{Secret: projectionSecret}
		secretVolumeProjections = append(secretVolumeProjections, secretProjection)
	}
	return secretVolumeProjections
}

// VaultVolumeAndMount returns Vault volume and volume mount
func VaultVolumeAndMount(kmsVaultConfigFiles map[string]string, tokenSecretName string) (v1.Volume, v1.VolumeMount) {
	v := v1.Volume{
		Name: secrets.TypeVault,
		VolumeSource: v1.VolumeSource{
			Projected: &v1.ProjectedVolumeSource{
				Sources: VaultSecretVolumeAndMount(kmsVaultConfigFiles, tokenSecretName),
			},
		},
	}

	m := v1.VolumeMount{
		Name:      secrets.TypeVault,
		ReadOnly:  true,
		MountPath: EtcVaultDir,
	}

	return v, m
}

func tlsSecretPath(tlsOption string) string {
	switch tlsOption {
	case api.EnvVaultCACert:
		return VaultCAFileName
	case api.EnvVaultClientCert:
		return VaultCertFileName
	case api.EnvVaultClientKey:
		return VaultKeyFileName

	}

	return ""
}
