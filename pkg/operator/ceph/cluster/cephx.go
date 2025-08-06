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
	"fmt"
	"os"
	"path/filepath"

	"github.com/pkg/errors"
	"github.com/rook/rook/pkg/clusterd"
	"github.com/rook/rook/pkg/daemon/ceph/client"
	"github.com/rook/rook/pkg/operator/ceph/config/keyring"
	"github.com/rook/rook/pkg/operator/k8sutil"
)

const (
	//#nosec G101 -- This is only a secret name prefix
	adminRotatorSecretPrefix = "rook-ceph-admin-rotator"
	//#nosec G101 -- This is only the admin client name
	adminRotatorUsername = "client.admin-rotator"
)

// note: must match adminKeyringTemplate
var adminKeyAccessCaps = []string{"mds", "allow *", "mon", "allow *", "osd", "allow *", "mgr", "allow *"}

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
	o += fmt.Sprintf("	key = %s", authKey)
	if len(clientCaps)%2 != 0 {
		return "", fmt.Errorf("cannot generate %q keyring for caps list %v with uneven number of items", clientName, clientCaps)
	}
	for i := 0; i < len(clientCaps); i += 2 {
		o += fmt.Sprintf("	caps %s = %q\n", clientCaps[i], clientCaps[i+1])
	}
	return o, nil
}

// TODO: unexport
func RotateAdminCephxKey(context *clusterd.Context, clusterInfo *client.ClusterInfo, ownerInfo *k8sutil.OwnerInfo) error {
	logger.Infof("beginning admin key rotation for cluster in namespace %q", clusterInfo.Namespace)

	s := keyring.GetSecretStore(context, clusterInfo, ownerInfo)

	// generate client.admin-rotator admin user
	rotatorKey, err := s.GenerateKey(adminRotatorUsername, adminKeyAccessCaps)
	if err != nil {
		return errors.Wrapf(err, "failed to generate key for admin rotator %q", adminRotatorUsername)
	}

	// store client.admin-rotator in secrets
	rotatorKeyring, err := genKeyring(adminRotatorUsername, rotatorKey, adminKeyAccessCaps)
	if err != nil {
		return errors.Wrapf(err, "failed to generate admin rotator keyring file")
	}
	_, err = s.CreateOrUpdate(adminRotatorSecretPrefix, rotatorKeyring)
	if err != nil {
		return errors.Wrapf(err, "failed to store admin rotator keyring to secret")
	}

	// read back the secret's keyring and verify it. this verification is extremely paranoid, but
	// because a mistake during admin key rotation could brick a Rook cluster, we do it anyway
	kInSecret, err := s.GetKeyringFromSecret(adminRotatorSecretPrefix)
	if err != nil {
		return errors.Wrapf(err, "failed to get admin rotator secret in order to verify its contents")
	}
	if kInSecret != rotatorKeyring {
		// trace log because the content is sensitive
		logger.Tracef("admin rotator secret contains keyring %q, not the expected %q", kInSecret, rotatorKeyring)
		return fmt.Errorf("admin rotator secret does not have the expected contents")
	}

	return RotateAdminCephxKeyUsingRotator(context, clusterInfo, ownerInfo, rotatorKeyring)

	// return nil
}

// TODO: unexport
func RotateAdminCephxKeyUsingRotator(context *clusterd.Context, clusterInfo *client.ClusterInfo, ownerInfo *k8sutil.OwnerInfo, rotatorKeyring string) error {
	// keep keyring files used for admin key rotation in a subdir intended to be temporary
	tmpDir := filepath.Join(context.ConfigDir, "admin-rotate")
	// DO NOT CLEAN UP THIS DIR USING defer() - keeping the most recent keyring files on disk may be
	// the only thing that allows manual recovery in the worst possible cases

	rotatorKeyringTmpfile := filepath.Join(tmpDir, "client.admin-rotator.keyring")
	logger.Infof("storing admin rotator keyring at %q for cluster in namespace %q", rotatorKeyringTmpfile, clusterInfo.Namespace)
	err := client.WriteKeyring(rotatorKeyringTmpfile, rotatorKeyring)
	if err != nil {
		return errors.Wrapf(err, "failed to write admin rotator keyring to %q", rotatorKeyringTmpfile)
	}

	// again being paranoid and verifying file contents
	kInFile, err := os.ReadFile(rotatorKeyringTmpfile)
	if err != nil {
		return errors.Wrapf(err, "failed to read admin rotator keyring file at %q in order to verify its contents", rotatorKeyringTmpfile)
	}
	if string(kInFile) != rotatorKeyring {
		// trace log because the content is sensitive
		logger.Tracef("admin rotator keyring file contains keyring %q, not the expected %q", string(kInFile), rotatorKeyring)
		return fmt.Errorf("admin rotator file does not have the expected contents")
	}

	// as `client.admin-rotator`: run `ceph auth ls` to ensure it has permissions
	// TODO: store admin-rotator keyring into temp file in context.ConfigDir
	//   then run ceph command with that in the --keyring argument

	// as `client.admin-rotator`: run `ceph auth rotate client.admin`
	// `client.admin` run `ceph auth ls` to ensure it has permissions
	// update the on-disk ceph config file with new `client.admin` keyring
	//   - use mon.WriteConnectionConfig() -- may need to move it or this into new package

	// update the `rook-ceph-mon` secret with new `client.admin` keyring
	//   - use controller.UpdateClusterAccessSecret() -- may need to move it or this into new package

	// read back the `client.admin` keyring from its secret, and verify it
	//   - CreateOrLoadClusterInfo() ???

	// as `client.admin`: run `ceph auth rm client.admin-rotator`

	// TODO: clean up tmpDir only after we are sure we don't need it for any recovery

	// delete the `rook-ceph-admin-keyring` secret (store.Delete)
	// update `CephCluster.status.cephx.admin` with updated CephX status
}
