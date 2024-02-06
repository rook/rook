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
	"fmt"

	"github.com/pkg/errors"
	cephclient "github.com/rook/rook/pkg/daemon/ceph/client"
	"github.com/rook/rook/pkg/operator/k8sutil"
	v1 "k8s.io/api/core/v1"
	kerrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	//nolint:gosec // OsdEncryptionSecretNameKeyName is the key name of the Secret that contains the OSD encryption key
	OsdEncryptionSecretNameKeyName = "dmcrypt-key"

	//nolint:gosec // since this is not leaking any hardcoded credentials, it's just the prefix of the secret name
	osdEncryptionSecretNamePrefix = "rook-ceph-osd-encryption-key"

	//nolint:gosec // KMSTokenSecretNameKey is the key name of the Secret that contains the KMS authentication token,
	KMSTokenSecretNameKey = "token"
)

// storeSecretInKubernetes stores the dmcrypt key in a Kubernetes Secret
func (c *Config) storeSecretInKubernetes(pvcName, key string) error {
	s, err := generateOSDEncryptedKeySecret(pvcName, key, c.ClusterInfo)
	if err != nil {
		return err
	}

	// Create the Kubernetes Secret
	_, err = c.context.Clientset.CoreV1().Secrets(c.ClusterInfo.Namespace).Create(c.ClusterInfo.Context, s, metav1.CreateOptions{})
	if err != nil && !kerrors.IsAlreadyExists(err) {
		return errors.Wrapf(err, "failed to save ceph osd encryption key as a secret for pvc %q", pvcName)
	}

	return nil
}

// updateSecretInKubernetes updates the dmcrypt key in a Kubernetes Secret
func (c *Config) updateSecretInKubernetes(pvcName, key string) error {
	secretName := GenerateOSDEncryptionSecretName(pvcName)
	secret, err := c.context.Clientset.CoreV1().Secrets(c.ClusterInfo.Namespace).Get(c.ClusterInfo.Context, secretName, metav1.GetOptions{})
	if err != nil {
		return errors.Wrapf(err, "failed to get secret %q", secretName)
	}

	secret.StringData = map[string]string{OsdEncryptionSecretNameKeyName: key}
	// Update the Kubernetes Secret
	_, err = c.context.Clientset.CoreV1().Secrets(c.ClusterInfo.Namespace).Update(c.ClusterInfo.Context, secret, metav1.UpdateOptions{})
	if err != nil {
		return errors.Wrapf(err, "failed to update ceph osd encryption key for pvc %q", pvcName)
	}

	return nil
}

// getKubernetesSecret returns key value from secret.
func (c *Config) getKubernetesSecret(pvcName string) (string, error) {
	secretName := GenerateOSDEncryptionSecretName(pvcName)
	secret, err := c.context.Clientset.CoreV1().Secrets(c.ClusterInfo.Namespace).Get(c.ClusterInfo.Context, secretName, metav1.GetOptions{})
	if err != nil {
		return "", errors.Wrapf(err, "failed to get secret %q", secretName)
	}

	return string(secret.Data[OsdEncryptionSecretNameKeyName]), nil
}

func generateOSDEncryptedKeySecret(pvcName, key string, clusterInfo *cephclient.ClusterInfo) (*v1.Secret, error) {
	s := &v1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      GenerateOSDEncryptionSecretName(pvcName),
			Namespace: clusterInfo.Namespace,
			Labels: map[string]string{
				"pvc_name": pvcName,
			},
		},
		StringData: map[string]string{
			OsdEncryptionSecretNameKeyName: key,
		},
		Type: k8sutil.RookType,
	}

	// Set the ownerref to the Secret
	err := clusterInfo.OwnerInfo.SetControllerReference(s)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to set owner reference to osd encryption key secret %q", s.Name)
	}

	return s, nil
}

// GenerateOSDEncryptionSecretName generate the Kubernetes Secret name of the encrypted key
func GenerateOSDEncryptionSecretName(pvcName string) string {
	return fmt.Sprintf("%s-%s", osdEncryptionSecretNamePrefix, pvcName)
}

// IsK8s determines whether the configured KMS is Kubernetes
func (c *Config) IsK8s() bool {
	return c.Provider == "kubernetes" || c.Provider == "k8s"
}
