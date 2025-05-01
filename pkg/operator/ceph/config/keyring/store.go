/*
Copyright 2019 The Rook Authors. All rights reserved.

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

// Package keyring provides methods for accessing keyrings for Ceph daemons stored securely in
// Kubernetes secrets. It also provides methods for creating keyrings with desired permissions which
// are stored persistently and a special subset of methods for the Ceph admin keyring.
package keyring

import (
	"github.com/coreos/pkg/capnslog"
	"github.com/pkg/errors"
	"github.com/rook/rook/pkg/clusterd"
	"github.com/rook/rook/pkg/daemon/ceph/client"
	"github.com/rook/rook/pkg/operator/k8sutil"
	v1 "k8s.io/api/core/v1"
	kerrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var logger = capnslog.NewPackageLogger("github.com/rook/rook", "op-cfg-keyring")

const (
	keyringFileName = "keyring"

	// KeyringAnnotation identifies a Kubernetes Secret as a cephx keyring file
	KeyringAnnotation = "cephx-keyring"
)

// SecretStore is a helper to store Ceph daemon keyrings as Kubernetes secrets.
type SecretStore struct {
	context     *clusterd.Context
	clusterInfo *client.ClusterInfo
	ownerInfo   *k8sutil.OwnerInfo
}

// GetSecretStore returns a new SecretStore struct.
func GetSecretStore(context *clusterd.Context, clusterInfo *client.ClusterInfo, ownerInfo *k8sutil.OwnerInfo) *SecretStore {
	return &SecretStore{
		context:     context,
		clusterInfo: clusterInfo,
		ownerInfo:   ownerInfo,
	}
}

func keyringSecretName(resourceName string) string {
	return resourceName + "-keyring" // all keyrings named by suffixing keyring to the resource name
}

// GenerateKey generates a key for a Ceph user with the given access permissions. It returns the key
// generated on success. Ceph will always return the most up-to-date key for a daemon, and the key
// usually does not change.
func (k *SecretStore) GenerateKey(user string, access []string) (string, error) {
	// get-or-create-key for the user account
	key, err := client.AuthGetOrCreateKey(k.context, k.clusterInfo, user, access)
	if err != nil {
		logger.Infof("Error getting or creating key for %q. "+
			"Attempting to update capabilities in case the user already exists. %v", user, err)
		uErr := client.AuthUpdateCaps(k.context, k.clusterInfo, user, access)
		if uErr != nil {
			return "", errors.Wrapf(err, "failed to get, create, or update auth key for %s", user)
		}
		key, uErr = client.AuthGetKey(k.context, k.clusterInfo, user)
		if uErr != nil {
			return "", errors.Wrapf(err, "failed to get key after updating existing auth capabilities for %s", user)
		}
	}
	return key, nil
}

// RotateKey rotates a key for a Ceph user without modifying permissions. It returns the new key on success.
func (k *SecretStore) RotateKey(user string) (string, error) {
	key, err := client.AuthRotate(k.context, k.clusterInfo, user)
	if err != nil {
		return "", errors.Wrapf(err, "failed to rotate key for %q", user)
	}
	return key, nil
}

// CreateOrUpdate creates or updates the keyring secret for the resource with the keyring specified.
// Returns the secret resource version.
// WARNING: Do not use "rook-ceph-admin" as the resource name; conflicts with the AdminStore.
func (k *SecretStore) CreateOrUpdate(resourceName string, keyring string) (string, error) {
	secret := &v1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      keyringSecretName(resourceName),
			Namespace: k.clusterInfo.Namespace,
			Annotations: map[string]string{
				KeyringAnnotation: "",
			},
		},
		StringData: map[string]string{
			keyringFileName: keyring,
		},
		Type: k8sutil.RookType,
	}
	err := k.ownerInfo.SetControllerReference(secret)
	if err != nil {
		return "", errors.Wrapf(err, "failed to set owner reference to keyring secret %q", secret.Name)
	}

	return k.CreateSecret(secret)
}

// Delete deletes the keyring secret for the resource.
func (k *SecretStore) Delete(resourceName string) error {
	secretName := keyringSecretName(resourceName)
	err := k.context.Clientset.CoreV1().Secrets(k.clusterInfo.Namespace).Delete(k.clusterInfo.Context, secretName, metav1.DeleteOptions{})
	if err != nil && !kerrors.IsNotFound(err) {
		logger.Warningf("failed to delete keyring secret for %q. user may need to delete the resource manually. %v", secretName, err)
	}

	return nil
}

// CreateSecret creates or update a kubernetes secret.
// Returns the resource version of the secret.
func (k *SecretStore) CreateSecret(secret *v1.Secret) (string, error) {
	secretName := secret.ObjectMeta.Name
	_, err := k.context.Clientset.CoreV1().Secrets(k.clusterInfo.Namespace).Get(k.clusterInfo.Context, secretName, metav1.GetOptions{})
	if err != nil {
		if kerrors.IsNotFound(err) {
			logger.Debugf("creating secret for %s", secretName)
			s, err := k.context.Clientset.CoreV1().Secrets(k.clusterInfo.Namespace).Create(k.clusterInfo.Context, secret, metav1.CreateOptions{})
			if err != nil {
				return "", errors.Wrapf(err, "failed to create secret for %s", secretName)
			}
			return s.ResourceVersion, nil
		}
		return "", errors.Wrapf(err, "failed to get secret for %s", secretName)
	}

	logger.Debugf("updating secret for %s", secretName)
	s, err := k.context.Clientset.CoreV1().Secrets(k.clusterInfo.Namespace).Update(k.clusterInfo.Context, secret, metav1.UpdateOptions{})
	if err != nil {
		return "", errors.Wrapf(err, "failed to update secret for %s", secretName)
	}
	return s.ResourceVersion, nil
}
