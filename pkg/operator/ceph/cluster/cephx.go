/*
Copyright 2025 The Rook Authors. All rights reserved.

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

package cluster

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/pkg/errors"
	cephv1 "github.com/rook/rook/pkg/apis/ceph.rook.io/v1"
	"github.com/rook/rook/pkg/clusterd"
	"github.com/rook/rook/pkg/daemon/ceph/client"
	cephclient "github.com/rook/rook/pkg/daemon/ceph/client"
	"github.com/rook/rook/pkg/operator/ceph/cluster/mon"
	"github.com/rook/rook/pkg/operator/ceph/config"
	"github.com/rook/rook/pkg/operator/ceph/config/keyring"
	"github.com/rook/rook/pkg/operator/ceph/controller"
	"github.com/rook/rook/pkg/operator/ceph/reporting"
	"github.com/rook/rook/pkg/operator/k8sutil"
	kerrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/client-go/util/retry"
)

const (
	//#nosec G101 -- This is only a secret name prefix
	adminRotatorSecretPrefix = "rook-ceph-admin-rotator"
	//#nosec G101 -- This is only the admin client name
	adminRotatorUsername = "client.admin-rotator"
)

// note: must match adminKeyringTemplate
var (
	adminKeyAccessCaps = []string{"mds", "allow *", "mon", "allow *", "osd", "allow *", "mgr", "allow *"}

	// ReloadManager sends SIGHUP to the main process. Allow this to be stubbed for unit tests.
	reloadManagerFunc = controller.ReloadManager
)

// turn a client name and its caps into keyring file contents
// example: client.my-user, []string{"mon", "allow *"} becomes:
//
//	[client.my-user]
//		caps mon = "allow *"
func genKeyring(clientName, authKey string, clientCaps []string) (string, error) {
	// DO NOT LOG authKey
	if authKey == "" {
		return "", fmt.Errorf("cannot generate %q keyring with empty cephx key", clientName)
	}
	o := fmt.Sprintf("[%s]\n", clientName)
	o += fmt.Sprintf("	key = %s\n", authKey)
	if len(clientCaps)%2 != 0 {
		return "", fmt.Errorf("cannot generate %q keyring for caps list %v with uneven number of items", clientName, clientCaps)
	}
	for i := 0; i < len(clientCaps); i += 2 {
		o += fmt.Sprintf("	caps %s = %q\n", clientCaps[i], clientCaps[i+1])
	}
	return o, nil
}

// main routine for rotating the client.admin key
// should be called after mon daemons are upgraded to current ceph image in the cephcluster spec
func rotateAdminCephxKey(
	clusterdCtx *clusterd.Context,
	clusterInfo *client.ClusterInfo,
	ownerInfo *k8sutil.OwnerInfo,
	cephCluster *cephv1.CephCluster,
) error {
	if clusterInfo.CephCred.Username != cephclient.AdminUsername {
		// this shouldn't happen during normal runtime - this could indicate external mode cluster
		logger.Infof("cannot rotate admin cephx key with non-rotatable username %q for cluster in namespace %q", clusterInfo.CephCred.Username, clusterInfo.Namespace)
		return nil
	}

	desiredCephVersion := clusterInfo.CephVersion // TODO: update this when/if WithCephVersionUpdate is implemented
	shouldRotate, err := keyring.ShouldRotateCephxKeys(
		cephCluster.Spec.Security.CephX.Daemon, clusterInfo.CephVersion, desiredCephVersion, *cephCluster.Status.Cephx.Admin)
	if err != nil {
		return errors.Wrap(err, "failed to determine if admin cephx key should be rotated")
	}
	if !shouldRotate {
		logger.Debugf("not rotating admin cephx key for cluster in namespace %q", clusterInfo.Namespace)
		// no rotation, but still update cephx status - critical for greenfield Uninitialized clusters
		err := updateCephClusterAdminCephxStatus(clusterdCtx, clusterInfo, false)
		if err != nil {
			return errors.Wrap(err, "failed to update admin cephx key status after not rotating")
		}
		return nil
	}

	logger.Infof("beginning admin cephx key rotation for cluster in namespace %q", clusterInfo.Namespace)

	// As an optimization, replace the clusterInfo.Context with a new context that won't be canceled
	// during normally-expected reconciles. This disallows the rotation process from being
	// interrupted halfway through in the event of a CephCluster spec change, which can happen for
	// any number of reasons during normal runtime. While this rotation is designed to be able to be
	// recovered if it's interrupted, it is still safest to allow the rotation process to finish in
	// full. This means fewer risks of issues during normal runtime interruptions.
	oldClusterInfoContext := clusterInfo.Context
	clusterInfo.Context = context.Background()
	defer func() { clusterInfo.Context = oldClusterInfoContext }()
	// TODO: not working -- need copy of clusterInfo instead?????

	logger.Debugf("TESTING TESTING TESTING: calling ReloadManager() to cancel mgr context -- hope to see admin key rotation finish end to end")
	reloadManagerFunc()

	s := keyring.GetSecretStore(clusterdCtx, clusterInfo, ownerInfo)

	// generate client.admin-rotator admin user
	// if client.admin rotation fails or is blocked, the client.admin-rotator user can be used to
	// recover from bugs/blockages in rotation of primary client.admin key
	rotatorKey, err := s.GenerateKey(adminRotatorUsername, adminKeyAccessCaps)
	if err != nil {
		return errors.Wrapf(err, "failed to generate cephx key for admin rotator %q", adminRotatorUsername)
	}

	// store client.admin-rotator in secret
	rotatorKeyring, err := genKeyring(adminRotatorUsername, rotatorKey, adminKeyAccessCaps)
	if err != nil {
		return errors.Wrapf(err, "failed to generate admin rotator cephx keyring file")
	}
	_, err = s.CreateOrUpdate(adminRotatorSecretPrefix, rotatorKeyring)
	if err != nil {
		return errors.Wrapf(err, "failed to store admin rotator cephx keyring to secret")
	}

	// keep keyring files used for admin key rotation in a subdir intended to be temporary
	// stored in rook-ceph-operator emptyDir
	// do not clean up in defer() so that any issues can be debugged with shell into operator pod
	tmpDir := adminRotationTmpDir(clusterdCtx, clusterInfo)

	return rotateAdminCephxKeyUsingRotator(clusterdCtx, clusterInfo, ownerInfo, tmpDir, rotatorKeyring)
}

// routine for recovering from client.admin key rotation that was interrupted in a prior reconcile
// because the currently-stored client.admin key could be incorrect, this should be called very
// early in the cephcluster reconcile, before any ceph commands are run: failures running commands
// could block the reconcile from recovering admin key rotation
func recoverPriorAdminCephxKeyRotation(
	clusterdCtx *clusterd.Context,
	clusterInfo *client.ClusterInfo,
	ownerInfo *k8sutil.OwnerInfo,
	clusterNamespace string,
) error {
	if clusterInfo == nil {
		// new cluster hasn't been deployed yet. there should be no way for recovery to be needed
		logger.Debugf("no admin cephx key recovery possible for cluster with empty clusterInfo in namespace %q", clusterNamespace)
		return nil
	}

	s := keyring.GetSecretStore(clusterdCtx, clusterInfo, ownerInfo)

	rotatorKeyring, err := s.GetKeyringFromSecret(adminRotatorSecretPrefix)
	if err != nil {
		if kerrors.IsNotFound(err) {
			logger.Debugf("no admin cephx key recovery needed for cluster in namespace %q", clusterInfo.Namespace)
			return nil
		}
		return errors.Wrap(err, "failed to get admin rotator cephx key secret to check if admin cephx key rotation recovery is needed")
	}

	// As an optimization, replace the clusterInfo.Context with a new context that won't be canceled
	// during normally-expected reconciles. This disallows the rotation process from being
	// interrupted halfway through in the event of a CephCluster spec change, which can happen for
	// any number of reasons during normal runtime. While this rotation is designed to be able to be
	// recovered if it's interrupted, it is still safest to allow the rotation process to finish in
	// full. This means fewer risks of issues during normal runtime interruptions.
	oldClusterInfoContext := clusterInfo.Context
	clusterInfo.Context = context.Background()
	defer func() { clusterInfo.Context = oldClusterInfoContext }()

	logger.Infof("recovering from interrupted admin cephx key rotation for cluster in namespace %q", clusterInfo.Namespace)

	// if the operator pod has restarted and recovery is needed, the ceph.conf file will not be
	// present in the rook temp dir. Write/update the cluster config to handle this case
	err = mon.WriteConnectionConfig(clusterdCtx, clusterInfo)
	if err != nil {
		return errors.Wrap(err, "failed to write/update cluster config in ceph.conf before recovering admin cephx key rotation")
	}

	// keep keyring files used for admin key rotation in a subdir intended to be temporary
	// stored in rook-ceph-operator emptyDir
	// do not clean up in defer() so that any issues can be debugged with shell into operator pod
	tmpDir := adminRotationTmpDir(clusterdCtx, clusterInfo)

	authList, err := client.AuthList(clusterdCtx, clusterInfo)
	if err != nil {
		logger.Debugf("primary admin user failed to list ceph auth - this is an expected condition when recovering from admin cephx key rotation")
	} else if !entityExistsInAuthList(adminRotatorUsername, authList) { // err == nil
		// TODO: get ceph version from mons using primary admin clusterInfo

		// corner case: if the primary client.admin works AND client.admin-rotator is not present in
		// the `auth ls` output, it means rotation succeeded and was interrupted during cleanup.
		// client.auth-rotator is no longer present/valid, so it can't be used to take any actions
		return finalizeAdminKeyRotation(clusterdCtx, clusterInfo, ownerInfo, tmpDir)
	}

	// TODO: get ceph version from mons using admin-rotator

	// a prior rotation failed, but we don't know exactly where in the process it failed. we have to
	// re-run the main rotation routine from the beginning to ensure we don't skip rotation
	return rotateAdminCephxKeyUsingRotator(clusterdCtx, clusterInfo, ownerInfo, tmpDir, rotatorKeyring)
}

func entityExistsInAuthList(entity string, authList client.AuthListOutput) bool {
	for _, a := range authList.AuthDump {
		if a.Entity == entity {
			return true
		}
	}
	return false
}

// helper routine that rotate client.admin cephx key including cleanup and cephx status update
// uses the provided client.admin-rotator keyring to rotate client.admin
func rotateAdminCephxKeyUsingRotator(
	clusterdCtx *clusterd.Context,
	clusterInfo *client.ClusterInfo,
	ownerInfo *k8sutil.OwnerInfo,
	tmpDir, rotatorKeyring string,
) error {
	// write admin rotator keyring file to tempdir so it can be used for rotation
	rotatorKeyringTmpfile := filepath.Join(tmpDir, adminRotatorUsername+".keyring")
	logger.Infof("temporarily storing admin rotator keyring at %q for cluster in namespace %q", rotatorKeyringTmpfile, clusterInfo.Namespace)
	err := client.WriteKeyring(rotatorKeyringTmpfile, rotatorKeyring)
	if err != nil {
		return errors.Wrapf(err, "failed to write admin rotator keyring to %q", rotatorKeyringTmpfile)
	}

	// make copy of ClusterInfo struct that uses admin-rotator user/key for ceph commands
	rotatorInfo := minimalCopyClusterInfo(clusterInfo, adminRotatorUsername, rotatorKeyringTmpfile)

	// as `client.admin-rotator`: run `ceph auth ls` to ensure it has permissions
	_, err = cephclient.AuthList(clusterdCtx, rotatorInfo)
	if err != nil {
		return errors.Wrapf(err, "failed to list ceph auth using admin rotator client")
	}

	// as `client.admin-rotator`: run `ceph auth rotate client.admin`
	logger.Infof("admin cephx key will be rotated for cluster in namespace %q. rook will restart afterwards. some reconciles and health checks may fail in between - this is normal", clusterInfo.Namespace)
	// newAdminKey, err := cephclient.AuthRotate(clusterdCtx, rotatorInfo, cephclient.AdminUsername)
	newAdminKey, err := cephclient.AuthGetKey(clusterdCtx, rotatorInfo, "client.admin") // TESTING TESTING TESTING
	if err != nil {
		return errors.Wrapf(err, "failed to rotate admin key using admin rotator client")
	}
	newAdminKeyring := cephclient.CephKeyring(cephclient.CephCred{Username: cephclient.AdminUsername, Secret: newAdminKey})

	// write new admin keyring to tempdir so it can be verified before it's stored permanently
	newAdminKeyringTmpfile := filepath.Join(tmpDir, cephclient.AdminUsername+".keyring")
	logger.Infof("temporarily storing rotated admin cephx keyring at %q for cluster in namespace %q", newAdminKeyringTmpfile, clusterInfo.Namespace)
	err = client.WriteKeyring(newAdminKeyringTmpfile, newAdminKeyring)
	if err != nil {
		return errors.Wrapf(err, "failed to write admin rotator keyring to %q", newAdminKeyringTmpfile)
	}

	// `client.admin` run `ceph auth ls` to ensure it has permissions
	// make copy of ClusterInfo struct that uses rotated admin's temporary keyring for ceph commands
	newAdminInfo := minimalCopyClusterInfo(clusterInfo, cephclient.AdminUsername, newAdminKeyringTmpfile)
	_, err = cephclient.AuthList(clusterdCtx, newAdminInfo)
	if err != nil {
		return errors.Wrapf(err, "admin rotator failed to list ceph auth")
	}

	// now that the new key is verified working, update clusterInfo with new key
	clusterInfo.CephCred.Secret = newAdminKey

	// update the on-disk ceph config file with new `client.admin` key (now in clusterInfo)
	err = mon.WriteConnectionConfig(clusterdCtx, clusterInfo)
	if err != nil {
		return errors.Wrapf(err, "failed to write latest cluster config to disk")
	}

	// update the `rook-ceph-mon` secret with new `client.admin` key (now in clusterInfo)
	err = controller.UpdateClusterAccessSecret(clusterdCtx.Clientset, clusterInfo)
	if err != nil {
		return errors.Wrapf(err, "failed to update cluster access secret with latest %q admin cephx key", cephclient.AdminUsername)
	}

	// as `client.admin`: run `ceph auth rm client.admin-rotator`
	err = cephclient.AuthDelete(clusterdCtx, clusterInfo, adminRotatorUsername)
	if err != nil {
		return errors.Wrapf(err, "failed to delete cephx auth for admin rotator %q", adminRotatorUsername)
	}

	return finalizeAdminKeyRotation(clusterdCtx, clusterInfo, ownerInfo, tmpDir)
}

// helper routine that performs final cleanup steps for admin key rotation. should be used
// immediately after `ceph auth rm client.admin-rotator`. updates cephx status when finished
func finalizeAdminKeyRotation(
	clusterdCtx *clusterd.Context,
	clusterInfo *client.ClusterInfo,
	ownerInfo *k8sutil.OwnerInfo,
	tmpDir string,
) error {
	// clean up tmpDir now that it's no longer useful for recovery
	if err := os.RemoveAll(tmpDir); err != nil {
		// most likely, operator pod got rescheduled to a different node where dir isn't present
		logger.Infof("non-critical failure removing admin cephx key rotation temp dir %q. %v", tmpDir, err)
	}

	// delete the `rook-ceph-admin-rotator-keyring` secret (store.Delete)
	s := keyring.GetSecretStore(clusterdCtx, clusterInfo, ownerInfo)
	err := s.Delete(adminRotatorSecretPrefix)
	if err != nil {
		return errors.Wrapf(err, "failed to delete admin rotator cephx keyring secret")
	}

	// during recovery, clusterInfo doesn't know the ceph version, so set it if needed
	if clusterInfo.CephVersion.Major == 0 { // Major==0 should be good enough for detecting unknown ver
		ver, err := cephclient.LeastUptodateDaemonVersion(clusterdCtx, clusterInfo, config.MonType)
		if err != nil {
			// this should be rare, and failure here doesn't need to block finalization of admin key
			// rotation. status will be missing ceph version, but cluster can continue to reconcile
			logger.Errorf("non-critical failure to determine missing ceph version of cluster in namespace %q for admin cephx key status", clusterInfo.Namespace)
		} else {
			clusterInfo.CephVersion = ver
		}
	}

	// update `CephCluster.status.cephx.admin` with updated CephX status
	// rotation is a given if this code line was reached
	if err := updateCephClusterAdminCephxStatus(clusterdCtx, clusterInfo, true); err != nil {
		return err
	}

	// restart Rook to force health checkers to restart with latest clusterInfo
	// do this even during rotation recovery since the health checkers could still be running
	logger.Infof("restarting rook operator after successful admin cephx key rotation for cluster in namespace %q", clusterInfo.Namespace)
	reloadManagerFunc()

	return nil // TODO: return error since we know the context is just going to be canceled anyway?
}

// helper routine that updates cephcluster 'admin' cephx status
func updateCephClusterAdminCephxStatus(clusterdCtx *clusterd.Context, clusterInfo *cephclient.ClusterInfo, didRotate bool) error {
	err := retry.RetryOnConflict(retry.DefaultRetry, func() error {
		cluster := &cephv1.CephCluster{}
		if err := clusterdCtx.Client.Get(clusterInfo.Context, clusterInfo.NamespacedName(), cluster); err != nil {
			return errors.Wrap(err, "failed to get CephCluster to update the admin key cephx status")
		}
		cephx := keyring.UpdatedCephxStatus(didRotate, cluster.Spec.Security.CephX.Daemon, clusterInfo.CephVersion, *cluster.Status.Cephx.Admin)
		cluster.Status.Cephx.Admin = &cephx
		logger.Debugf("updating admin key cephx status to %+v", cephx)
		if err := reporting.UpdateStatus(clusterdCtx.Client, cluster); err != nil {
			return errors.Wrap(err, "failed to update admin key cephx status")
		}

		return nil
	})
	return err
}

func adminRotationTmpDir(clusterdCtx *clusterd.Context, clusterInfo *client.ClusterInfo) string {
	return filepath.Join(clusterdCtx.ConfigDir, clusterInfo.Namespace, "admin-rotate")
}

// make a minimal copy of the clusterInfo that tells called cephclient functions to use the given
// username and keyring path instead the defaults
func minimalCopyClusterInfo(clusterInfo *cephclient.ClusterInfo, username, keyringFilePath string) *cephclient.ClusterInfo {
	c := &cephclient.ClusterInfo{
		Context:   clusterInfo.Context,
		Namespace: clusterInfo.Namespace,
		CephCred: cephclient.CephCred{
			Username: username,
			// Secret not used by cephclient executor
		},
		CephVersion:         clusterInfo.CephVersion,
		KeyringFileOverride: keyringFilePath,
	}
	c.SetName(clusterInfo.NamespacedName().Name)
	return c
}
