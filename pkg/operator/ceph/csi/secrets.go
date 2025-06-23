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

type csiCephUserStore struct {
	Name string
	Key  string
}

type csiSecretStore map[string]csiCephUserStore

// createCSIKeyring returns four values: the cephClientID with the keyCount as a suffix,
// the cephClient key value, a boolean indicating whether the key should be rotated, and an error.
// It calls the method `getCsiKeyRotationInfo` to find any Ceph clients matching the CSI keyBase
// and the maximum keyGen. If no matching Ceph clients are found, it returns keyGen as 0 and an
// empty list. If keys are found, a new key is generated with an incremented keyGen as a suffix,
// and the list of matching Ceph clients will be deleted later in the method.
// It fetches the current `interpretedCephxStatus` and keyGen from the CephCluster,
// and then calls the method `ShouldRotateCephxKeys` to determine if the old key should be rotated.
// Based on this, it updates the cephxStatus, generates a new client name, and creates a new key.
func createCSIKeyring(
	context *clusterd.Context, clusterInfo *client.ClusterInfo, cephCluster *cephv1.CephCluster,
	s *keyring.SecretStore, csiKeyrigName string, keyCaps []string,
) (string, string, int, bool, error) {
	logger.Debugf("starting CSI key generation for cluster in namespace %q", clusterInfo.Namespace)

	currentMaxKeyGen, allKeyWithBaseName, err := getCsiKeyRotationInfo(context, clusterInfo, csiKeyrigName)
	if err != nil {
		return "", "", 0, false, errors.Wrapf(err, "failed to determine key rotation info for CSI key %s", csiKeyrigName)
	}

	// corner cases could have the status KeyGeneration out of sync with what was determined above
	// apply the current max key gen to the current status to find the definitive current status
	interpretedCephxStatus := cephCluster.Status.Cephx.CSI.DeepCopy()
	interpretedCephxStatus.KeyGeneration = uint32(currentMaxKeyGen) //nolint:gosec // disable G115

	// determine shouldRotate based on 'definitive' status
	shouldRotate, err := keyring.ShouldRotateCephxKeys(
		cephCluster.Spec.Security.CephX.CSI.CephxConfig,
		clusterInfo.CephVersion, clusterInfo.CephVersion,
		interpretedCephxStatus.CephxStatus,
	)
	if err != nil {
		return "", "", 0, shouldRotate, errors.Wrap(err, "failed to call `shouldRotateCephxKeys` during CSI key rotation")
	}

	// generate the future cephx status to determine what the new key gen should be
	desiredCephxStatus := keyring.UpdatedCephxStatus(shouldRotate,
		cephCluster.Spec.Security.CephX.CSI.CephxConfig,
		clusterInfo.CephVersion,
		interpretedCephxStatus.CephxStatus)

	// determine the client ID that should exist as the latest entry
	latestClientId := generateCsiUserIdWithGenerationSuffix(csiKeyrigName, desiredCephxStatus.KeyGeneration)

	// ensure the client key exists
	key, err := s.GenerateKey(latestClientId, keyCaps)
	if err != nil {
		return "", "", 0, shouldRotate, errors.Wrapf(err, "failed to check if keys should to be rotated for CSI key %s", csiKeyrigName)
	}

	keysDeleted, err := deleteOldKeyGen(context, clusterInfo, allKeyWithBaseName, cephCluster.Spec.Security.CephX.CSI.KeepPriorKeyCountMax)
	if err != nil {
		logger.Errorf("failed to delete keys during CSI key rotation. %v. Continuing with key rotation", err)
	}

	// add the new key to the list of all keys
	allKeyWithBaseName = append(allKeyWithBaseName, latestClientId)
	allKeyWithBaseName = deduplicate(allKeyWithBaseName)

	currentKeyCount := len(allKeyWithBaseName) - len(keysDeleted)

	return latestClientId, key, currentKeyCount, shouldRotate, nil
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

func createOrUpdateCSISecret(clusterInfo *client.ClusterInfo, csiSecretContent csiSecretStore, k *keyring.SecretStore) error {
	csiRBDProvisionerSecrets := map[string][]byte{
		// userID is expected for the rbd provisioner driver
		"userID":  []byte(csiSecretContent[CsiRBDProvisionerSecret].Name),
		"userKey": []byte(csiSecretContent[CsiRBDProvisionerSecret].Key),
	}

	csiRBDNodeSecrets := map[string][]byte{
		// userID is expected for the rbd node driver
		"userID":  []byte(csiSecretContent[CsiRBDNodeSecret].Name),
		"userKey": []byte(csiSecretContent[CsiRBDNodeSecret].Key),
	}

	csiCephFSProvisionerSecrets := map[string][]byte{
		// adminID is expected for the cephfs provisioner driver
		"adminID":  []byte(csiSecretContent[CsiCephFSProvisionerSecret].Name),
		"adminKey": []byte(csiSecretContent[CsiCephFSProvisionerSecret].Key),
	}

	csiCephFSNodeSecrets := map[string][]byte{
		// adminID is expected for the cephfs node driver
		"adminID":  []byte(csiSecretContent[CsiCephFSNodeSecret].Name),
		"adminKey": []byte(csiSecretContent[CsiCephFSNodeSecret].Key),
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
func CreateCSISecrets(context *clusterd.Context, clusterInfo *client.ClusterInfo, clusterNamespaced types.NamespacedName) error {
	if clusterInfo.CSIDriverSpec.SkipUserCreation {
		if err := deleteOwnedCSISecretsByCephCluster(context, clusterInfo); err != nil {
			return err
		}
		logger.Info("CSI user creation is disabled; skipping user and secret creation")
		return nil
	}
	k := keyring.GetSecretStore(context, clusterInfo, clusterInfo.OwnerInfo)

	logger.Info("getting CephCluster %s during CSI key creation in namespace %s", clusterNamespaced.Name, clusterNamespaced.Namespace)
	cephCluster := &cephv1.CephCluster{}

	if err := context.Client.Get(clusterInfo.Context, clusterNamespaced, cephCluster); err != nil {
		if kerrors.IsNotFound(err) {
			logger.Debug("CephCluster resource not found. Ignoring since object must be deleted.")
			return nil
		}
		return errors.Wrapf(err, "failed to retrieve cephCluster %q to generate CSICephx keys", clusterNamespaced.Namespace)
	}

	// Create CSI RBD Provisioner Ceph key
	rbdProvName, rbdProvKey, rbdProvKeyCount, didRotateRbdProvisioner, err := createCSIKeyring(context, clusterInfo, cephCluster, k, csiKeyringRBDProvisionerUsername, cephCSIKeyringRBDProvisionerCaps())
	if err != nil {
		return errors.Wrap(err, "failed to create csi rbd provisioner ceph keyring")
	}

	// Create CSI RBD Node Ceph key
	rbdNodeName, rbdNodeKey, rbdNodeKeyCount, didRotateRbdNode, err := createCSIKeyring(context, clusterInfo, cephCluster, k, csiKeyringRBDNodeUsername, cephCSIKeyringRBDNodeCaps())
	if err != nil {
		return errors.Wrap(err, "failed to create csi rbd node ceph keyring")
	}

	// Create CSI Cephfs provisioner Ceph key
	cFSProvName, cFSProvKey, cFSProvKeyCount, didRotateCephFsProvisioner, err := createCSIKeyring(context, clusterInfo, cephCluster, k, csiKeyringCephFSProvisionerUsername, cephCSIKeyringCephFSProvisionerCaps())
	if err != nil {
		return errors.Wrap(err, "failed to create csi cephfs provisioner ceph keyring")
	}

	// Create CSI Cephfs node Ceph key
	cFSNodeName, cFSNodeKey, cFSNodeKeyCount, didRotateCephFsNode, err := createCSIKeyring(context, clusterInfo, cephCluster, k, csiKeyringCephFSNodeUsername, cephCSIKeyringCephFSNodeCaps())
	if err != nil {
		return errors.Wrap(err, "failed to create csi cephfs node ceph keyring")
	}

	// The latestClientIdMap contains latestClientID without prefix `client.` in the format CSI requires in the secret created below.
	// Ex for "client.csi-rbd-provisioner" values will `csi-rbd-provisioner.<keyGen>` that is  latestClientID.
	csiSecretContent := csiSecretStore{
		CsiRBDProvisionerSecret:    {Name: strings.TrimPrefix(rbdProvName, "client."), Key: rbdProvKey},
		CsiRBDNodeSecret:           {Name: strings.TrimPrefix(rbdNodeName, "client."), Key: rbdNodeKey},
		CsiCephFSProvisionerSecret: {Name: strings.TrimPrefix(cFSProvName, "client."), Key: cFSProvKey},
		CsiCephFSNodeSecret:        {Name: strings.TrimPrefix(cFSNodeName, "client."), Key: cFSNodeKey},
	}

	// Create or update Kubernetes CSI secret
	if err := createOrUpdateCSISecret(clusterInfo, csiSecretContent, k); err != nil {
		return errors.Wrap(err, "failed to create kubernetes csi secret")
	}

	currentKeyCount := max(rbdProvKeyCount, rbdNodeKeyCount, cFSProvKeyCount, cFSNodeKeyCount)
	didRotateAny := didRotateRbdProvisioner || didRotateRbdNode || didRotateCephFsProvisioner || didRotateCephFsNode
	err = updateCephStatusWithCephxStatus(context, clusterInfo, cephCluster, clusterNamespaced, didRotateAny, currentKeyCount)
	if err != nil {
		return err
	}

	return nil
}

// updateCephStatusWithCephxStatus updates the cephCluster status with CSI cephxStatus
func updateCephStatusWithCephxStatus(context *clusterd.Context, clusterInfo *client.ClusterInfo, cephCluster *cephv1.CephCluster,
	namespacedName types.NamespacedName, didRotate bool, currentKeyCount int,
) error {
	logger.Infof("updating cephCluster %s cephStatus with CSI cephxStatus in namespace %s", namespacedName.Name, namespacedName.Namespace)
	cephxStatus := keyring.UpdatedCephxStatus(didRotate, cephCluster.Spec.Security.CephX.Daemon, clusterInfo.CephVersion, cephCluster.Status.Cephx.CSI.CephxStatus)

	err := retry.RetryOnConflict(retry.DefaultRetry, func() error {
		cephCluster := &cephv1.CephCluster{}
		if err := context.Client.Get(clusterInfo.Context, namespacedName, cephCluster); err != nil {
			if kerrors.IsNotFound(err) {
				logger.Debug("CephCluster resource not found. Ignoring since object must be deleted.")
				return nil
			}
			return errors.Wrapf(err, "failed to retrieve cephCluster %q to update CSICephxStatus", namespacedName.Namespace)
		}
		cephCluster.Status.Cephx.CSI.CephxStatus = cephxStatus
		cephCluster.Status.Cephx.CSI.PriorKeyCount = uint8(currentKeyCount) //nolint:gosec // disable G115

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
func getCsiKeyRotationInfo(context *clusterd.Context, clusterInfo *client.ClusterInfo, keyBaseName string) (int, []string, error) {
	authList, err := client.AuthList(context, clusterInfo)
	if err != nil {
		return 0, []string{}, err
	}

	keyWithBaseName, err := getMatchingClient(authList, keyBaseName)
	if err != nil {
		return 0, keyWithBaseName, err
	}

	// In fresh deployment `keyWithBaseName` length will be empty, so let's not return error.
	if len(keyWithBaseName) == 0 {
		logger.Debugf("no key matching with %s found in auth list must be fresh cluster", keyBaseName)
		return 0, keyWithBaseName, nil
	}

	sortedClientList := sortCSIClientName(keyWithBaseName)

	if len(sortedClientList) == 0 {
		return 0, keyWithBaseName, fmt.Errorf("empty list found while fetching CSI key from")
	}

	_, currentMaxKeyGen, err := parseCsiClient(sortedClientList[len(sortedClientList)-1])
	if err != nil {
		return 0, keyWithBaseName, errors.Wrap(err, "failed to get currentMaxKeyCount from key entity")
	}

	return currentMaxKeyGen, keyWithBaseName, nil
}

// getMatchingClient return list of all the ceph key which contains `client.csi-rbd-node`, we will have keys like
// `client.csi-rbd-node.1`, `client.csi-rbd-node.2`, `client.csi-rbd-node.3` so the list will have all these key name.
// Return list of keys are expected to contain `client.`.
func getMatchingClient(authList client.AuthListOutput, clientBaseName string) ([]string, error) {
	keyWithBaseName := []string{}

	for _, entry := range authList.AuthDump {
		basename, _, err := parseCsiClient(entry.Entity)
		if err != nil {
			// We want to continue processing other keys even if one fails to parse.
			logger.Debugf("no CSI client name found matching with %s. %v", clientBaseName, err)
		}

		if basename == clientBaseName {
			keyWithBaseName = append(keyWithBaseName, entry.Entity)
		}
	}

	return keyWithBaseName, nil
}

// parseCsiClient parses a CSI client name of the format "client.<baseName>[.<generation>]"
// Example valid formats: "client.csi-rbd-node", "client.csi-rbd-node.3"
// Returns: basename ("client.csi-rbd-node"), generation (int, default 0 if absent), error if format is invalid.
func parseCsiClient(c string) (string, int, error) {
	cSplit := strings.Split(c, ".")
	if len(cSplit) < 2 || len(cSplit) > 3 {
		return "", 0, fmt.Errorf("unexpected CSI client name format: %q", c)
	}
	if cSplit[0] != "client" {
		return "", 0, fmt.Errorf(`CSI client name %q does not begin with "client"`, c)
	} else if strings.HasSuffix(cSplit[1], " ") {
		return "", 0, errors.Errorf("no CSI client name found matching with %s contains space", c)
	}

	gen := 0
	if len(cSplit) == 3 {
		var err error
		gen, err = strconv.Atoi(cSplit[2])
		if err != nil {
			return "", 0, errors.Wrapf(err, "failed to parse generation for CSI client name %q", c)
		}
		if gen < 0 {
			return "", 0, fmt.Errorf("parsed generation for CSI client name %q is negative", c)
		}
	}

	// return basename, generation, err
	return cSplit[0] + "." + cSplit[1], gen, nil
}

// deleteOldKeyGen deletes CephX keys based on the highest key generation count (KeyGen).
// It keeps the latest `count` keys by sorting the client keys by their numeric suffix.
// Example: For `client.csi-rbd-node.3`, the generation is 3.
// If `count` is 2 and there are 3 keys, one old key will be deleted.
func deleteOldKeyGen(context *clusterd.Context, clusterInfo *client.ClusterInfo, allKeyWithBaseName []string, count uint8) ([]string, error) {
	sortedClientList := sortCSIClientName(allKeyWithBaseName)

	if len(sortedClientList) < 2 {
		logger.Debug("not enough matching CSI keys to delete during rotation. Could be fresh deployment.")
		return []string{}, nil
	}

	keyDeleteCount := max(0, len(sortedClientList)-int(count)) // can't delete negative count
	if len(sortedClientList) == keyDeleteCount && keyDeleteCount > 0 {
		// Above condition will delete all the keys related to specific CSI driver keys, this will break CSI and Ceph connection.
		// To avoid above we'll delete `count-1`
		logger.Warningf("current `keepPriorCountMax` value %d will delete all the related CSI driver key count %d. Deleting `keepPriorCountMax +1` keys", count, keyDeleteCount)
		keyDeleteCount -= 1
	}

	listOfKeysDeleted := []string{}
	for i := range keyDeleteCount {
		err := client.AuthDelete(context, clusterInfo, sortedClientList[i])
		if err != nil {
			// in case of partial success, return successfully deleted list on error
			return listOfKeysDeleted, errors.Wrapf(err, "failed to delete CSI key %s more than keepPriorCount %d", sortedClientList[i], count)
		}
		listOfKeysDeleted = append(listOfKeysDeleted, sortedClientList[i]) // Delete the lowest-numbered keys first
	}
	return listOfKeysDeleted, nil
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

func sortCSIClientName(clientList []string) []string {
	type entry struct {
		clientId       string
		clientIdSuffix int
	}

	var newKeyGenNameCounts []entry
	for _, name := range clientList {
		_, suffix, err := parseCsiClient(name)
		if err != nil {
			// We want to continue processing other keys even if one fails to parse.
			logger.Debugf("client %s doesn't match with CSI client name. %v", name, err)
		}

		newKeyGenNameCounts = append(newKeyGenNameCounts, entry{
			clientId:       name,
			clientIdSuffix: suffix,
		})
	}

	if len(newKeyGenNameCounts) == 0 {
		logger.Debugf("failed to generate list of sorted CSI client name no matching key found")
		return []string{}
	}

	sort.Slice(newKeyGenNameCounts, func(i, j int) bool {
		return newKeyGenNameCounts[i].clientIdSuffix < newKeyGenNameCounts[j].clientIdSuffix
	})

	sortedClientName := []string{}
	for _, e := range newKeyGenNameCounts {
		sortedClientName = append(sortedClientName, e.clientId)
	}

	return sortedClientName
}

func deduplicate(list []string) []string {
	allKeys := make(map[string]bool)
	uniqueList := []string{}
	for _, v := range list {
		if _, value := allKeys[v]; !value {
			allKeys[v] = true
			uniqueList = append(uniqueList, v)
		}
	}
	return uniqueList
}
