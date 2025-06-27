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
	cephv1 "github.com/rook/rook/pkg/apis/ceph.rook.io/v1"
	"github.com/rook/rook/pkg/clusterd"
	"github.com/rook/rook/pkg/daemon/ceph/client"
	"github.com/rook/rook/pkg/operator/ceph/config/keyring"
	"github.com/rook/rook/pkg/operator/ceph/reporting"
	"github.com/rook/rook/pkg/operator/k8sutil"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/util/retry"
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

func createCSIKeyringRBDNode(context *clusterd.Context, clusterInfo *client.ClusterInfo, s *keyring.SecretStore, keepPriorCount uint32, shouldRotateCephxKeys bool) (string, error) {
	if shouldRotateCephxKeys {
		if keepPriorCount > 0 {
			key, err := s.GenerateKey(appendPriorKeyCountToSecretName(csiKeyringRBDNodeUsername, keepPriorCount), cephCSIKeyringRBDNodeCaps())
			if err != nil {
				return "", err
			}
			return key, nil
		}

		err := client.AuthDelete(context, clusterInfo, csiKeyringRBDNodeUsername)
		if err != nil {
			return "", errors.Wrapf(err, "failed to delete older RBD node username key %s", csiKeyringRBDNodeUsername)
		}
	}

	key, err := s.GenerateKey(csiKeyringRBDNodeUsername, cephCSIKeyringRBDNodeCaps())
	if err != nil {
		return "", err
	}

	return key, nil
}

func createCSIKeyringRBDProvisioner(context *clusterd.Context, clusterInfo *client.ClusterInfo, s *keyring.SecretStore, keepPriorCount uint32, shouldRotateCephxKeys bool) (string, error) {
	if shouldRotateCephxKeys {
		if keepPriorCount > 0 {
			key, err := s.GenerateKey(appendPriorKeyCountToSecretName(csiKeyringRBDProvisionerUsername, keepPriorCount), cephCSIKeyringRBDProvisionerCaps())
			if err != nil {
				return "", err
			}
			return key, nil
		}

		err := client.AuthDelete(context, clusterInfo, csiKeyringRBDProvisionerUsername)
		if err != nil {
			return "", errors.Wrapf(err, "failed to delete older RBD provisioner username key %s", csiKeyringRBDProvisionerUsername)
		}
	}

	key, err := s.GenerateKey(csiKeyringRBDProvisionerUsername, cephCSIKeyringRBDProvisionerCaps())
	if err != nil {
		return "", err
	}

	return key, nil
}

func createCSIKeyringCephFSNode(context *clusterd.Context, clusterInfo *client.ClusterInfo, s *keyring.SecretStore, keepPriorCount uint32, shouldRotateCephxKeys bool) (string, error) {
	if shouldRotateCephxKeys {
		if keepPriorCount > 0 {
			key, err := s.GenerateKey(appendPriorKeyCountToSecretName(csiKeyringCephFSNodeUsername, keepPriorCount), cephCSIKeyringCephFSNodeCaps())
			if err != nil {
				return "", err
			}
			return key, nil
		}

		err := client.AuthDelete(context, clusterInfo, csiKeyringCephFSNodeUsername)
		if err != nil {
			return "", errors.Wrapf(err, "failed to delete older CephFS node username key %s", csiKeyringCephFSNodeUsername)
		}
	}

	key, err := s.GenerateKey(csiKeyringCephFSNodeUsername, cephCSIKeyringCephFSNodeCaps())
	if err != nil {
		return "", err
	}

	return key, nil
}

func createCSIKeyringCephFSProvisioner(context *clusterd.Context, clusterInfo *client.ClusterInfo, s *keyring.SecretStore, keepPriorCount uint32, shouldRotateCephxKeys bool) (string, error) {
	if shouldRotateCephxKeys {
		if keepPriorCount > 0 {
			key, err := s.GenerateKey(appendPriorKeyCountToSecretName(csiKeyringCephFSProvisionerUsername, keepPriorCount), cephCSIKeyringCephFSProvisionerCaps())
			if err != nil {
				return "", err
			}
			return key, nil
		}

		err := client.AuthDelete(context, clusterInfo, csiKeyringCephFSProvisionerUsername)
		if err != nil {
			return "", errors.Wrapf(err, "failed to delete older CephFS provisioner username key %s", csiKeyringCephFSProvisionerUsername)
		}
	}

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

func createOrUpdateCSISecret(clusterInfo *client.ClusterInfo, csiRBDProvisionerSecretKey, csiRBDNodeSecretKey, csiCephFSProvisionerSecretKey, csiCephFSNodeSecretKey string, k *keyring.SecretStore) error {
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
		_, err = k.CreateSecret(s)
		if err != nil {
			return errors.Wrapf(err, "failed to create kubernetes secret %q for cluster %q", s.Name, clusterInfo.Namespace)
		}

	}

	logger.Infof("created kubernetes csi secrets for cluster %q", clusterInfo.Namespace)
	return nil
}

func appendPriorKeyCountToSecretName(csiSecretKeyName string, priorKeyCount uint32) string {
	if priorKeyCount > 0 {
		return fmt.Sprintf("%s-%d", csiSecretKeyName, priorKeyCount)
	}
	return csiSecretKeyName
}

// CreateCSISecrets creates all the Kubernetes CSI Secrets
func CreateCSISecrets(context *clusterd.Context, clusterInfo *client.ClusterInfo, clusterSpec *cephv1.ClusterSpec, clusterNamespaced types.NamespacedName) error {
	k := keyring.GetSecretStore(context, clusterInfo, clusterInfo.OwnerInfo)
	csiCephXConfig := clusterSpec.Security.CephX

	// In case of CSI or overlapping key rotation, we don't need desired cephVersion as key rotation on `WithCephVersionUpdate` is not supported.
	// We can pass any cephVersion as a place holder to the `ShouldRotateCephxKeys`.
	shouldRotateCephxKeys, err := keyring.ShouldRotateCephxKeys(
		csiCephXConfig.CSI.CephxConfig, clusterInfo.CephVersion, clusterInfo.CephVersion, cephv1.CephxStatus{})
	if err != nil {
		return errors.Wrap(err, "failed to determine if cephx keys should be rotated")
	}

	if shouldRotateCephxKeys {
		logger.Infof("cephx keys for CSI daemons in namespace %q will be rotated", clusterInfo.Namespace)
	}

	// Create CSI RBD Provisioner Ceph key
	csiRBDProvisionerSecretKey, err := createCSIKeyringRBDProvisioner(context, clusterInfo, k, csiCephXConfig.CSI.KeepPriorKeyCount, shouldRotateCephxKeys)
	if err != nil {
		return errors.Wrap(err, "failed to create csi rbd provisioner ceph keyring")
	}

	// Create CSI RBD Node Ceph key
	csiRBDNodeSecretKey, err := createCSIKeyringRBDNode(context, clusterInfo, k, csiCephXConfig.CSI.KeepPriorKeyCount, shouldRotateCephxKeys)
	if err != nil {
		return errors.Wrap(err, "failed to create csi rbd node ceph keyring")
	}

	// Create CSI Cephfs provisioner Ceph key
	csiCephFSProvisionerSecretKey, err := createCSIKeyringCephFSProvisioner(context, clusterInfo, k, csiCephXConfig.CSI.KeepPriorKeyCount, shouldRotateCephxKeys)
	if err != nil {
		return errors.Wrap(err, "failed to create csi cephfs provisioner ceph keyring")
	}

	// Create CSI Cephfs node Ceph key
	csiCephFSNodeSecretKey, err := createCSIKeyringCephFSNode(context, clusterInfo, k, csiCephXConfig.CSI.KeepPriorKeyCount, shouldRotateCephxKeys)
	if err != nil {
		return errors.Wrap(err, "failed to create csi cephfs node ceph keyring")
	}

	// Create or update Kubernetes CSI secret
	if err := createOrUpdateCSISecret(clusterInfo, csiRBDProvisionerSecretKey, csiRBDNodeSecretKey, csiCephFSProvisionerSecretKey, csiCephFSNodeSecretKey, k); err != nil {
		return errors.Wrap(err, "failed to create kubernetes csi secret")
	}

	err = updateCephStatusWithCephxStatus(context, clusterInfo, clusterSpec, clusterNamespaced, shouldRotateCephxKeys)
	if err != nil {
		return err
	}

	return nil
}

func updateCephStatusWithCephxStatus(context *clusterd.Context, clusterInfo *client.ClusterInfo, clusterSpec *cephv1.ClusterSpec, name types.NamespacedName, shouldRotateCephxKeys bool) error {
	cephCluster := &cephv1.CephCluster{}
	err := retry.OnError(
		retry.DefaultRetry,
		func(err error) bool {
			return err != nil
		},
		func() error {
			if getErr := context.Client.Get(clusterInfo.Context, name, cephCluster); getErr != nil {
				logger.Errorf("failed to retrieve cephCluster %q in namespace %q to update csi cephx status. %v", name, name.Namespace, getErr)
				return getErr
			}

			cephxStatus := keyring.UpdatedCephxStatus(shouldRotateCephxKeys, clusterSpec.Security.CephX.Daemon, clusterInfo.CephVersion, cephCluster.Status.Cephx.CSI)
			cephCluster.Status.Cephx.CSI = cephxStatus

			if getErr := reporting.UpdateStatus(context.Client, cephCluster); getErr != nil {
				logger.Errorf("failed to update cephCluster %q to update csi cephx status to %q in namespace %q with error %v", name.Name, cephxStatus, name.Namespace, getErr)
				return getErr
			}
			return nil
		})
	if err != nil {
		return errors.Wrapf(err, "failed to get and update Ceph cluster %q status in namespace %q when updating csi cephx status", name.Name, name.Namespace)
	}

	logger.Info("successfully update Ceph cluster %q status with CSI Cephx status in namespace %q", name.Name, name.Namespace)
	return nil
}
