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

package csi

import (
	"github.com/pkg/errors"
	"github.com/rook/rook/pkg/clusterd"
	"github.com/rook/rook/pkg/operator/ceph/config/keyring"
	"github.com/rook/rook/pkg/operator/k8sutil"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	csiKeyringRBDProvisionerUsername = "client.csi-rbd-provisioner"
	csiKeyringRBDNodeUsername        = "client.csi-rbd-node"
	csiRBDNodeSecret                 = "rook-csi-rbd-node"
	csiRBDProvisionerSecret          = "rook-csi-rbd-provisioner"
)

const (
	csiKeyringCephFSProvisionerUsername = "client.csi-cephfs-provisioner"
	csiKeyringCephFSNodeUsername        = "client.csi-cephfs-node"
	csiCephFSNodeSecret                 = "rook-csi-cephfs-node"
	csiCephFSProvisionerSecret          = "rook-csi-cephfs-provisioner"
)

func createCSIKeyringRBDNode(s *keyring.SecretStore) (string, error) {
	key, err := s.GenerateKey(csiKeyringRBDNodeUsername, cephCSIKeyringRBDNodeCaps())
	if err != nil {
		return "", err
	}

	return key, nil
}

func createCSIKeyringRBDProvisioner(s *keyring.SecretStore) (string, error) {
	key, err := s.GenerateKey(csiKeyringRBDProvisionerUsername, cephCSIKeyringRBDProvisionerCaps())
	if err != nil {
		return "", err
	}

	return key, nil
}

func createCSIKeyringCephFSNode(s *keyring.SecretStore) (string, error) {
	key, err := s.GenerateKey(csiKeyringCephFSNodeUsername, cephCSIKeyringCephFSNodeCaps())
	if err != nil {
		return "", err
	}

	return key, nil
}

func createCSIKeyringCephFSProvisioner(s *keyring.SecretStore) (string, error) {
	key, err := s.GenerateKey(csiKeyringCephFSProvisionerUsername, cephCSIKeyringCephFSProvisionerCaps())
	if err != nil {
		return "", err
	}

	return key, nil
}

func cephCSIKeyringRBDNodeCaps() []string {
	return []string{
		"mon", "profile rbd",
		"osd", "profile rbd",
	}
}

func cephCSIKeyringRBDProvisionerCaps() []string {
	return []string{
		"mon", "profile rbd",
		"mgr", "allow rw",
		"osd", "profile rbd",
	}
}

func cephCSIKeyringCephFSNodeCaps() []string {
	return []string{
		"mon", "allow r",
		"mgr", "allow rw",
		"osd", "allow rw tag cephfs *=*",
		"mds", "allow rw",
	}
}

func cephCSIKeyringCephFSProvisionerCaps() []string {
	return []string{
		"mon", "allow r",
		"mgr", "allow rw",
		"osd", "allow rw tag cephfs metadata=*",
	}
}

func createOrUpdateCSISecret(namespace, csiRBDProvisionerSecretKey, csiRBDNodeSecretKey, csiCephFSProvisionerSecretKey, csiCephFSNodeSecretKey string, k *keyring.SecretStore, ownerRef *metav1.OwnerReference) error {
	csiRBDProvisionerSecrets := map[string][]byte{
		// userID is expected for the rbd provisioner driver
		"userID":  []byte("csi-rbd-provisioner"),
		"userKey": []byte(csiRBDProvisionerSecretKey),
	}

	csiRBDNodeSecrets := map[string][]byte{
		// userID is expected for the rbd node driver
		"userID":  []byte("csi-rbd-node"),
		"userKey": []byte(csiRBDNodeSecretKey),
	}

	csiCephFSProvisionerSecrets := map[string][]byte{
		// adminID is expected for the cephfs provisioner driver
		"adminID":  []byte("csi-cephfs-provisioner"),
		"adminKey": []byte(csiCephFSProvisionerSecretKey),
	}

	csiCephFSNodeSecrets := map[string][]byte{
		// adminID is expected for the cephfs node driver
		"adminID":  []byte("csi-cephfs-node"),
		"adminKey": []byte(csiCephFSNodeSecretKey),
	}

	keyringSecretMap := make(map[string]map[string][]byte)
	keyringSecretMap[csiRBDProvisionerSecret] = csiRBDProvisionerSecrets
	keyringSecretMap[csiRBDNodeSecret] = csiRBDNodeSecrets
	keyringSecretMap[csiCephFSProvisionerSecret] = csiCephFSProvisionerSecrets
	keyringSecretMap[csiCephFSNodeSecret] = csiCephFSNodeSecrets

	for secretName, secret := range keyringSecretMap {
		s := &v1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name:      secretName,
				Namespace: namespace,
			},
			Data: secret,
			Type: k8sutil.RookType,
		}
		k8sutil.SetOwnerRef(&s.ObjectMeta, ownerRef)

		// Create Kubernetes Secret
		err := k.CreateSecret(s)
		if err != nil {
			return errors.Wrapf(err, "failed to create kubernetes secret %q for cluster %q", secret, namespace)
		}

	}

	logger.Infof("created kubernetes csi secrets for cluster %q", namespace)
	return nil
}

// CreateCSISecrets creates all the Kubernetes CSI Secrets
func CreateCSISecrets(context *clusterd.Context, clusterName string, ownerRef *metav1.OwnerReference) error {
	k := keyring.GetSecretStore(context, clusterName, ownerRef)

	// Create CSI RBD Provisioner Ceph key
	csiRBDProvisionerSecretKey, err := createCSIKeyringRBDProvisioner(k)
	if err != nil {
		return errors.Wrapf(err, "failed to create csi rbd provisioner ceph keyring")
	}

	// Create CSI RBD Node Ceph key
	csiRBDNodeSecretKey, err := createCSIKeyringRBDNode(k)
	if err != nil {
		return errors.Wrapf(err, "failed to create csi rbd node ceph keyring")
	}

	// Create CSI Cephfs provisioner Ceph key
	csiCephFSProvisionerSecretKey, err := createCSIKeyringCephFSProvisioner(k)
	if err != nil {
		return errors.Wrapf(err, "failed to create csi cephfs provisioner ceph keyring")
	}

	// Create CSI Cephfs node Ceph key
	csiCephFSNodeSecretKey, err := createCSIKeyringCephFSNode(k)
	if err != nil {
		return errors.Wrapf(err, "failed to create csi cephfs node ceph keyring")
	}

	// Create or update Kubernetes CSI secret
	if err := createOrUpdateCSISecret(clusterName, csiRBDProvisionerSecretKey, csiRBDNodeSecretKey, csiCephFSProvisionerSecretKey, csiCephFSNodeSecretKey, k, ownerRef); err != nil {
		return errors.Wrapf(err, "failed to create kubernetes csi secret")
	}

	return nil
}
