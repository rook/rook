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
	kerrors "k8s.io/apimachinery/pkg/api/errors"
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

func createCSIKeyring(context *clusterd.Context, clusterInfo *client.ClusterInfo, clusterSpec *cephv1.ClusterSpec, s *keyring.SecretStore, csiConfig cephv1.CephXConfigWithPriorCount, csiKeyrigName string, keyCaps []string) (string, error) {
	var key string

	// We only want to trigger below block `KeyGeneration` is greater 0 or else we'll get error when we are reconcile on fresh cluster
	// which don't have any CSI keys yet
	if csiConfig.KeyGeneration > 0 {
		shouldRotate, allKeyWithBaseName, err := getCsiKeyRotationInfo(context, clusterInfo, clusterSpec, csiKeyrigName)
		if err != nil {
			return "", errors.Wrapf(err, "failed to determine key rotation info for  CSI key %s", csiKeyrigName)
		}
		keysDeleted, err := deleteOldKeyGen(context, clusterInfo, allKeyWithBaseName, csiConfig.KeepPriorKeyCountMax)
		if err != nil {
			logger.Errorf("failed to delete keys during CSI key rotation. %v", err)
		}

		for _, key := range keysDeleted {
			logger.Debugf("CSI %s key deleted during key rotation when keyGeneration set to %d", key, csiConfig.KeyGeneration)
		}
		if shouldRotate {
			key, err = s.GenerateKey(generateCsiUserIdWithGenerationSuffix(csiKeyrigName, csiConfig.KeyGeneration), keyCaps)
			if err != nil {
				return "", errors.Wrapf(err, "failed to check if keys should to be rotated for CSI key %s", csiKeyrigName)
			}
			return key, nil
		}
	}

	key, err := s.GenerateKey(csiKeyrigName, keyCaps)
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

// generateCsiUserIdWithGenerationSuffix generate ceph client ID with suffix `<baseName>.<keyGeneration>`
// Example; client.csi-rbd-node.1
func generateCsiUserIdWithGenerationSuffix(clientBaseName string, keyGeneration uint32) string {
	if keyGeneration > 0 {
		return fmt.Sprintf("%s.%d", clientBaseName, keyGeneration)
	}
	return clientBaseName
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

	csiCephXConfig := clusterSpec.Security.CephX.CSI

	// Create CSI RBD Provisioner Ceph key
	csiRBDProvisionerSecretKey, err := createCSIKeyring(context, clusterInfo, clusterSpec, k, csiCephXConfig, csiKeyringRBDProvisionerUsername, cephCSIKeyringRBDProvisionerCaps())
	if err != nil {
		return errors.Wrap(err, "failed to create csi rbd provisioner ceph keyring")
	}

	// Create CSI RBD Node Ceph key
	csiRBDNodeSecretKey, err := createCSIKeyring(context, clusterInfo, clusterSpec, k, csiCephXConfig, csiKeyringRBDNodeUsername, cephCSIKeyringRBDNodeCaps())
	if err != nil {
		return errors.Wrap(err, "failed to create csi rbd node ceph keyring")
	}

	// Create CSI Cephfs provisioner Ceph key
	csiCephFSProvisionerSecretKey, err := createCSIKeyring(context, clusterInfo, clusterSpec, k, csiCephXConfig, csiKeyringCephFSProvisionerUsername, cephCSIKeyringCephFSProvisionerCaps())
	if err != nil {
		return errors.Wrap(err, "failed to create csi cephfs provisioner ceph keyring")
	}

	// Create CSI Cephfs node Ceph key
	csiCephFSNodeSecretKey, err := createCSIKeyring(context, clusterInfo, clusterSpec, k, csiCephXConfig, csiKeyringCephFSNodeUsername, cephCSIKeyringCephFSNodeCaps())
	if err != nil {
		return errors.Wrap(err, "failed to create csi cephfs node ceph keyring")
	}

	// Create or update Kubernetes CSI secret
	if err := createOrUpdateCSISecret(clusterInfo, csiRBDProvisionerSecretKey, csiRBDNodeSecretKey, csiCephFSProvisionerSecretKey, csiCephFSNodeSecretKey, k); err != nil {
		return errors.Wrap(err, "failed to create kubernetes csi secret")
	}

	err = updateCephStatusWithCephxStatus(context, clusterInfo, clusterNamespaced)
	if err != nil {
		return err
	}

	return nil
}

// updateCephStatusWithCephxStatus updates the cephCluster status with CSI cephxStatus
func updateCephStatusWithCephxStatus(context *clusterd.Context, clusterInfo *client.ClusterInfo, namespacedName types.NamespacedName) error {
	cephCluster := &cephv1.CephCluster{}
	err := retry.OnError(
		retry.DefaultRetry,
		func(err error) bool {
			return err != nil
		},
		func() error {
			if err := context.Client.Get(clusterInfo.Context, namespacedName, cephCluster); err != nil {
				if kerrors.IsNotFound(err) {
					logger.Debug("CephCluster resource not found. Ignoring since object must be deleted.")
					return nil
				}
				return errors.Wrapf(err, "failed to retrieve object store %q to update CSICephxStatus", namespacedName.String())
			}

			keyRotationEnabled, err := keyring.ShouldRotateCephxKeys(cephCluster.Spec.Security.CephX.CSI.CephxConfig, clusterInfo.CephVersion, clusterInfo.CephVersion, cephv1.CephxStatus{})
			if err != nil {
				return errors.Wrap(err, "failed to determine if cephx keys should be rotated")
			}

			cephxStatus := keyring.UpdatedCephxStatus(keyRotationEnabled, cephCluster.Spec.Security.CephX.Daemon, clusterInfo.CephVersion, cephCluster.Status.Cephx.CSI.CephxStatus)
			cephCluster.Status.Cephx.CSI.CephxStatus = cephxStatus
			cephCluster.Status.Cephx.CSI.PriorKeyCount = cephCluster.Spec.Security.CephX.CSI.KeepPriorKeyCountMax

			if err := reporting.UpdateStatus(context.Client, cephCluster); err != nil {
				return errors.Wrapf(err, "failed to update cephCluster %q to update csi cephx status to %q in namespace %q", namespacedName.Name, cephxStatus, namespacedName.Namespace)
			}
			return nil
		})
	if err != nil {
		return errors.Wrapf(err, "failed to get and update Ceph cluster %q status in namespace %q when updating csi cephx status", namespacedName.Name, namespacedName.Namespace)
	}

	logger.Debugf("successfully updated Ceph cluster %q status with CSI Cephx status in namespace %q", namespacedName.Name, namespacedName.Namespace)
	return nil
}

// getCsiKeyRotationInfo runs the `ceph auth ls` command to fetch all the keys and filter out the keys with same base name. Example
// for base name `client.csi-rbd-node`.
// From the list of key name return from `getMatchingClient` we'll read the suffix index eg; for key `client.csi-rbd-node.3` we'll take `3` and compare with
// KeyGeneration and basaed on the comparison we'll return bool to check if we should rotate the keys or not and returning list
// of all the matching keys.
func getCsiKeyRotationInfo(context *clusterd.Context, clusterInfo *client.ClusterInfo, clusterSpec *cephv1.ClusterSpec, keyBaseName string) (bool, []string, error) {
	authList, err := client.AuthLsList(context, clusterInfo)
	if err != nil {
		return false, []string{}, err
	}

	keyWithBaseName, err := getMatchingClient(authList, keyBaseName)
	if err != nil {
		return false, keyWithBaseName, err
	}

	if len(keyWithBaseName) == 0 {
		return false, keyWithBaseName, errors.Wrapf(err, "no key matching with %s found in auth list", keyBaseName)
	}

	sortedClientList, err := sortCSIClientName(keyWithBaseName)
	if err != nil {
		return false, keyWithBaseName, errors.Wrapf(err, "failed to get sorted CSI key list from %s while reading `maxCount`", sortedClientList)
	}

	_, maxCount, err := parseCsiClient(sortedClientList[len(sortedClientList)-1])
	if err != nil {
		return false, keyWithBaseName, errors.Wrap(err, "failed to get currentMaxKeyCount from key entity")
	}

	currentMaxKeyCount := uint32(maxCount) //nolint:gosec // disable G115 // already checked
	interpretedCephxStatus := cephv1.CephxStatus{
		KeyCephVersion: "", // key ceph ver irrelevant for CSI keys
		KeyGeneration:  currentMaxKeyCount,
	}

	// In case of CSI or overlapping key rotation, we don't need desired cephVersion as key rotation on `WithCephVersionUpdate` is not supported.
	// We can pass any cephVersion as a place holder to the `ShouldRotateCephxKeys`.
	shouldRotate, err := keyring.ShouldRotateCephxKeys(clusterSpec.Security.CephX.CSI.CephxConfig, clusterInfo.CephVersion, clusterInfo.CephVersion, interpretedCephxStatus)
	if err != nil {
		return false, keyWithBaseName, errors.Wrap(err, "failed to determine if cephx keys should be rotated")
	}

	return shouldRotate, keyWithBaseName, nil
}

// getMatchingClient return list of all the ceph key which contains `client.csi-rbd-node`, we will have keys like
// `client.csi-rbd-node.1`, `client.csi-rbd-node.2`, `client.csi-rbd-node.3` so the list will have all these key name.
func getMatchingClient(authList client.AuthList, clientBaseName string) ([]string, error) {
	keyWithBaseName := []string{}

	for _, entry := range authList.AuthDump {
		basename, _, err := parseCsiClient(entry.Entity)
		if err != nil {
			logger.Debugf("no CSI client name found matching with %s.%v", clientBaseName, err)
		}

		if basename == clientBaseName {
			keyWithBaseName = append(keyWithBaseName, entry.Entity)
		}
	}

	if len(keyWithBaseName) == 0 {
		return []string{}, errors.Errorf("failed to get list of CSI client name matching with %s", clientBaseName)
	}
	return keyWithBaseName, nil
}

// return basename, generation, error
func parseCsiClient(c string) (string, int, error) {
	cSplit := strings.Split(c, ".")
	if len(cSplit) < 2 || len(cSplit) > 3 {
		return "", 0, fmt.Errorf("unexpected CSI client name format: %q", c)
	}
	if cSplit[0] != "client" {
		return "", 0, fmt.Errorf(`CSI client name %q does not begin with "client"`, c)
	} else if strings.HasPrefix(cSplit[0], " ") || strings.HasSuffix(cSplit[1], " ") {
		return "", 0, errors.Errorf("no CSI client name found matching with %s contains space", c)
	}

	var err error
	gen := 0
	if len(cSplit) == 3 {
		gen, err = strconv.Atoi(cSplit[2])
		if err != nil {
			return "", 0, fmt.Errorf("failed to parse generation for CSI client name %q", c)
		}
		if gen < 0 {
			return "", 0, fmt.Errorf("parsed generation for CSI client name %q is negative", c)
		}
	}

	// return basename, generation, err
	return cSplit[0] + "." + cSplit[1], gen, nil
}

// deleteOldKeyGen deletes the CephxKeys based on the highest KeyGenCount and keepPriorCount. We'll sort the list of strings
// based on highest suffix in the CephXkeys. Example; `client.csi-rbd-node.3` we'll compare `3` with keepCount let's says `2`,
// then we'll delete 1 extra CephXkeys basically deleting the last key from the sorted key name list.
func deleteOldKeyGen(context *clusterd.Context, clusterInfo *client.ClusterInfo, allKeyWithBaseName []string, count uint8) ([]string, error) {
	sortedClientList, err := sortCSIClientName(allKeyWithBaseName)
	if err != nil {
		return []string{}, errors.Wrapf(err, "failed to get sorted CSI key list from %v for deletion", allKeyWithBaseName)
	}

	logger.Info(sortedClientList, "+", err)

	listOfKeyDeleted := []string{}
	keyDeleteCount := max(0, len(sortedClientList)-int(count)) // can't delete negative count
	for i := range keyDeleteCount {
		listOfKeyDeleted = append(listOfKeyDeleted, sortedClientList[i]) // Delete the lowest-numbered keys first
		err = client.AuthDelete(context, clusterInfo, sortedClientList[i])
		if err != nil {
			return []string{}, errors.Wrapf(err, "failed to delete CSI key %s more than keepPriorCount %d", sortedClientList[i], count)
		}
	}
	return listOfKeyDeleted, nil
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

func sortCSIClientName(clientList []string) ([]string, error) {
	type entry struct {
		clientId       string
		clientIdSuffix int
	}

	var newKeyGenNameCount []entry
	for _, name := range clientList {
		_, suffix, err := parseCsiClient(name)
		if err != nil {
			logger.Debugf("client %s doesn't match with CSI client name. %v", name, err)
		}

		newKeyGenNameCount = append(newKeyGenNameCount, entry{
			clientId:       name,
			clientIdSuffix: int(suffix),
		})
	}

	if len(newKeyGenNameCount) == 0 {
		return []string{}, errors.Errorf("failed to generate list of sorted CSI client name no matching key found")
	}

	sort.Slice(newKeyGenNameCount, func(i, j int) bool {
		return newKeyGenNameCount[i].clientIdSuffix < newKeyGenNameCount[j].clientIdSuffix
	})

	sortedClientName := []string{}
	for _, name := range newKeyGenNameCount {
		sortedClientName = append(sortedClientName, name.clientId)
	}

	return sortedClientName, nil
}
