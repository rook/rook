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
// Kubernetes.
package keyring

import (
	"fmt"
	"path"

	"github.com/coreos/pkg/capnslog"
	"github.com/rook/rook/pkg/clusterd"
	"github.com/rook/rook/pkg/operator/k8sutil"
	"k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
)

var logger = capnslog.NewPackageLogger("github.com/rook/rook", "op-cfg-keyring")

const (
	keyringStorePath = "/etc/ceph/keyring-store/"
	keyringFileName  = "keyring"
)

// SecretStore is a helper to store Ceph daemon keyrings as Kubernetes secrets.
type SecretStore struct {
	clientset kubernetes.Interface
	namespace string
	ownerRef  *metav1.OwnerReference
}

// GetSecretStore returns a new SecretStore struct.
func GetSecretStore(context *clusterd.Context, namespace string, ownerRef *metav1.OwnerReference) *SecretStore {
	return &SecretStore{
		clientset: context.Clientset,
		namespace: namespace,
		ownerRef:  ownerRef,
	}
}

func secretName(resourceName string) string {
	return resourceName + "-keyring" // all keyrings named by suffixing keyring to the resource name
}

// CreateOrUpdate creates or updates the keyring secret for the resource with the keyring specified.
func (k *SecretStore) CreateOrUpdate(resourceName, keyring string) error {
	secretName := secretName(resourceName)
	secret := &v1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      secretName,
			Namespace: k.namespace,
		},
		StringData: map[string]string{
			keyringFileName: keyring,
		},
		Type: k8sutil.RookType,
	}
	k8sutil.SetOwnerRef(k.clientset, k.namespace, &secret.ObjectMeta, k.ownerRef)

	_, err := k.clientset.CoreV1().Secrets(k.namespace).Get(secretName, metav1.GetOptions{})
	if err != nil {
		if errors.IsNotFound(err) {
			logger.Debugf("creating keyring secret for %s", secretName)
			if _, err := k.clientset.CoreV1().Secrets(k.namespace).Create(secret); err != nil {
				return fmt.Errorf("failed to create keyring secret for %s. %+v", secretName, err)
			}
		}
		return fmt.Errorf("failed to get keyring secret for %s. %+v", secretName, err)
	}

	logger.Debugf("updating keyring secret for %s", secretName)
	if _, err := k.clientset.CoreV1().Secrets(k.namespace).Update(secret); err != nil {
		return fmt.Errorf("failed to update keyring secret for %s. %+v", secretName, err)
	}

	return nil
}

// Delete deletes the keyring secret for the resource.
func (k *SecretStore) Delete(resourceName string) error {
	secretName := secretName(resourceName)
	err := k.clientset.CoreV1().Secrets(k.namespace).Delete(secretName, &metav1.DeleteOptions{})
	if err != nil && !errors.IsNotFound(err) {
		logger.Warningf("failed to delete keyring secret for %s. user may need to delete the resource manually. %+v", secretName, err)
	}

	return nil
}

// StoredVolume returns a pod volume that mounts the resource's keyring secret.
func StoredVolume(resourceName string) v1.Volume {
	return v1.Volume{
		Name: secretName(resourceName),
		VolumeSource: v1.VolumeSource{Secret: &v1.SecretVolumeSource{
			SecretName: secretName(resourceName),
		}},
	}
}

// StoredVolumeMount returns a volume mount that mounts the resource's keyring secret in the
// container at the given directory path. Keyring is mounted at `<dirPath>/keyring`.
func StoredVolumeMount(resourceName string) v1.VolumeMount {
	return v1.VolumeMount{
		Name:      secretName(resourceName),
		ReadOnly:  true, // should be no reason to write to the keyring in pods, so enforce this
		MountPath: keyringStorePath,
	}
}

// ContainerMountedFilePath is the path where the keyring file can be found in pods+containers with
// the volumes and volume mounts from the keyring store.
func ContainerMountedFilePath() string {
	return path.Join(keyringStorePath, keyringFileName)
}
