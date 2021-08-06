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
	"context"
	"encoding/json"
	"os"
	"sync"

	"github.com/coreos/pkg/capnslog"
	"github.com/pkg/errors"
	cephclient "github.com/rook/rook/pkg/daemon/ceph/client"
	"github.com/rook/rook/pkg/operator/k8sutil"
	v1 "k8s.io/api/core/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
)

var (
	logger = capnslog.NewPackageLogger("github.com/rook/rook", "ceph-csi")
)

type csiClusterConfigEntry struct {
	ClusterID string   `json:"clusterID"`
	Monitors  []string `json:"monitors"`
}

type csiClusterConfig []csiClusterConfigEntry

// FormatCsiClusterConfig returns a json-formatted string containing
// the cluster-to-mon mapping required to configure ceph csi.
func FormatCsiClusterConfig(
	clusterKey string, mons map[string]*cephclient.MonInfo) (string, error) {

	cc := make(csiClusterConfig, 1)
	cc[0].ClusterID = clusterKey
	cc[0].Monitors = []string{}
	for _, m := range mons {
		cc[0].Monitors = append(cc[0].Monitors, m.Endpoint)
	}

	ccJson, err := json.Marshal(cc)
	if err != nil {
		return "", errors.Wrap(err, "failed to marshal csi cluster config")
	}
	return string(ccJson), nil
}

func parseCsiClusterConfig(c string) (csiClusterConfig, error) {
	var cc csiClusterConfig
	err := json.Unmarshal([]byte(c), &cc)
	if err != nil {
		return cc, errors.Wrap(err, "failed to parse csi cluster config")
	}
	return cc, nil
}

func formatCsiClusterConfig(cc csiClusterConfig) (string, error) {
	ccJson, err := json.Marshal(cc)
	if err != nil {
		return "", errors.Wrap(err, "failed to marshal csi cluster config")
	}
	return string(ccJson), nil
}

func monEndpoints(mons map[string]*cephclient.MonInfo) []string {
	endpoints := make([]string, 0)
	for _, m := range mons {
		endpoints = append(endpoints, m.Endpoint)
	}
	return endpoints
}

// updateCsiClusterConfig returns a json-formatted string containing
// the cluster-to-mon mapping required to configure ceph csi.
func updateCsiClusterConfig(
	curr, clusterKey string, mons map[string]*cephclient.MonInfo) (string, error) {

	var (
		cc     csiClusterConfig
		centry csiClusterConfigEntry
		found  bool
	)
	cc, err := parseCsiClusterConfig(curr)
	if err != nil {
		return "", errors.Wrap(err, "failed to parse current csi cluster config")
	}

	for i, centry := range cc {
		if centry.ClusterID == clusterKey {
			centry.Monitors = monEndpoints(mons)
			found = true
			cc[i] = centry
			break
		}
	}
	if !found {
		centry.ClusterID = clusterKey
		centry.Monitors = monEndpoints(mons)
		cc = append(cc, centry)
	}
	return formatCsiClusterConfig(cc)
}

// CreateCsiConfigMap creates an empty config map that will be later used
// to provide cluster configuration to ceph-csi. If a config map already
// exists, it will return it.
func CreateCsiConfigMap(namespace string, clientset kubernetes.Interface, ownerInfo *k8sutil.OwnerInfo) error {
	ctx := context.TODO()
	configMap := &v1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      ConfigName,
			Namespace: namespace,
		},
	}
	configMap.Data = map[string]string{
		ConfigKey: "[]",
	}

	err := ownerInfo.SetControllerReference(configMap)
	if err != nil {
		return errors.Wrapf(err, "failed to set owner reference to csi configmap %q", configMap.Name)
	}
	_, err = clientset.CoreV1().ConfigMaps(namespace).Create(ctx, configMap, metav1.CreateOptions{})
	if err != nil {
		if !k8serrors.IsAlreadyExists(err) {
			return errors.Wrapf(err, "failed to create initial csi config map %q (in %q)", configMap.Name, namespace)
		}
	}

	logger.Infof("successfully created csi config map %q", configMap.Name)
	return nil
}

// SaveClusterConfig updates the config map used to provide ceph-csi with
// basic cluster configuration. The clusterNamespace and clusterInfo are
// used to determine what "cluster" in the config map will be updated and
// and the clusterNamespace value is expected to match the clusterID
// value that is provided to ceph-csi uses in the storage class.
// The locker l is typically a mutex and is used to prevent the config
// map from being updated for multiple clusters simultaneously.
func SaveClusterConfig(
	clientset kubernetes.Interface, clusterNamespace string,
	clusterInfo *cephclient.ClusterInfo, l sync.Locker) error {
	if !CSIEnabled() {
		return nil
	}
	l.Lock()
	defer l.Unlock()
	// csi is deployed into the same namespace as the operator
	csiNamespace := os.Getenv(k8sutil.PodNamespaceEnvVar)
	if csiNamespace == "" {
		return errors.Errorf("namespace value missing for %s", k8sutil.PodNamespaceEnvVar)
	}
	logger.Debugf("Using %+v for CSI ConfigMap Namespace", csiNamespace)

	// fetch current ConfigMap contents
	configMap, err := clientset.CoreV1().ConfigMaps(csiNamespace).Get(clusterInfo.Context,
		ConfigName, metav1.GetOptions{})
	if err != nil {
		return errors.Wrap(err, "failed to fetch current csi config map")
	}

	// update ConfigMap contents for current cluster
	currData := configMap.Data[ConfigKey]
	if currData == "" {
		currData = "[]"
	}
	newData, err := updateCsiClusterConfig(
		currData, clusterNamespace, clusterInfo.Monitors)
	if err != nil {
		return errors.Wrap(err, "failed to update csi config map data")
	}
	configMap.Data[ConfigKey] = newData

	// update ConfigMap with new contents
	if _, err := clientset.CoreV1().ConfigMaps(csiNamespace).Update(clusterInfo.Context, configMap, metav1.UpdateOptions{}); err != nil {
		return errors.Wrapf(err, "failed to update csi config map")
	}

	return nil
}
