/*
Copyright 2022 The Rook Authors. All rights reserved.

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

package controller

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/pkg/errors"
	cephv1 "github.com/rook/rook/pkg/apis/ceph.rook.io/v1"
	"github.com/rook/rook/pkg/clusterd"
	cephclient "github.com/rook/rook/pkg/daemon/ceph/client"
	"github.com/rook/rook/pkg/operator/ceph/cluster/osd/topology"
	"github.com/rook/rook/pkg/operator/k8sutil"
	"github.com/rook/rook/pkg/util/exec"
	"github.com/rook/rook/pkg/util/sys"
	v1 "k8s.io/api/core/v1"
	kerrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
)

const (
	// OperatorCreds is the name of the secret
	//nolint:gosec // since this is not leaking any hardcoded credentials, it's just the secret name
	OperatorCreds     = "rook-ceph-operator-creds"
	fsidSecretNameKey = "fsid"
	MonSecretNameKey  = "mon-secret"
	// AdminSecretName is the name of the admin secret
	AdminSecretNameKey = "admin-secret"
	CephUsernameKey    = "ceph-username"
	CephUserSecretKey  = "ceph-secret"
	// EndpointConfigMapName is the name of the configmap with mon endpoints
	EndpointConfigMapName = "rook-ceph-mon-endpoints"
	// EndpointDataKey is the name of the key inside the mon configmap to get the endpoints
	EndpointDataKey = "data"
	// EndpointExternalMonsKey key in EndpointConfigMapName configmap containing IDs of external mons
	EndpointExternalMonsKey = "externalMons"
	// OutOfQuorumKey is the name of the key for tracking mons detected out of quorum
	OutOfQuorumKey = "outOfQuorum"
	// MaxMonIDKey is the name of the max mon id used
	MaxMonIDKey = "maxMonId"
	// MappingKey is the name of the mapping for the mon->node and node->port
	MappingKey = "mapping"
	// AppName is the name of the secret storing cluster mon.admin key, fsid and name
	AppName                         = "rook-ceph-mon"
	DisasterProtectionFinalizerName = cephv1.CustomResourceGroup + "/disaster-protection"
	// MonCephxUser represents the cephx user name for all the mon daemons
	MonCephxUser = "mon."
)

var (
	ClusterInfoNoClusterNoSecret = errors.New("not expected to create new cluster info and did not find existing secret")
	externalConnectionRetry      = 60 * time.Second
)

// Mapping is mon node and port mapping
type Mapping struct {
	// This isn't really node info since it could also be for zones, but we leave it as "node" for backward compatibility.
	Schedule map[string]*MonScheduleInfo `json:"node"`
}

// MonScheduleInfo contains name and address of a node.
type MonScheduleInfo struct {
	// Name of the node. **json names are capitalized for backwards compat**
	Name     string `json:"Name,omitempty"`
	Hostname string `json:"Hostname,omitempty"`
	Address  string `json:"Address,omitempty"`
	Zone     string `json:"zone,omitempty"`
}

// LoadClusterInfo constructs or loads a clusterinfo and returns it along with the maxMonID
func LoadClusterInfo(ctx *clusterd.Context, context context.Context, namespace string, cephClusterSpec *cephv1.ClusterSpec) (*cephclient.ClusterInfo, int, *Mapping, error) {
	return CreateOrLoadClusterInfo(ctx, context, namespace, nil, cephClusterSpec)
}

// CreateOrLoadClusterInfo constructs or loads a clusterinfo and returns it along with the maxMonID
func CreateOrLoadClusterInfo(clusterdContext *clusterd.Context, context context.Context, namespace string, ownerInfo *k8sutil.OwnerInfo, cephClusterSpec *cephv1.ClusterSpec) (*cephclient.ClusterInfo, int, *Mapping, error) {
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
			return nil, maxMonID, monMapping, errors.Wrap(err, "failed to create initial cluster info")
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
			MonitorSecret: string(secrets.Data[MonSecretNameKey]),
			Context:       context,
		}
		if cephUsername, ok := secrets.Data[CephUsernameKey]; ok {
			clusterInfo.CephCred.Username = string(cephUsername)
			clusterInfo.CephCred.Secret = string(secrets.Data[CephUserSecretKey])
		} else if adminSecretKey, ok := secrets.Data[AdminSecretNameKey]; ok {
			clusterInfo.CephCred.Username = cephclient.AdminUsername
			clusterInfo.CephCred.Secret = string(adminSecretKey)

			secrets.Data[CephUsernameKey] = []byte(cephclient.AdminUsername)
			secrets.Data[CephUserSecretKey] = adminSecretKey
			if _, err = clusterdContext.Clientset.CoreV1().Secrets(namespace).Update(context, secrets, metav1.UpdateOptions{}); err != nil {
				return nil, maxMonID, monMapping, errors.Wrap(err, "failed to update mon secrets")
			}
		} else {
			return nil, maxMonID, monMapping, errors.New("failed to find either the cluster admin key or the username")
		}
		if len(secrets.OwnerReferences) > 0 {
			clusterInfo.OwnerInfo = k8sutil.NewOwnerInfoWithOwnerRef(&secrets.GetOwnerReferences()[0], namespace)
		}
		logger.Debugf("found existing monitor secrets for cluster %s", clusterInfo.Namespace)
	}

	// get the existing monitor config
	clusterInfo.ExternalMons, clusterInfo.InternalMonitors, maxMonID, monMapping, err = loadMonConfig(clusterdContext.Clientset, namespace)
	if err != nil {
		return nil, maxMonID, monMapping, errors.Wrap(err, "failed to get mon config")
	}

	// update clusterInfo with cephCLusterSpec Network info
	// it will also update if Multus is enabled, so ceph cmds
	// can run on remote executor
	clusterInfo.NetworkSpec = cephClusterSpec.Network

	// update clusterInfo with cephClusterSpec.CSI.
	clusterInfo.CSIDriverSpec = cephClusterSpec.CSI
	if len(clusterInfo.CSIDriverSpec.ReadAffinity.CrushLocationLabels) == 0 {
		clusterInfo.CSIDriverSpec.ReadAffinity.CrushLocationLabels = strings.Split(topology.GetDefaultTopologyLabels(), ",")
	}

	// If an admin key was provided we don't need to load the other resources
	// Some people might want to give the admin key
	// The necessary users/keys/secrets will be created by Rook
	// This is also done to allow backward compatibility
	if clusterInfo.CephCred.Username == cephclient.AdminUsername && clusterInfo.CephCred.Secret != AdminSecretNameKey {
		return clusterInfo, maxMonID, monMapping, nil
	}

	// If the admin secret is "admin-secret", look for the deprecated secret that has the external creds
	if clusterInfo.CephCred.Secret == AdminSecretNameKey {
		secret, err := clusterdContext.Clientset.CoreV1().Secrets(namespace).Get(context, OperatorCreds, metav1.GetOptions{})
		if err != nil {
			return nil, maxMonID, monMapping, err
		}
		// Populate external credential
		clusterInfo.CephCred.Username = string(secret.Data["userID"])
		clusterInfo.CephCred.Secret = string(secret.Data["userKey"])
	}

	return clusterInfo, maxMonID, monMapping, nil
}

// create new cluster info (FSID, shared keys)
func createNamedClusterInfo(context *clusterd.Context, namespace string) (*cephclient.ClusterInfo, error) {
	fsid, err := uuid.NewRandom()
	if err != nil {
		return nil, err
	}

	dir := path.Join(context.ConfigDir, namespace)
	if err = os.MkdirAll(dir, 0o744); err != nil {
		return nil, errors.Wrapf(err, "failed to create dir %s", dir)
	}

	// generate the mon secret
	monSecret, err := genSecret(context.Executor, dir, MonCephxUser, []string{"--cap", "mon", "'allow *'"})
	if err != nil {
		return nil, err
	}

	// generate the admin secret if one was not provided at the command line
	args := []string{
		"--cap", "mon", "'allow *'",
		"--cap", "osd", "'allow *'",
		"--cap", "mgr", "'allow *'",
		"--cap", "mds", "'allow'",
	}
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

	contents, err := os.ReadFile(filepath.Clean(path))
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

func UpdateMonsOutOfQuorum(clientset kubernetes.Interface, namespace string, monsOutOfQuorum []string) error {
	ctx := context.TODO()
	cm, err := clientset.CoreV1().ConfigMaps(namespace).Get(ctx, EndpointConfigMapName, metav1.GetOptions{})
	if err != nil {
		return errors.Wrap(err, "failed to get mon endpoints configmap")
	}

	cm.Data[OutOfQuorumKey] = strings.Join(monsOutOfQuorum, ",")
	_, err = clientset.CoreV1().ConfigMaps(namespace).Update(ctx, cm, metav1.UpdateOptions{})
	if err != nil {
		return errors.Wrap(err, "failed to update mon endpoints configmap with mon(s) out of quorum")
	}
	return nil
}

// loadMonConfig returns the monitor endpoints and maxMonID
func loadMonConfig(clientset kubernetes.Interface, namespace string) (extMons map[string]*cephclient.MonInfo, internalMons map[string]*cephclient.MonInfo, maxMonID int, monMapping *Mapping, err error) {
	ctx := context.TODO()
	extMons = map[string]*cephclient.MonInfo{}
	internalMons = map[string]*cephclient.MonInfo{}
	maxMonID = -1
	monMapping = &Mapping{
		Schedule: map[string]*MonScheduleInfo{},
	}

	cm, err := clientset.CoreV1().ConfigMaps(namespace).Get(ctx, EndpointConfigMapName, metav1.GetOptions{})
	if err != nil {
		if !kerrors.IsNotFound(err) {
			return nil, nil, maxMonID, monMapping, err
		}
		// If the config map was not found, initialize the empty set of monitors
		return extMons, internalMons, maxMonID, monMapping, nil
	}

	// Parse the monitor List
	if info, ok := cm.Data[EndpointDataKey]; ok {
		internalMons = ParseMonEndpoints(info)
	}

	// Parse the mons that were detected out of quorum
	if outOfQuorum, ok := cm.Data[OutOfQuorumKey]; ok && len(outOfQuorum) > 0 {
		monNames := strings.Split(outOfQuorum, ",")
		for _, monName := range monNames {
			if monInfo, ok := internalMons[monName]; ok {
				monInfo.OutOfQuorum = true
			} else {
				logger.Warningf("did not find mon %q to set it out of quorum in the cluster info", monName)
			}
		}
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
	for _, m := range internalMons {
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
	// filter external mons:
	if extMonIDsStr, ok := cm.Data[EndpointExternalMonsKey]; ok && extMonIDsStr != "" {
		extMonIDs := strings.Split(extMonIDsStr, ",")
		for monID, mon := range internalMons {
			if slices.Contains(extMonIDs, monID) {
				extMons[monID] = mon
				delete(internalMons, monID)
			}
		}
	}

	logger.Debugf("loaded: maxMonID=%d, extMons=%+v, mons=%+v, assignment=%+v", maxMonID, extMons, internalMons, monMapping)
	return extMons, internalMons, maxMonID, monMapping, nil
}

// convert the mon name to the numeric mon ID
func fullNameToIndex(name string) (int, error) {
	// remove the "rook-ceph-mon" prefix
	name = strings.TrimPrefix(name, AppName)
	// remove the "-" prefix
	name = strings.TrimPrefix(name, "-")
	return k8sutil.NameToIndex(name)
}

func createClusterAccessSecret(clientset kubernetes.Interface, namespace string, clusterInfo *cephclient.ClusterInfo, ownerInfo *k8sutil.OwnerInfo) error {
	logger.Infof("creating mon secrets for a new cluster")
	var err error

	// store the secrets for internal usage of the rook pods
	secrets := map[string][]byte{
		fsidSecretNameKey: []byte(clusterInfo.FSID),
		MonSecretNameKey:  []byte(clusterInfo.MonitorSecret),
		CephUsernameKey:   []byte(clusterInfo.CephCred.Username),
		CephUserSecretKey: []byte(clusterInfo.CephCred.Secret),
	}
	secret := &v1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:       AppName,
			Namespace:  namespace,
			Finalizers: []string{DisasterProtectionFinalizerName},
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

// UpdateClusterAccessSecret updates the rook-ceph-mon secret
func UpdateClusterAccessSecret(clientset kubernetes.Interface, clusterInfo *cephclient.ClusterInfo) error {
	secret, err := clientset.CoreV1().Secrets(clusterInfo.Namespace).Get(clusterInfo.Context, AppName, metav1.GetOptions{})
	if err != nil {
		return errors.Wrapf(err, "failed to get secret %q in the namespace %q", AppName, clusterInfo.Namespace)
	}

	// update admin key secret
	secret.Data[CephUserSecretKey] = []byte(clusterInfo.CephCred.Secret)

	secret.Data[MonSecretNameKey] = []byte(clusterInfo.MonitorSecret)

	logger.Infof("updating secret %q in the namespace %q", secret.Name, clusterInfo.Namespace)
	_, err = clientset.CoreV1().Secrets(clusterInfo.Namespace).Update(clusterInfo.Context, secret, metav1.UpdateOptions{})
	if err != nil {
		return errors.Wrapf(err, "failed to update secret %q in the namespace %q", secret.Name, clusterInfo.Namespace)
	}

	return nil
}

// ParseMonEndpoints parses a flattened representation of mons and endpoints in the form
// <mon-name>=<mon-endpoint> and returns a list of Ceph mon configs.
func ParseMonEndpoints(input string) map[string]*cephclient.MonInfo {
	logger.Infof("parsing mon endpoints: %s", input)
	mons := map[string]*cephclient.MonInfo{}
	rawMons := strings.Split(input, ",")
	for _, rawMon := range rawMons {
		parts := strings.Split(rawMon, "=")
		if len(parts) != 2 {
			logger.Warningf("ignoring invalid monitor %s", rawMon)
			continue
		}
		mons[parts[0]] = &cephclient.MonInfo{Name: parts[0], Endpoint: parts[1]}
	}
	return mons
}

// PopulateExternalClusterInfo Add validation in the code to fail if the external cluster has no
// OSDs keep waiting
func PopulateExternalClusterInfo(cephClusterSpec *cephv1.ClusterSpec, context *clusterd.Context, ctx context.Context, namespace string, ownerInfo *k8sutil.OwnerInfo) (*cephclient.ClusterInfo, error) {
	for {
		// Checking for the context makes sure we don't loop forever with a canceled context
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		clusterInfo, _, _, err := CreateOrLoadClusterInfo(context, ctx, namespace, nil, cephClusterSpec)
		if err != nil {
			logger.Warningf("waiting for connection info of the external cluster. retrying in %s.", externalConnectionRetry.String())
			logger.Debugf("%v", err)
			time.Sleep(externalConnectionRetry)
			continue
		}
		logger.Infof("found the cluster info to connect to the external cluster. will use %q to check health and monitor status. mons=%+v", clusterInfo.CephCred.Username, clusterInfo.AllMonitors())
		clusterInfo.OwnerInfo = ownerInfo

		return clusterInfo, nil
	}
}
