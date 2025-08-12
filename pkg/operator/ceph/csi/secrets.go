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
	"fmt"

	"github.com/pkg/errors"
	"github.com/rook/rook/pkg/clusterd"
	"github.com/rook/rook/pkg/daemon/ceph/client"
	"github.com/rook/rook/pkg/operator/ceph/config/keyring"
	"github.com/rook/rook/pkg/operator/k8sutil"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

//nolint:gosec // because of the word `Secret`
const (
	csiKeyringRBDProvisionerUsername = "client.csi-rbd-provisioner"
	csiKeyringRBDNodeUsername        = "client.csi-rbd-node"
	CsiRBDNodeSecret                 = "rook-csi-rbd-node"
	CsiRBDProvisionerSecret          = "rook-csi-rbd-provisioner"
)

//nolint:gosec // because of the word `Secret`
const (
	csiKeyringCephFSProvisionerUsername = "client.csi-cephfs-provisioner"
	csiKeyringCephFSNodeUsername        = "client.csi-cephfs-node"
	CsiCephFSNodeSecret                 = "rook-csi-cephfs-node"
	CsiCephFSProvisionerSecret          = "rook-csi-cephfs-provisioner"
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
		"mgr", "allow rw",
		"osd", "profile rbd",
	}
}

func cephCSIKeyringRBDProvisionerCaps() []string {
	return []string{
		"mon", "profile rbd, allow command 'osd blocklist'",
		"mgr", "allow rw",
		"osd", "profile rbd",
	}
}

func cephCSIKeyringCephFSNodeCaps() []string {
	return []string{
		"mon", "allow r",
		"mgr", "allow rw",
		"osd", "allow rwx tag cephfs metadata=*, allow rw tag cephfs data=*",
		"mds", "allow rw",
	}
}

func cephCSIKeyringCephFSProvisionerCaps() []string {
	return []string{
		"mon", "allow r, allow command 'osd blocklist'",
		"mgr", "allow rw",
		"osd", "allow rw tag cephfs metadata=*",
		// MDS require all(*) permissions to be able to execute admin socket commands like ceph tell required for client eviction in cephFS.
		"mds", "allow *",
	}
}

<<<<<<< HEAD
func createOrUpdateCSISecret(clusterInfo *client.ClusterInfo, csiRBDProvisionerSecretKey, csiRBDNodeSecretKey, csiCephFSProvisionerSecretKey, csiCephFSNodeSecretKey string, k *keyring.SecretStore) error {
	csiRBDProvisionerSecrets := map[string][]byte{
		// userID is expected for the rbd provisioner driver
		"userID":  []byte("csi-rbd-provisioner"),
		"userKey": []byte(csiRBDProvisionerSecretKey),
=======
func createOrUpdateCSISecret(clusterInfo *client.ClusterInfo, csiSecretContent csiSecretStore, k *keyring.SecretStore) error {
	const userID = "userID"
	const userKey = "userKey"
	csiRBDProvisionerSecrets := map[string][]byte{
		// userID is expected for the rbd provisioner driver
		userID:  []byte(csiSecretContent[CsiRBDProvisionerSecret].Name),
		userKey: []byte(csiSecretContent[CsiRBDProvisionerSecret].Key),
>>>>>>> cb89f351e (csi: update cephfs user and key in secret)
	}

	csiRBDNodeSecrets := map[string][]byte{
		// userID is expected for the rbd node driver
<<<<<<< HEAD
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
=======
		userID:  []byte(csiSecretContent[CsiRBDNodeSecret].Name),
		userKey: []byte(csiSecretContent[CsiRBDNodeSecret].Key),
	}

	csiCephFSProvisionerSecrets := map[string][]byte{
		// userID is expected for the cephfs provisioner driver
		userID:  []byte(csiSecretContent[CsiCephFSProvisionerSecret].Name),
		userKey: []byte(csiSecretContent[CsiCephFSProvisionerSecret].Key),
	}

	csiCephFSNodeSecrets := map[string][]byte{
		// userID is expected for the cephfs node driver
		userID:  []byte(csiSecretContent[CsiCephFSNodeSecret].Name),
		userKey: []byte(csiSecretContent[CsiCephFSNodeSecret].Key),
>>>>>>> cb89f351e (csi: update cephfs user and key in secret)
	}

	keyringSecretMap := make(map[string]map[string][]byte)
	keyringSecretMap[CsiRBDProvisionerSecret] = csiRBDProvisionerSecrets
	keyringSecretMap[CsiRBDNodeSecret] = csiRBDNodeSecrets
	keyringSecretMap[CsiCephFSProvisionerSecret] = csiCephFSProvisionerSecrets
	keyringSecretMap[CsiCephFSNodeSecret] = csiCephFSNodeSecrets

	for secretName, secret := range keyringSecretMap {
		s := &v1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name:      secretName,
				Namespace: clusterInfo.Namespace,
			},
			Data: secret,
			Type: k8sutil.RookType,
		}
		err := clusterInfo.OwnerInfo.SetControllerReference(s)
		if err != nil {
			return errors.Wrapf(err, "failed to set owner reference to keyring secret %q", secretName)
		}

		// Create Kubernetes Secret
		err = k.CreateSecret(s)
		if err != nil {
			return errors.Wrapf(err, "failed to create kubernetes secret %q for cluster %q", s.Name, clusterInfo.Namespace)
		}

	}

	logger.Infof("created kubernetes csi secrets for cluster %q", clusterInfo.Namespace)
	return nil
}

// CreateCSISecrets creates all the Kubernetes CSI Secrets
func CreateCSISecrets(context *clusterd.Context, clusterInfo *client.ClusterInfo) error {
	if clusterInfo.CSIDriverSpec.SkipUserCreation {
		if err := deleteOwnedCSISecretsByCephCluster(context, clusterInfo); err != nil {
			return err
		}
		logger.Info("CSI user creation is disabled; skipping user and secret creation")
		return nil
	}
	k := keyring.GetSecretStore(context, clusterInfo, clusterInfo.OwnerInfo)

	// Create CSI RBD Provisioner Ceph key
	csiRBDProvisionerSecretKey, err := createCSIKeyringRBDProvisioner(k)
	if err != nil {
		return errors.Wrap(err, "failed to create csi rbd provisioner ceph keyring")
	}

	// Create CSI RBD Node Ceph key
	csiRBDNodeSecretKey, err := createCSIKeyringRBDNode(k)
	if err != nil {
		return errors.Wrap(err, "failed to create csi rbd node ceph keyring")
	}

	// Create CSI Cephfs provisioner Ceph key
	csiCephFSProvisionerSecretKey, err := createCSIKeyringCephFSProvisioner(k)
	if err != nil {
		return errors.Wrap(err, "failed to create csi cephfs provisioner ceph keyring")
	}

	// Create CSI Cephfs node Ceph key
	csiCephFSNodeSecretKey, err := createCSIKeyringCephFSNode(k)
	if err != nil {
		return errors.Wrap(err, "failed to create csi cephfs node ceph keyring")
	}

	// Create or update Kubernetes CSI secret
	if err := createOrUpdateCSISecret(clusterInfo, csiRBDProvisionerSecretKey, csiRBDNodeSecretKey, csiCephFSProvisionerSecretKey, csiCephFSNodeSecretKey, k); err != nil {
		return errors.Wrap(err, "failed to create kubernetes csi secret")
	}

	return nil
}

func deleteOwnedCSISecretsByCephCluster(context *clusterd.Context, clusterInfo *client.ClusterInfo) error {
	ownerRef := metav1.OwnerReference{
		APIVersion: "ceph.rook.io/v1",
		Kind:       "CephCluster",
		Name:       clusterInfo.NamespacedName().Name,
	}
	secrets := []string{CsiRBDNodeSecret, CsiRBDProvisionerSecret, CsiCephFSNodeSecret, CsiCephFSProvisionerSecret}
	for _, secretName := range secrets {
		err := k8sutil.DeleteSecretIfOwnedBy(
			clusterInfo.Context,
			context.Clientset,
			secretName,
			clusterInfo.Namespace,
			ownerRef,
		)
		if err != nil {
			return fmt.Errorf("failed to delete secret %q: %w", secretName, err)
		}
	}

	return nil
}
