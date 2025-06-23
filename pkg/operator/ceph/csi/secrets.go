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
	"sort"
	"strconv"
	"strings"

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

func createCSIKeyringRBDNode(context *clusterd.Context, clusterInfo *client.ClusterInfo, clusterSpec *cephv1.ClusterSpec, s *keyring.SecretStore, keepPriorCount uint32, keyRotationEnabled bool) (string, error) {
	if keyRotationEnabled {
		shouldRotate, allKeyWithBaseName, err := handleCsiKeys(context, clusterInfo, clusterSpec, csiKeyringRBDNodeUsername)
		if err != nil {
			return "", errors.Wrapf(err, "failed to check if key rotation is supported for CSI key %s", csiKeyringRBDNodeUsername)
		}

		var key string
		if shouldRotate {
			key, err = s.GenerateKey(appendPriorKeyCountToSecretName(csiKeyringRBDNodeUsername, keepPriorCount), cephCSIKeyringRBDNodeCaps())
			if err != nil {
				return "", errors.Wrapf(err, "failed to check if keys should be rotated for CSI key %s", csiKeyringRBDNodeUsername)
			}

			err = deleteOldKeyGen(context, clusterInfo, allKeyWithBaseName, int(keepPriorCount))
			if err != nil {
				logger.Errorf("failed to delete keys during CSI key rotation.%v", err)
			}
			return key, nil
		}

		key, err = s.GenerateKey(appendPriorKeyCountToSecretName(csiKeyringRBDNodeUsername, 1), cephCSIKeyringRBDNodeCaps())
		if err != nil {
			return "", errors.Wrapf(err, "failed to check if keys should be rotated for CSI key %s", csiKeyringRBDNodeUsername)
		}
		return key, nil
	}

	key, err := s.GenerateKey(csiKeyringRBDNodeUsername, cephCSIKeyringRBDNodeCaps())
	if err != nil {
		return "", err
	}

	return key, nil
}

func createCSIKeyringRBDProvisioner(context *clusterd.Context, clusterInfo *client.ClusterInfo, clusterSpec *cephv1.ClusterSpec, s *keyring.SecretStore, keepPriorCount uint32, keyRotationEnabled bool) (string, error) {
	if keyRotationEnabled {
		shouldRotate, allKeyWithBaseName, err := handleCsiKeys(context, clusterInfo, clusterSpec, csiKeyringRBDProvisionerUsername)
		if err != nil {
			return "", errors.Wrapf(err, "failed to check if key rotation is supported for CSI key %s", csiKeyringRBDProvisionerUsername)
		}

		var key string
		if shouldRotate {
			key, err = s.GenerateKey(appendPriorKeyCountToSecretName(csiKeyringRBDProvisionerUsername, keepPriorCount), cephCSIKeyringRBDProvisionerCaps())
			if err != nil {
				return "", errors.Wrapf(err, "failed to check if keys should to be rotated for CSI key %s", csiKeyringRBDProvisionerUsername)
			}

			err = deleteOldKeyGen(context, clusterInfo, allKeyWithBaseName, int(keepPriorCount))
			if err != nil {
				logger.Errorf("failed to delete keys during CSI key rotation.%v", err)
			}
			return key, nil
		}

		key, err = s.GenerateKey(appendPriorKeyCountToSecretName(csiKeyringRBDProvisionerUsername, 1), cephCSIKeyringRBDProvisionerCaps())
		if err != nil {
			return "", errors.Wrapf(err, "failed to check if keys should to be rotated for CSI key %s", csiKeyringRBDProvisionerUsername)
		}
		return key, nil
	}

	key, err := s.GenerateKey(csiKeyringRBDProvisionerUsername, cephCSIKeyringRBDProvisionerCaps())
	if err != nil {
		return "", err
	}

	return key, nil
}

func createCSIKeyringCephFSNode(context *clusterd.Context, clusterInfo *client.ClusterInfo, clusterSpec *cephv1.ClusterSpec, s *keyring.SecretStore, keepPriorCount uint32, keyRotationEnabled bool) (string, error) {
	if keyRotationEnabled {
		shouldRotate, allKeyWithBaseName, err := handleCsiKeys(context, clusterInfo, clusterSpec, csiKeyringCephFSNodeUsername)
		if err != nil {
			return "", errors.Wrapf(err, "failed to check if key rotation is supported for CSI key %s", csiKeyringCephFSNodeUsername)
		}

		var key string
		if shouldRotate {
			key, err = s.GenerateKey(appendPriorKeyCountToSecretName(csiKeyringCephFSNodeUsername, keepPriorCount), cephCSIKeyringCephFSNodeCaps())
			if err != nil {
				return "", errors.Wrapf(err, "failed to check if keys should to be rotated for CSI key %s", csiKeyringCephFSNodeUsername)
			}

			err = deleteOldKeyGen(context, clusterInfo, allKeyWithBaseName, int(keepPriorCount))
			if err != nil {
				logger.Errorf("failed to delete keys during CSI key rotation.%v", err)
			}
			return key, nil
		}

		key, err = s.GenerateKey(appendPriorKeyCountToSecretName(csiKeyringCephFSNodeUsername, 1), cephCSIKeyringRBDNodeCaps())
		if err != nil {
			return "", errors.Wrapf(err, "failed to check if keys should be rotated for CSI key %s", csiKeyringCephFSNodeUsername)
		}
		return key, nil
	}

	key, err := s.GenerateKey(csiKeyringCephFSNodeUsername, cephCSIKeyringCephFSNodeCaps())
	if err != nil {
		return "", err
	}

	return key, nil
}

func createCSIKeyringCephFSProvisioner(context *clusterd.Context, clusterInfo *client.ClusterInfo, clusterSpec *cephv1.ClusterSpec, s *keyring.SecretStore, keepPriorCount uint32, keyRotationEnabled bool) (string, error) {
	if keyRotationEnabled {
		shouldRotate, allKeyWithBaseName, err := handleCsiKeys(context, clusterInfo, clusterSpec, csiKeyringCephFSProvisionerUsername)
		if err != nil {
			return "", errors.Wrapf(err, "failed to check if key rotation is supported for CSI key %s", csiKeyringCephFSProvisionerUsername)
		}

		var key string
		if shouldRotate {
			key, err = s.GenerateKey(appendPriorKeyCountToSecretName(csiKeyringCephFSProvisionerUsername, keepPriorCount), cephCSIKeyringCephFSNodeCaps())
			if err != nil {
				return "", errors.Wrapf(err, "failed to check if keys should to be rotated for CSI key %s", csiKeyringCephFSProvisionerUsername)
			}

			err = deleteOldKeyGen(context, clusterInfo, allKeyWithBaseName, int(keepPriorCount))
			if err != nil {
				logger.Errorf("failed to delete keys during CSI key rotation.%v", err)
			}
			return key, nil
		}

		key, err = s.GenerateKey(appendPriorKeyCountToSecretName(csiKeyringCephFSProvisionerUsername, 1), cephCSIKeyringRBDNodeCaps())
		if err != nil {
			return "", errors.Wrapf(err, "failed to check if keys should be rotated for CSI key %s", csiKeyringCephFSProvisionerUsername)
		}
		return key, nil
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

// appendPriorKeyCountToSecretName generate CSI key with suffix `<baseName>.<priorKeyCount>`
// Example; client.csi-rbd-node.1
func appendPriorKeyCountToSecretName(csiSecretKeyName string, priorKeyCount uint32) string {
	if priorKeyCount > 0 {
		return fmt.Sprintf("%s.%d", csiSecretKeyName, priorKeyCount)
	}
	return csiSecretKeyName
}

// CreateCSISecrets creates all the Kubernetes CSI Secrets
func CreateCSISecrets(context *clusterd.Context, clusterInfo *client.ClusterInfo, clusterSpec *cephv1.ClusterSpec, clusterNamespaced types.NamespacedName) error {
	if clusterInfo.CSIDriverSpec.SkipUserCreation {
		if err := deleteOwnedCSISecretsByCephCluster(context, clusterInfo); err != nil {
			return err
		}
		logger.Info("CSI user creation is disabled; skipping user and secret creation")
		return nil
	}
	k := keyring.GetSecretStore(context, clusterInfo, clusterInfo.OwnerInfo)
	csiCephXConfig := clusterSpec.Security.CephX

	// In case of CSI or overlapping key rotation, we don't need desired cephVersion as key rotation on `WithCephVersionUpdate` is not supported.
	// We can pass any cephVersion as a place holder to the `ShouldRotateCephxKeys`.
	keyRotationEnabled, err := keyring.ShouldRotateCephxKeys(
		csiCephXConfig.CSI.CephxConfig, clusterInfo.CephVersion, clusterInfo.CephVersion, cephv1.CephxStatus{})
	if err != nil {
		return errors.Wrap(err, "failed to determine if cephx keys should be rotated")
	}

	if keyRotationEnabled {
		logger.Infof("cephx keys for CSI daemons in namespace %q will be rotated", clusterInfo.Namespace)
	}

	// Create CSI RBD Provisioner Ceph key
	csiRBDProvisionerSecretKey, err := createCSIKeyringRBDProvisioner(context, clusterInfo, clusterSpec, k, csiCephXConfig.CSI.KeepPriorKeyCount, keyRotationEnabled)
	if err != nil {
		return errors.Wrap(err, "failed to create csi rbd provisioner ceph keyring")
	}

	// Create CSI RBD Node Ceph key
	csiRBDNodeSecretKey, err := createCSIKeyringRBDNode(context, clusterInfo, clusterSpec, k, csiCephXConfig.CSI.KeepPriorKeyCount, keyRotationEnabled)
	if err != nil {
		return errors.Wrap(err, "failed to create csi rbd node ceph keyring")
	}

	// Create CSI Cephfs provisioner Ceph key
	csiCephFSProvisionerSecretKey, err := createCSIKeyringCephFSProvisioner(context, clusterInfo, clusterSpec, k, csiCephXConfig.CSI.KeepPriorKeyCount, keyRotationEnabled)
	if err != nil {
		return errors.Wrap(err, "failed to create csi cephfs provisioner ceph keyring")
	}

	// Create CSI Cephfs node Ceph key
	csiCephFSNodeSecretKey, err := createCSIKeyringCephFSNode(context, clusterInfo, clusterSpec, k, csiCephXConfig.CSI.KeepPriorKeyCount, keyRotationEnabled)
	if err != nil {
		return errors.Wrap(err, "failed to create csi cephfs node ceph keyring")
	}

	// Create or update Kubernetes CSI secret
	if err := createOrUpdateCSISecret(clusterInfo, csiRBDProvisionerSecretKey, csiRBDNodeSecretKey, csiCephFSProvisionerSecretKey, csiCephFSNodeSecretKey, k); err != nil {
		return errors.Wrap(err, "failed to create kubernetes csi secret")
	}

	err = updateCephStatusWithCephxStatus(context, clusterInfo, clusterSpec, clusterNamespaced, keyRotationEnabled)
	if err != nil {
		return err
	}

	return nil
}

// updateCephStatusWithCephxStatus updates the cephCluster status with CSI cephxStatus
func updateCephStatusWithCephxStatus(context *clusterd.Context, clusterInfo *client.ClusterInfo, clusterSpec *cephv1.ClusterSpec, name types.NamespacedName, keyRotationEnabled bool) error {
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

			cephxStatus := keyring.UpdatedCephxStatus(keyRotationEnabled, clusterSpec.Security.CephX.Daemon, clusterInfo.CephVersion, cephCluster.Status.Cephx.CSI)
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

// handleCsiKeys runs the `ceph auth ls` command to fetch all the keys and filter out the keys with same base name. Example
// for base name `client.csi-rbd-node` we'll make a list of all the ceph key which contains `client.csi-rbd-node`, we will have
// keys like `client.csi-rbd-node.1`, `client.csi-rbd-node.3`, `client.csi-rbd-node.4` so the list will have all these key name.
// And from the list of key name we'll read the suffix index eg; for key `client.csi-rbd-node.3` we'll take `3` and compare with
// KeyGeneration and basaed on the comparison we'll return bool to check if we should rotate the keys or not and returning list
// of all the matching keys.
func handleCsiKeys(context *clusterd.Context, clusterInfo *client.ClusterInfo, clusterSpec *cephv1.ClusterSpec, keyBaseName string) (bool, []string, error) {
	authList, err := client.AuthLS(context, clusterInfo)
	if err != nil {
		return false, []string{}, err
	}

	allKeyWithBaseName := []string{}
	for i, entry := range authList {
		if strings.Contains(entry.AuthDump[i].Entity, keyBaseName) {
			allKeyWithBaseName = append(allKeyWithBaseName, entry.AuthDump[i].Entity)
		}
	}

	if len(allKeyWithBaseName) == 0 {
		return true, allKeyWithBaseName, nil
	}

	currentMaxKeyCount, err := getKeySuffixNumber(allKeyWithBaseName[len(allKeyWithBaseName)-1])
	if err != nil {
		return false, allKeyWithBaseName, errors.Wrapf(err, "failed to get currentMaxKeyCount from key entity")
	}

	shouldRotate := false
	if clusterSpec.Security.CephX.CSI.KeyGeneration > uint32(currentMaxKeyCount) {
		shouldRotate = true
	}

	return shouldRotate, allKeyWithBaseName, nil
}

// deleteOldKeyGen deletes the CephxKeys based on the highest KeyGenCount and keepPriorCount. We'll sort the list of strings
// based on highest suffix in the CephXkeys. Example; `client.csi-rbd-node.3` we'll compare `3` with keepCount let's says `2`,
// then we'll delete 1 extra CephXkeys basically deleting the last key from the sorted key name list.
func deleteOldKeyGen(context *clusterd.Context, clusterInfo *client.ClusterInfo, allKeyWithBaseName []string, count int) error {
	type entry struct {
		secretName         string
		secretKeyGenSuffix int
	}

	var newKeyGenSecretCount []entry

	for _, name := range allKeyWithBaseName {
		suffix, err := getKeySuffixNumber(name)
		if err != nil {
			return err
		}

		newKeyGenSecretCount = append(newKeyGenSecretCount, entry{
			secretName:         name,
			secretKeyGenSuffix: int(suffix),
		})
	}

	sort.Slice(newKeyGenSecretCount, func(i, j int) bool {
		return newKeyGenSecretCount[i].secretKeyGenSuffix > newKeyGenSecretCount[j].secretKeyGenSuffix
	})

	newKeyGenSecretCount = newKeyGenSecretCount[:min(count, len(newKeyGenSecretCount))]

	keysFailedToDelete := []string{}
	var err error
	for _, p := range newKeyGenSecretCount {
		err = client.AuthDelete(context, clusterInfo, p.secretName)
		if err != nil {
			keysFailedToDelete = append(keysFailedToDelete, p.secretName)
			logger.Errorf("failed to delete CSI key %s more than keepPriorCount %d. %v", p.secretName, count, err)
		}
	}

	if len(keysFailedToDelete) > 0 {
		return errors.Wrapf(err, "list of CSI keys failed to be deleted %v", keysFailedToDelete)
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

// getKeySuffixNumber returns the suffix from the key. Example; `client.csi-rbd-node.3` will return `3` and for
// `client.csi-rbd-node` will return `0`.
func getKeySuffixNumber(key string) (uint32, error) {
	parts := strings.Split(key, ".")
	last := parts[len(parts)-1]

	if num, err := strconv.Atoi(last); err == nil && num >= 0 {
		return uint32(num), nil //nolint:gosec //nolint:gosec // disable G115 // Since I have already checked `num>0` we are good with the lint.
	} else if err != nil {
		return 0, errors.Wrapf(err, "failed to get keyGen suffix for CSI key %s", key)
	}

	return 0, nil
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
