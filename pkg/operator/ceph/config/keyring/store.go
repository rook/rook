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
	"fmt"

	"github.com/coreos/pkg/capnslog"
	"github.com/rook/rook/pkg/clusterd"
	"github.com/rook/rook/pkg/daemon/ceph/client"
	"github.com/rook/rook/pkg/operator/k8sutil"
	"k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var logger = capnslog.NewPackageLogger("github.com/rook/rook", "op-cfg-keyring")

const (
	keyKeyName      = "key"
	keyringFileName = "keyring"
)

// SecretStore is a helper to store Ceph daemon keyrings as Kubernetes secrets.
type SecretStore struct {
	context   *clusterd.Context
	namespace string
	ownerRef  *metav1.OwnerReference
}

// GetSecretStore returns a new SecretStore struct.
func GetSecretStore(context *clusterd.Context, namespace string, ownerRef *metav1.OwnerReference) *SecretStore {
	return &SecretStore{
		context:   context,
		namespace: namespace,
		ownerRef:  ownerRef,
	}
}

func keySecretName(resourceName string) string {
	return resourceName + "-key" // all keys named by suffixing key to the resource name
}

func keyringSecretName(resourceName string) string {
	return resourceName + "-keyring" // all keyrings named by suffixing keyring to the resource name
}

// GenerateKey generates a key for a Ceph user with the given access permissions. It returns the key
// generated on success. Ceph will always return the most up-to-date key for a daemon, and the key
// usually does not change.
func (k *SecretStore) GenerateKey(resourceName, user string, access []string) (string, error) {
	// get-or-create-key for the user account
	key, err := client.AuthGetOrCreateKey(k.context, k.namespace, user, access)
	if err != nil {
		logger.Infof("Error getting or creating key for %s. "+
			"Attempting to update capabilities in case the user already exists. %+v", user, err)
		uErr := client.AuthUpdateCaps(k.context, k.namespace, user, access)
		if uErr != nil {
			return "", fmt.Errorf("failed to get, create, or update auth key for %s. %+v", user, err)
		}
		key, uErr = client.AuthGetKey(k.context, k.namespace, user)
		if uErr != nil {
			return "", fmt.Errorf("failed to get key after updating existing auth capabilities for %s. %+v", user, err)
		}
	}
	return key, nil
}

// CreateOrUpdate creates or updates the keyring secret for the resource with the keyring specified.
// WARNING: Do not use "rook-ceph-admin" as the resource name; conflicts with the AdminStore.
func (k *SecretStore) CreateOrUpdate(resourceName, keyring string) error {
	secret := &v1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      keyringSecretName(resourceName),
			Namespace: k.namespace,
		},
		StringData: map[string]string{
			keyringFileName: keyring,
		},
		Type: k8sutil.RookType,
	}
	k8sutil.SetOwnerRef(k.context.Clientset, k.namespace, &secret.ObjectMeta, k.ownerRef)

	return k.createSecret(secret)
}

// Delete deletes the keyring secret for the resource.
func (k *SecretStore) Delete(resourceName string) error {
	secretName := keyringSecretName(resourceName)
	err := k.context.Clientset.CoreV1().Secrets(k.namespace).Delete(secretName, &metav1.DeleteOptions{})
	if err != nil && !errors.IsNotFound(err) {
		logger.Warningf("failed to delete keyring secret for %s. user may need to delete the resource manually. %+v", secretName, err)
	}

	return nil
}

func (k *SecretStore) createSecret(secret *v1.Secret) error {
	secretName := secret.ObjectMeta.Name
	_, err := k.context.Clientset.CoreV1().Secrets(k.namespace).Get(secretName, metav1.GetOptions{})
	if err != nil {
		if errors.IsNotFound(err) {
			logger.Debugf("creating secret for %s", secretName)
			if _, err := k.context.Clientset.CoreV1().Secrets(k.namespace).Create(secret); err != nil {
				return fmt.Errorf("failed to create secret for %s. %+v", secretName, err)
			}
			return nil
		}
		return fmt.Errorf("failed to get secret for %s. %+v", secretName, err)
	}
	logger.Debugf("updating secret for %s", secretName)
	if _, err := k.context.Clientset.CoreV1().Secrets(k.namespace).Update(secret); err != nil {
		return fmt.Errorf("failed to update secret for %s. %+v", secretName, err)
	}
	return nil
}
