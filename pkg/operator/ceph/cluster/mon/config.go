/*
Copyright 2017 The Rook Authors. All rights reserved.

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

package mon

import (
	"context"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"os"
	"path"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/pkg/errors"
	"github.com/rook/rook/pkg/clusterd"
	cephclient "github.com/rook/rook/pkg/daemon/ceph/client"
	"github.com/rook/rook/pkg/operator/ceph/csi"
	"github.com/rook/rook/pkg/operator/k8sutil"
	"github.com/rook/rook/pkg/util/exec"
	"github.com/rook/rook/pkg/util/sys"
	v1 "k8s.io/api/core/v1"
	kerrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
)

const (
	// All mons share the same keyring
	keyringStoreName = "rook-ceph-mons"

	// The final string field is for the admin keyring
	keyringTemplate = `
[mon.]
	key = %s
	caps mon = "allow *"

%s`

	externalConnectionRetry = 60 * time.Second
)

var (
	ClusterInfoNoClusterNoSecret = errors.New("not expected to create new cluster info and did not find existing secret")
)

func (c *Cluster) genMonSharedKeyring() string {
	return fmt.Sprintf(
		keyringTemplate,
		c.ClusterInfo.MonitorSecret,
		cephclient.CephKeyring(c.ClusterInfo.CephCred),
	)
}

// return mon data dir path relative to the dataDirHostPath given a mon's name
func dataDirRelativeHostPath(monName string) string {
	monHostDir := monName // support legacy case where the mon name is "mon#" and not a lettered ID
	if !strings.Contains(monName, "mon") {
		// if the mon name doesn't have "mon" in it, mon dir is "mon-<ID>"
		monHostDir = "mon-" + monName
	}
	// Keep existing behavior where Rook stores the mon's data in the "data" subdir
	return path.Join(monHostDir, "data")
}

// LoadClusterInfo constructs or loads a clusterinfo and returns it along with the maxMonID
func LoadClusterInfo(ctx *clusterd.Context, context context.Context, namespace string) (*cephclient.ClusterInfo, int, *Mapping, error) {
	return CreateOrLoadClusterInfo(ctx, context, namespace, nil)
}

// CreateOrLoadClusterInfo constructs or loads a clusterinfo and returns it along with the maxMonID
func CreateOrLoadClusterInfo(clusterdContext *clusterd.Context, context context.Context, namespace string, ownerInfo *k8sutil.OwnerInfo) (*cephclient.ClusterInfo, int, *Mapping, error) {
	var clusterInfo *cephclient.ClusterInfo
	maxMonID := -1
	monMapping := &Mapping{
		Schedule: map[string]*MonScheduleInfo{},
	}

	secrets, err := clusterdContext.Clientset.CoreV1().Secrets(namespace).Get(context, AppName, metav1.GetOptions{})
	if err != nil {
		if !kerrors.IsNotFound(err) {
			return nil, maxMonID, monMapping, errors.Wrap(err, "failed to get mon secrets")
		}
		if ownerInfo == nil {
			return nil, maxMonID, monMapping, ClusterInfoNoClusterNoSecret
		}

		clusterInfo, err = createNamedClusterInfo(clusterdContext, namespace)
		if err != nil {
			return nil, maxMonID, monMapping, errors.Wrap(err, "failed to create mon secrets")
		}
		clusterInfo.Context = context

		err = createClusterAccessSecret(clusterdContext.Clientset, namespace, clusterInfo, ownerInfo)
		if err != nil {
			return nil, maxMonID, monMapping, err
		}
	} else {
		clusterInfo = &cephclient.ClusterInfo{
			Namespace:     namespace,
			FSID:          string(secrets.Data[fsidSecretNameKey]),
			MonitorSecret: string(secrets.Data[monSecretNameKey]),
			Context:       context,
		}
		if cephUsername, ok := secrets.Data[cephUsernameKey]; ok {
			clusterInfo.CephCred.Username = string(cephUsername)
			clusterInfo.CephCred.Secret = string(secrets.Data[cephUserSecretKey])
		} else if adminSecretKey, ok := secrets.Data[adminSecretNameKey]; ok {
			clusterInfo.CephCred.Username = cephclient.AdminUsername
			clusterInfo.CephCred.Secret = string(adminSecretKey)

			secrets.Data[cephUsernameKey] = []byte(cephclient.AdminUsername)
			secrets.Data[cephUserSecretKey] = adminSecretKey
			if _, err = clusterdContext.Clientset.CoreV1().Secrets(namespace).Update(context, secrets, metav1.UpdateOptions{}); err != nil {
				return nil, maxMonID, monMapping, errors.Wrap(err, "failed to update mon secrets")
			}
		} else {
			return nil, maxMonID, monMapping, errors.New("failed to find either the cluster admin key or the username")
		}
		logger.Debugf("found existing monitor secrets for cluster %s", clusterInfo.Namespace)
	}

	// get the existing monitor config
	clusterInfo.Monitors, maxMonID, monMapping, err = loadMonConfig(clusterdContext.Clientset, namespace)
	if err != nil {
		return nil, maxMonID, monMapping, errors.Wrap(err, "failed to get mon config")
	}

	// If an admin key was provided we don't need to load the other resources
	// Some people might want to give the admin key
	// The necessary users/keys/secrets will be created by Rook
	// This is also done to allow backward compatibility
	if clusterInfo.CephCred.Username == cephclient.AdminUsername && clusterInfo.CephCred.Secret != adminSecretNameKey {
		return clusterInfo, maxMonID, monMapping, nil
	}

	// If the admin secret is "admin-secret", look for the deprecated secret that has the external creds
	if clusterInfo.CephCred.Secret == adminSecretNameKey {
		secret, err := clusterdContext.Clientset.CoreV1().Secrets(namespace).Get(context, OperatorCreds, metav1.GetOptions{})
		if err != nil {
			return clusterInfo, maxMonID, monMapping, err
		}
		// Populate external credential
		clusterInfo.CephCred.Username = string(secret.Data["userID"])
		clusterInfo.CephCred.Secret = string(secret.Data["userKey"])
	}

	if err := ValidateCephCSIConnectionSecrets(clusterdContext, namespace); err != nil {
		return clusterInfo, maxMonID, monMapping, err
	}

	return clusterInfo, maxMonID, monMapping, nil
}

// ValidateCephCSIConnectionSecrets returns the secret value of the client health checker key
func ValidateCephCSIConnectionSecrets(clusterdContext *clusterd.Context, namespace string) error {
	ctx := context.TODO()
	_, err := clusterdContext.Clientset.CoreV1().Secrets(namespace).Get(ctx, csi.CsiRBDNodeSecret, metav1.GetOptions{})
	if err != nil {
		if !kerrors.IsNotFound(err) {
			return errors.Wrapf(err, "failed to get %q secret", csi.CsiRBDNodeSecret)
		}
	}

	_, err = clusterdContext.Clientset.CoreV1().Secrets(namespace).Get(ctx, csi.CsiRBDProvisionerSecret, metav1.GetOptions{})
	if err != nil {
		if !kerrors.IsNotFound(err) {
			return errors.Wrapf(err, "failed to get %q secret", csi.CsiRBDProvisionerSecret)
		}
	}

	_, err = clusterdContext.Clientset.CoreV1().Secrets(namespace).Get(ctx, csi.CsiCephFSNodeSecret, metav1.GetOptions{})
	if err != nil {
		if !kerrors.IsNotFound(err) {
			return errors.Wrapf(err, "failed to get %q secret", csi.CsiCephFSNodeSecret)
		}
	}

	_, err = clusterdContext.Clientset.CoreV1().Secrets(namespace).Get(ctx, csi.CsiCephFSProvisionerSecret, metav1.GetOptions{})
	if err != nil {
		if !kerrors.IsNotFound(err) {
			return errors.Wrapf(err, "failed to get %q secret", csi.CsiCephFSProvisionerSecret)
		}
	}

	return nil
}

// WriteConnectionConfig save monitor connection config to disk
func WriteConnectionConfig(context *clusterd.Context, clusterInfo *cephclient.ClusterInfo) error {
	// write the latest config to the config dir
	if _, err := cephclient.GenerateConnectionConfig(context, clusterInfo); err != nil {
		return errors.Wrap(err, "failed to write connection config")
	}

	return nil
}

// loadMonConfig returns the monitor endpoints and maxMonID
func loadMonConfig(clientset kubernetes.Interface, namespace string) (map[string]*cephclient.MonInfo, int, *Mapping, error) {
	ctx := context.TODO()
	monEndpointMap := map[string]*cephclient.MonInfo{}
	maxMonID := -1
	monMapping := &Mapping{
		Schedule: map[string]*MonScheduleInfo{},
	}

	cm, err := clientset.CoreV1().ConfigMaps(namespace).Get(ctx, EndpointConfigMapName, metav1.GetOptions{})
	if err != nil {
		if !kerrors.IsNotFound(err) {
			return nil, maxMonID, monMapping, err
		}
		// If the config map was not found, initialize the empty set of monitors
		return monEndpointMap, maxMonID, monMapping, nil
	}

	// Parse the monitor List
	if info, ok := cm.Data[EndpointDataKey]; ok {
		monEndpointMap = ParseMonEndpoints(info)
	}

	// Parse the max monitor id
	storedMaxMonID := -1
	if id, ok := cm.Data[MaxMonIDKey]; ok {
		storedMaxMonID, err = strconv.Atoi(id)
		if err != nil {
			logger.Errorf("invalid max mon id %q. %v", id, err)
		} else {
			maxMonID = storedMaxMonID
		}
	}

	// Make sure the max id is consistent with the current monitors
	for _, m := range monEndpointMap {
		id, _ := fullNameToIndex(m.Name)
		if maxMonID < id {
			maxMonID = id
		}
	}
	if maxMonID != storedMaxMonID {
		logger.Infof("updating obsolete maxMonID %d to actual value %d", storedMaxMonID, maxMonID)
	}

	err = json.Unmarshal([]byte(cm.Data[MappingKey]), &monMapping)
	if err != nil {
		logger.Errorf("invalid JSON in mon mapping. %v", err)
	}

	logger.Debugf("loaded: maxMonID=%d, mons=%+v, assignment=%+v", maxMonID, monEndpointMap, monMapping)
	return monEndpointMap, maxMonID, monMapping, nil
}

func createClusterAccessSecret(clientset kubernetes.Interface, namespace string, clusterInfo *cephclient.ClusterInfo, ownerInfo *k8sutil.OwnerInfo) error {
	logger.Infof("creating mon secrets for a new cluster")
	var err error

	// store the secrets for internal usage of the rook pods
	secrets := map[string][]byte{
		fsidSecretNameKey: []byte(clusterInfo.FSID),
		monSecretNameKey:  []byte(clusterInfo.MonitorSecret),
		cephUsernameKey:   []byte(clusterInfo.CephCred.Username),
		cephUserSecretKey: []byte(clusterInfo.CephCred.Secret),
	}
	secret := &v1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      AppName,
			Namespace: namespace,
		},
		Data: secrets,
		Type: k8sutil.RookType,
	}
	err = ownerInfo.SetControllerReference(secret)
	if err != nil {
		return errors.Wrapf(err, "failed to set owner reference to mon secret %q", secret.Name)
	}
	if _, err = clientset.CoreV1().Secrets(namespace).Create(clusterInfo.Context, secret, metav1.CreateOptions{}); err != nil {
		return errors.Wrap(err, "failed to save mon secrets")
	}

	return nil
}

// create new cluster info (FSID, shared keys)
func createNamedClusterInfo(context *clusterd.Context, namespace string) (*cephclient.ClusterInfo, error) {
	fsid, err := uuid.NewRandom()
	if err != nil {
		return nil, err
	}

	dir := path.Join(context.ConfigDir, namespace)
	if err = os.MkdirAll(dir, 0744); err != nil {
		return nil, errors.Wrapf(err, "failed to create dir %s", dir)
	}

	// generate the mon secret
	monSecret, err := genSecret(context.Executor, dir, "mon.", []string{"--cap", "mon", "'allow *'"})
	if err != nil {
		return nil, err
	}

	// generate the admin secret if one was not provided at the command line
	args := []string{
		"--cap", "mon", "'allow *'",
		"--cap", "osd", "'allow *'",
		"--cap", "mgr", "'allow *'",
		"--cap", "mds", "'allow'"}
	adminSecret, err := genSecret(context.Executor, dir, cephclient.AdminUsername, args)
	if err != nil {
		return nil, err
	}

	return &cephclient.ClusterInfo{
		FSID:          fsid.String(),
		MonitorSecret: monSecret,
		Namespace:     namespace,
		CephCred: cephclient.CephCred{
			Username: cephclient.AdminUsername,
			Secret:   adminSecret,
		},
	}, nil
}

func genSecret(executor exec.Executor, configDir, name string, args []string) (string, error) {
	path := path.Join(configDir, fmt.Sprintf("%s.keyring", name))
	path = strings.Replace(path, "..", ".", 1)
	base := []string{
		"--create-keyring",
		path,
		"--gen-key",
		"-n", name,
	}
	args = append(base, args...)
	_, err := executor.ExecuteCommandWithOutput("ceph-authtool", args...)
	if err != nil {
		return "", errors.Wrap(err, "failed to gen secret")
	}

	contents, err := ioutil.ReadFile(filepath.Clean(path))
	if err != nil {
		return "", errors.Wrap(err, "failed to read secret file")
	}
	return ExtractKey(string(contents))
}

// ExtractKey retrieves mon secret key from the keyring file
func ExtractKey(contents string) (string, error) {
	secret := ""
	slice := strings.Fields(sys.Grep(string(contents), "key"))
	if len(slice) >= 3 {
		secret = slice[2]
	}
	if secret == "" {
		return "", errors.New("failed to parse secret")
	}
	return secret, nil
}

// PopulateExternalClusterInfo Add validation in the code to fail if the external cluster has no OSDs keep waiting
func PopulateExternalClusterInfo(context *clusterd.Context, ctx context.Context, namespace string, ownerInfo *k8sutil.OwnerInfo) *cephclient.ClusterInfo {
	for {
		clusterInfo, _, _, err := LoadClusterInfo(context, ctx, namespace)
		if err != nil {
			logger.Warningf("waiting for the csi connection info of the external cluster. retrying in %s.", externalConnectionRetry.String())
			logger.Debugf("%v", err)
			time.Sleep(externalConnectionRetry)
			continue
		}
		logger.Infof("found the cluster info to connect to the external cluster. will use %q to check health and monitor status. mons=%+v", clusterInfo.CephCred.Username, clusterInfo.Monitors)
		clusterInfo.OwnerInfo = ownerInfo
		return clusterInfo
	}
}
