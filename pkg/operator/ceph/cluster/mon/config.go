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
	"encoding/json"
	"fmt"
	"io/ioutil"
	"os"
	"path"
	"strconv"
	"strings"

	"github.com/google/uuid"
	"github.com/rook/rook/pkg/clusterd"
	"github.com/rook/rook/pkg/daemon/ceph/client"
	cephconfig "github.com/rook/rook/pkg/daemon/ceph/config"
	"github.com/rook/rook/pkg/operator/k8sutil"
	"github.com/rook/rook/pkg/util/exec"
	"github.com/rook/rook/pkg/util/sys"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
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
)

func (c *Cluster) genMonSharedKeyring() string {
	return fmt.Sprintf(
		keyringTemplate,
		c.clusterInfo.MonitorSecret,
		cephconfig.AdminKeyring(c.clusterInfo),
	)
}

// return mon data dir path relative to the dataDirHostPath given a mon's name
func dataDirRelativeHostPath(monName string) string {
	monHostDir := monName // support legacy case where the mon name is "mon#" and not a lettered ID
	if strings.Index(monName, "mon") == -1 {
		// if the mon name doesn't have "mon" in it, mon dir is "mon-<ID>"
		monHostDir = "mon-" + monName
	}
	// Keep existing behavior where Rook stores the mon's data in the "data" subdir
	return path.Join(monHostDir, "data")
}

// LoadClusterInfo constructs or loads a clusterinfo and returns it along with the maxMonID
func LoadClusterInfo(context *clusterd.Context, namespace string) (*cephconfig.ClusterInfo, int, *Mapping, error) {
	return CreateOrLoadClusterInfo(context, namespace, nil)
}

// CreateOrLoadClusterInfo constructs or loads a clusterinfo and returns it along with the maxMonID
func CreateOrLoadClusterInfo(context *clusterd.Context, namespace string, ownerRef *metav1.OwnerReference) (*cephconfig.ClusterInfo, int, *Mapping, error) {
	var clusterInfo *cephconfig.ClusterInfo
	maxMonID := -1
	monMapping := &Mapping{
		Node: map[string]*NodeInfo{},
		Port: map[string]int32{},
	}

	secrets, err := context.Clientset.CoreV1().Secrets(namespace).Get(appName, metav1.GetOptions{})
	if err != nil {
		if !errors.IsNotFound(err) {
			return nil, maxMonID, monMapping, fmt.Errorf("failed to get mon secrets. %+v", err)
		}
		if ownerRef == nil {
			return nil, maxMonID, monMapping, fmt.Errorf("not expected to create new cluster info and did not find existing secret")
		}

		clusterInfo, err = createNamedClusterInfo(context, namespace)
		if err != nil {
			return nil, maxMonID, monMapping, fmt.Errorf("failed to create mon secrets. %+v", err)
		}

		err = createClusterAccessSecret(context.Clientset, namespace, clusterInfo, ownerRef)
		if err != nil {
			return nil, maxMonID, monMapping, err
		}
	} else {
		clusterInfo = &cephconfig.ClusterInfo{
			Name:          string(secrets.Data[clusterSecretName]),
			FSID:          string(secrets.Data[fsidSecretName]),
			MonitorSecret: string(secrets.Data[monSecretName]),
			AdminSecret:   string(secrets.Data[adminSecretName]),
		}
		logger.Debugf("found existing monitor secrets for cluster %s", clusterInfo.Name)
	}

	// get the existing monitor config
	clusterInfo.Monitors, maxMonID, monMapping, err = loadMonConfig(context.Clientset, namespace)
	if err != nil {
		return nil, maxMonID, monMapping, fmt.Errorf("failed to get mon config. %+v", err)
	}

	return clusterInfo, maxMonID, monMapping, nil
}

// writeConnectionConfig save monitor connection config to disk
func writeConnectionConfig(context *clusterd.Context, clusterInfo *cephconfig.ClusterInfo) error {
	// write the latest config to the config dir
	if err := cephconfig.GenerateAdminConnectionConfig(context, clusterInfo); err != nil {
		return fmt.Errorf("failed to write connection config. %+v", err)
	}

	return nil
}

// loadMonConfig returns the monitor endpoints and maxMonID
func loadMonConfig(clientset kubernetes.Interface, namespace string) (map[string]*cephconfig.MonInfo, int, *Mapping, error) {
	monEndpointMap := map[string]*cephconfig.MonInfo{}
	maxMonID := -1
	monMapping := &Mapping{
		Node: map[string]*NodeInfo{},
		Port: map[string]int32{},
	}

	cm, err := clientset.CoreV1().ConfigMaps(namespace).Get(EndpointConfigMapName, metav1.GetOptions{})
	if err != nil {
		if !errors.IsNotFound(err) {
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
	if id, ok := cm.Data[MaxMonIDKey]; ok {
		maxMonID, err = strconv.Atoi(id)
		if err != nil {
			logger.Errorf("invalid max mon id %s. %+v", id, err)
		}
	}

	// Make sure the max id is consistent with the current monitors
	for _, m := range monEndpointMap {
		id, _ := fullNameToIndex(m.Name)
		if maxMonID < id {
			maxMonID = id
		}
	}

	err = json.Unmarshal([]byte(cm.Data[MappingKey]), &monMapping)
	if err != nil {
		logger.Errorf("invalid JSON in mon mapping. %+v", err)
	}

	logger.Infof("loaded: maxMonID=%d, mons=%+v, mapping=%+v", maxMonID, monEndpointMap, monMapping)
	return monEndpointMap, maxMonID, monMapping, nil
}

func createClusterAccessSecret(clientset kubernetes.Interface, namespace string, clusterInfo *cephconfig.ClusterInfo, ownerRef *metav1.OwnerReference) error {
	logger.Infof("creating mon secrets for a new cluster")
	var err error

	// store the secrets for internal usage of the rook pods
	secrets := map[string][]byte{
		clusterSecretName: []byte(clusterInfo.Name),
		fsidSecretName:    []byte(clusterInfo.FSID),
		monSecretName:     []byte(clusterInfo.MonitorSecret),
		adminSecretName:   []byte(clusterInfo.AdminSecret),
	}
	secret := &v1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      appName,
			Namespace: namespace,
		},
		Data: secrets,
		Type: k8sutil.RookType,
	}
	k8sutil.SetOwnerRef(&secret.ObjectMeta, ownerRef)

	if _, err = clientset.CoreV1().Secrets(namespace).Create(secret); err != nil {
		return fmt.Errorf("failed to save mon secrets. %+v", err)
	}

	return nil
}

// create new cluster info (FSID, shared keys)
func createNamedClusterInfo(context *clusterd.Context, clusterName string) (*cephconfig.ClusterInfo, error) {
	fsid, err := uuid.NewRandom()
	if err != nil {
		return nil, err
	}

	dir := path.Join(context.ConfigDir, clusterName)
	if err = os.MkdirAll(dir, 0744); err != nil {
		return nil, fmt.Errorf("failed to create dir %s. %+v", dir, err)
	}

	// generate the mon secret
	monSecret, err := genSecret(context.Executor, dir, "mon.", []string{"--cap", "mon", "'allow *'"})
	if err != nil {
		return nil, err
	}

	// generate the admin secret if one was not provided at the command line
	args := []string{"--cap", "mon", "'allow *'", "--cap", "osd", "'allow *'", "--cap", "mgr", "'allow *'", "--cap", "mds", "'allow'"}
	adminSecret, err := genSecret(context.Executor, dir, client.AdminUsername, args)
	if err != nil {
		return nil, err
	}

	return &cephconfig.ClusterInfo{
		FSID:          fsid.String(),
		MonitorSecret: monSecret,
		AdminSecret:   adminSecret,
		Name:          clusterName,
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
	_, err := executor.ExecuteCommandWithOutput(false, "gen secret", "ceph-authtool", args...)
	if err != nil {
		return "", fmt.Errorf("failed to gen secret. %+v", err)
	}

	contents, err := ioutil.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("failed to read secret file. %+v", err)
	}
	return extractKey(string(contents))
}

func extractKey(contents string) (string, error) {
	secret := ""
	slice := strings.Fields(sys.Grep(string(contents), "key"))
	if len(slice) >= 3 {
		secret = slice[2]
	}
	if secret == "" {
		return "", fmt.Errorf("failed to parse secret")
	}
	return secret, nil
}
