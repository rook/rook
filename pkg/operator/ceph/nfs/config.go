/*
Copyright 2018 The Rook Authors. All rights reserved.

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

package nfs

import (
	"fmt"
	"io"
	"io/fs"
	"os"
	"strings"

	"github.com/pkg/errors"
	cephv1 "github.com/rook/rook/pkg/apis/ceph.rook.io/v1"
	"github.com/rook/rook/pkg/clusterd"
	cephclient "github.com/rook/rook/pkg/daemon/ceph/client"
	"github.com/rook/rook/pkg/operator/ceph/config/keyring"
	cephver "github.com/rook/rook/pkg/operator/ceph/version"
	"github.com/rook/rook/pkg/operator/k8sutil"
	"github.com/rook/rook/pkg/util/exec"
)

const kerberosRadosObjectName = "kerberos"

const (
	keyringTemplate = `
[%s]
        key = %s
        caps mon = "allow r"
        caps osd = "%s"
`
)

func getNFSUserID(nodeID string) string {
	return fmt.Sprintf("nfs-ganesha.%s", nodeID)
}

func getNFSClientID(n *cephv1.CephNFS, name string) string {
	return fmt.Sprintf("client.%s", getNFSUserID(getNFSNodeID(n, name)))
}

func getNFSNodeID(n *cephv1.CephNFS, name string) string {
	return fmt.Sprintf("%s.%s", n.Name, name)
}

func getGaneshaConfigObject(n *cephv1.CephNFS) string {
	return fmt.Sprintf("conf-nfs.%s", n.Name)
}

func getRadosURL(n *cephv1.CephNFS) string {
	url := fmt.Sprintf("rados://%s/", n.Spec.RADOS.Pool)

	if n.Spec.RADOS.Namespace != "" {
		url += n.Spec.RADOS.Namespace + "/"
	}

	url += getGaneshaConfigObject(n)
	return url
}

func (r *ReconcileCephNFS) generateKeyring(n *cephv1.CephNFS, name string) error {
	osdCaps := fmt.Sprintf("allow rw pool=%s", n.Spec.RADOS.Pool)
	if n.Spec.RADOS.Namespace != "" {
		osdCaps = fmt.Sprintf("%s namespace=%s", osdCaps, n.Spec.RADOS.Namespace)
	}

	caps := []string{"mon", "allow r", "osd", osdCaps}
	user := getNFSClientID(n, name)

	ownerInfo := k8sutil.NewOwnerInfo(n, r.scheme)
	s := keyring.GetSecretStore(r.context, r.clusterInfo, ownerInfo)

	key, err := s.GenerateKey(user, caps)
	if err != nil {
		return errors.Wrapf(err, "failed to create user %s", user)
	}

	keyring := fmt.Sprintf(keyringTemplate, user, key, osdCaps)
	return s.CreateOrUpdate(instanceName(n, name), keyring)
}

func getGaneshaConfig(n *cephv1.CephNFS, version cephver.CephVersion, name string) string {
	nodeID := getNFSNodeID(n, name)
	userID := getNFSUserID(nodeID)
	url := getRadosURL(n)
	return `
NFS_CORE_PARAM {
	Enable_NLM = false;
	Enable_RQUOTA = false;
	Protocols = 4;
}

MDCACHE {
	Dir_Chunk = 0;
}

EXPORT_DEFAULTS {
	Attr_Expiration_Time = 0;
}

NFSv4 {
	Delegations = false;
	RecoveryBackend = "rados_cluster";
	Minor_Versions = 1, 2;
}

RADOS_KV {
	ceph_conf = "` + cephclient.DefaultConfigFilePath() + `";
	userid = ` + userID + `;
	nodeid = ` + nodeID + `;
	pool = "` + n.Spec.RADOS.Pool + `";
	namespace = "` + n.Spec.RADOS.Namespace + `";
}

RADOS_URLS {
	ceph_conf = "` + cephclient.DefaultConfigFilePath() + `";
	userid = ` + userID + `;
	watch_url = "` + url + `";
}

RGW {
        name = "client.` + userID + `";
}

%url	` + url + `
`
}

func ganeshaKrbConfigBlock(kerberosSpec *cephv1.KerberosSpec) string {
	return fmt.Sprintf(`NFS_KRB5 {
	PrincipalName = "%s" ;
	KeytabPath = /etc/krb5.keytab ;
	Active_krb5 = YES ;
}
`, kerberosSpec.GetPrincipalName())
}

func ganeshaConfigIncludeKrbBlock(nfs *cephv1.CephNFS, radosObjectName string) string {
	// don't use sprintf b/c %u on front makes compiler confused
	return `%url "rados://` + nfs.Spec.RADOS.Pool + `/` + nfs.Spec.RADOS.Namespace + `/` + radosObjectName + `"` + "\n\n"
}

func (r *ReconcileCephNFS) setRadosConfig(nfs *cephv1.CephNFS) error {
	if nfs.Spec.Security.KerberosEnabled() {
		return setKerberosRadosConfig(r.context, r.clusterInfo, nfs)
	}
	return removeKerberosRadosConfig(r.context, r.clusterInfo, nfs)
}

func setKerberosRadosConfig(context *clusterd.Context, clusterInfo *cephclient.ClusterInfo, nfs *cephv1.CephNFS) error {
	radosPool := nfs.Spec.RADOS.Pool
	radosNs := nfs.Spec.RADOS.Namespace
	radosInfoStr := fmt.Sprintf("rados://%s/%s/", radosPool, radosNs)

	logger.Infof("ensuring kerberos configuration exists in rados namespace %s", radosInfoStr)

	// write ganesha kerberos configuration block into a temp file
	krbBlockFile, err := os.CreateTemp("", "krb-block-file")
	if err != nil {
		return errors.Wrapf(err, "failed to create temp file for ganesha kerberos configuration block for %s", radosInfoStr)
	}
	defer krbBlockFile.Close()
	_, err = krbBlockFile.WriteString(ganeshaKrbConfigBlock(nfs.Spec.Security.Kerberos))
	if err != nil {
		return errors.Wrapf(err, "failed write ganesha kerberos configuration block temp file for %s", radosInfoStr)
	}

	radosFlags := []string{
		"--pool", radosPool,
		"--namespace", radosNs,
	}

	// write ganesha kerberos configuration block to rados object from temp file
	cmd := cephclient.NewRadosCommand(context, clusterInfo,
		append(radosFlags, "put", kerberosRadosObjectName, krbBlockFile.Name()))
	_, err = cmd.RunWithTimeout(exec.CephCommandsTimeout)
	if err != nil {
		return errors.Wrapf(err, "failed to create or update the ganesha kerberos config object %s/%s",
			radosInfoStr, kerberosRadosObjectName)
	}

	// prepend the config block that includes the kerberos config object to the ganesha config object
	ganeshaConfigObjName := getGaneshaConfigObject(nfs)
	krbIncludeBlock := ganeshaConfigIncludeKrbBlock(nfs, kerberosRadosObjectName)
	err = atomicPrependToConfigObject(context, clusterInfo, radosPool, radosNs, ganeshaConfigObjName, krbIncludeBlock)
	if err != nil {
		return errors.Wrapf(err, "failed to update the ganesha config object to include the kerberos object config")
	}

	return nil
}

func removeKerberosRadosConfig(context *clusterd.Context, clusterInfo *cephclient.ClusterInfo, nfs *cephv1.CephNFS) error {
	radosPool := nfs.Spec.RADOS.Pool
	radosNs := nfs.Spec.RADOS.Namespace
	radosInfoStr := fmt.Sprintf("rados://%s/%s/", radosPool, radosNs)

	logger.Infof("ensuring kerberos configuration is removed from rados namespace %s", radosInfoStr)

	// remove config block that includes the kerberos config object from the ganesha config object
	ganeshaConfigObjName := getGaneshaConfigObject(nfs)
	krbIncludeBlock := ganeshaConfigIncludeKrbBlock(nfs, kerberosRadosObjectName)
	err := atomicRemoveFromConfigObject(context, clusterInfo, radosPool, radosNs, ganeshaConfigObjName, krbIncludeBlock)
	if err != nil {
		return errors.Wrap(err, "failed to update the ganesha config object to remove the kerberos object config")
	}

	// remove the kerberos config rados object
	err = cephclient.RadosRemoveObject(context, clusterInfo, radosPool, radosNs, kerberosRadosObjectName)
	if err != nil {
		return errors.Wrap(err, "failed to remove the ganesha kerberos config object")
	}

	return nil
}

// prepend a config block to the rados object in an atomic function
// uses rados locks to ensure the changes we make here aren't going to race with changes that might
// be made by ceph mgr, ceph itself, or a user manually
func atomicPrependToConfigObject(
	context *clusterd.Context, clusterInfo *cephclient.ClusterInfo,
	radosPool, radosNamespace, objectName, configBlock string,
) error {
	tmpFilePattern := fmt.Sprintf("%s_%s_%s_prepend", radosPool, radosNamespace, objectName)
	objInfoString := fmt.Sprintf("rados://%s/%s/%s", radosPool, radosNamespace, objectName)

	// read object into temp file
	tempFile, err := os.CreateTemp("", tmpFilePattern)
	if err != nil {
		return errors.Wrapf(err, "failed to create temp file for %s", objInfoString)
	}
	defer tempFile.Close()

	radosFlags := []string{
		"--pool", radosPool,
		"--namespace", radosNamespace,
	}

	// acquire lock to ensure no other processes (user, ceph, or rook) are racing each other
	// most common contender will be CSI when creating/removing NFS exports
	lockName := AppName

	// planning to perform 2 commands after the lock: get object, and optionally write object, so
	// use lock timeout of 2x the normal ceph command timeout
	logger.Infof("locking rados object %q", objInfoString)
	cookie, err := cephclient.RadosLockObject(context, clusterInfo,
		radosPool, radosNamespace, objectName, lockName, exec.CephCommandsTimeout*2)
	if err != nil {
		return err // already a good err message
	}
	logger.Infof("successfully locked rados object %q", objInfoString)
	defer func() {
		logger.Infof("unlocking rados object %q", objInfoString)
		err := cephclient.RadosUnlockObject(context, clusterInfo,
			radosPool, radosNamespace, objectName, lockName, cookie)
		if err != nil {
			logger.Infof("failed to unlock rados object %q, but since the lock has a timeout, we will continue. %v", objInfoString, err)
		}
		logger.Infof("successfully unlocked rados object %q", objInfoString)
	}()

	cmd := cephclient.NewRadosCommand(context, clusterInfo,
		append(radosFlags, "get", objectName, tempFile.Name()))
	if _, err := cmd.RunWithTimeout(exec.CephCommandsTimeout); err != nil {
		return errors.Wrapf(err, "failed to get object %s", objInfoString)
	}

	rawObj, err := io.ReadAll(tempFile)
	if err != nil {
		return errors.Wrapf(err, "failed to read object %s from temp file", objInfoString)
	}

	if strings.Contains(string(rawObj), configBlock) {
		logger.Debugf("rados object %s already has config block: %s", objInfoString, configBlock)
		return nil
	}
	logger.Debugf("rados object %s will have config block prepended: %s", objInfoString, configBlock)

	newConfig := fmt.Sprintf("%s%s", configBlock, string(rawObj))
	if err := os.WriteFile(tempFile.Name(), []byte(newConfig), fs.FileMode(0o644)); err != nil {
		return errors.Wrapf(err, "failed to write new config content for object %s to temp file", objInfoString)
	}

	cmd = cephclient.NewRadosCommand(context, clusterInfo,
		append(radosFlags, "put", objectName, tempFile.Name()))
	if _, err := cmd.RunWithTimeout(exec.CephCommandsTimeout); err != nil {
		return errors.Wrapf(err, "failed to set new config content on object %s", objInfoString)
	}

	return nil
}

// remove a config block from the rados object in an atomic function
// uses rados locks to ensure the changes we make here aren't going to race with changes that might
// be made by ceph mgr, ceph itself, or a user manually
func atomicRemoveFromConfigObject(context *clusterd.Context, clusterInfo *cephclient.ClusterInfo,
	radosPool, radosNamespace, objectName, configBlock string,
) error {
	tmpFilePattern := fmt.Sprintf("%s_%s_%s_remove", radosPool, radosNamespace, objectName)
	objInfoString := fmt.Sprintf("rados://%s/%s/%s", radosPool, radosNamespace, objectName)

	// read object into temp file
	tempFile, err := os.CreateTemp("", tmpFilePattern)
	if err != nil {
		return errors.Wrapf(err, "failed to create temp file for %s", objInfoString)
	}
	defer tempFile.Close()

	radosFlags := []string{
		"--pool", radosPool,
		"--namespace", radosNamespace,
	}

	// acquire lock to ensure no other processes (user, ceph, or rook) are racing each other
	// most common contender will be CSI when creating/removing NFS exports
	lockName := AppName

	// planning to perform 2 commands after the lock: get object, and optionally write object, so
	// use lock timeout of 2x the normal ceph command timeout
	logger.Infof("locking rados object %q", objInfoString)
	cookie, err := cephclient.RadosLockObject(context, clusterInfo,
		radosPool, radosNamespace, objectName, lockName, exec.CephCommandsTimeout*2)
	if err != nil {
		return err // already a good err message
	}
	logger.Infof("successfully locked rados object %q", objInfoString)
	defer func() {
		logger.Infof("unlocking rados object %q", objInfoString)
		err := cephclient.RadosUnlockObject(context, clusterInfo,
			radosPool, radosNamespace, objectName, lockName, cookie)
		if err != nil {
			logger.Infof("failed to unlock rados object %q, but since the lock has a timeout, we will continue. %v", objInfoString, err)
		}
		logger.Infof("successfully unlocked rados object %q", objInfoString)
	}()

	cmd := cephclient.NewRadosCommand(context, clusterInfo,
		append(radosFlags, "get", objectName, tempFile.Name()))
	if _, err := cmd.RunWithTimeout(exec.CephCommandsTimeout); err != nil {
		return errors.Wrapf(err, "failed to get object %s", objInfoString)
	}

	rawObj, err := io.ReadAll(tempFile)
	if err != nil {
		return errors.Wrapf(err, "failed to read object %s from temp file", objInfoString)
	}

	if !strings.Contains(string(rawObj), configBlock) {
		logger.Debugf("rados object %q already does not have config block: %s", objInfoString, configBlock)
		return nil
	}
	logger.Debugf("rados object %s will have config block removed: %s", objInfoString, configBlock)

	newConfig := strings.ReplaceAll(string(rawObj), configBlock, "")
	if err := os.WriteFile(tempFile.Name(), []byte(newConfig), fs.FileMode(0o644)); err != nil {
		return errors.Wrapf(err, "failed to write new config content for object %s to temp file", objInfoString)
	}

	cmd = cephclient.NewRadosCommand(context, clusterInfo,
		append(radosFlags, "put", objectName, tempFile.Name()))
	if _, err := cmd.RunWithTimeout(exec.CephCommandsTimeout); err != nil {
		return errors.Wrapf(err, "failed to set new config content on object %s", objInfoString)
	}

	return nil
}
