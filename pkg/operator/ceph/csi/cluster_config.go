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
	cephv1 "github.com/rook/rook/pkg/apis/ceph.rook.io/v1"
	cephclient "github.com/rook/rook/pkg/daemon/ceph/client"
	"github.com/rook/rook/pkg/operator/k8sutil"
	v1 "k8s.io/api/core/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
)

var (
	logger      = capnslog.NewPackageLogger("github.com/rook/rook", "ceph-csi")
	configMutex sync.Mutex
)

type CsiClusterConfigEntry struct {
	ClusterID       string              `json:"clusterID"`
	Monitors        []string            `json:"monitors"`
	SubvolumeGroups []CsiSubvolumeGroup `json:"subvolumeGroups,omitempty"`
	RadosNamespaces []CsiRadosNamespace `json:"radosNamespaces,omitempty"`
	// Deprecated
	CephFS *CsiCephFSSpec `json:"cephFS,omitempty"`
	// Deprecated
	RadosNamespace string `json:"radosNamespace,omitempty"`
}

type CsiSubvolumeGroup struct {
	Name       string `json:"name"`
	Filesystem string `json:"filesystem"`
}
type CsiRadosNamespace struct {
	Pool      string `json:"pool"`
	Namespace string `json:"namespace"`
}

type CsiCephFSSpec struct {
	SubvolumeGroup string `json:"subvolumeGroup,omitempty"`
}

// FormatCsiClusterConfig returns a json-formatted string containing
// the cluster-to-mon mapping required to configure ceph csi.
func FormatCsiClusterConfig(
	clusterKey string, mons map[string]*cephclient.MonInfo) (string, error) {

	cc := make([]CsiClusterConfigEntry, 1)
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

func parseCsiClusterConfig(c string) ([]CsiClusterConfigEntry, error) {
	var cc []CsiClusterConfigEntry
	err := json.Unmarshal([]byte(c), &cc)
	if err != nil {
		return cc, errors.Wrap(err, "failed to parse csi cluster config")
	}
	return cc, nil
}

func formatCsiClusterConfig(cc []CsiClusterConfigEntry) (string, error) {
	ccJson, err := json.Marshal(cc)
	if err != nil {
		return "", errors.Wrap(err, "failed to marshal csi cluster config")
	}
	return string(ccJson), nil
}

func MonEndpoints(mons map[string]*cephclient.MonInfo) []string {
	endpoints := make([]string, 0)
	for _, m := range mons {
		endpoints = append(endpoints, m.Endpoint)
	}
	return endpoints
}

// CreateCsiConfigMap creates an empty config map that will be later used
// to provide cluster configuration to ceph-csi. If a config map already
// exists, it will return it.
func CreateCsiConfigMap(ctx context.Context, namespace string, clientset kubernetes.Interface, ownerInfo *k8sutil.OwnerInfo) error {
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

	logger.Infof("successfully created csi config map %q in ns %q", configMap.Name, namespace)
	return nil
}

// SaveClusterConfig updates the config map used to provide ceph-csi with
// basic cluster configuration. The clusterNamespace and clusterInfo are
// used to determine what "cluster" in the config map will be updated and
// and the clusterNamespace value is expected to match the clusterID
// value that is provided to ceph-csi uses in the storage class.
// A mutex is used to prevent the config map from being updated for
// multiple clusters simultaneously.
func SaveClusterConfig(clientset kubernetes.Interface, clusterInfo *cephclient.ClusterInfo, clusterConfig *CsiClusterConfigEntry) error {
	updateConfig := func(cc []CsiClusterConfigEntry) ([]CsiClusterConfigEntry, error) {
		for i, c := range cc {
			// Update the mons if the entry already exists
			if c.ClusterID == clusterConfig.ClusterID {
				c.Monitors = clusterConfig.Monitors
				cc[i] = c
				return cc, nil
			}
		}
		// Create the new entry since it was not found
		newCluster := CsiClusterConfigEntry{ClusterID: clusterConfig.ClusterID, Monitors: clusterConfig.Monitors}
		return append(cc, newCluster), nil
	}

	return updateClusterConfig(clientset, clusterInfo, updateConfig)
}

func AddRadosNamespace(clientset kubernetes.Interface, clusterInfo *cephclient.ClusterInfo, radosNamespace *cephv1.CephBlockPoolRadosNamespace) error {
	customUpdate := func(cc []CsiClusterConfigEntry) ([]CsiClusterConfigEntry, error) {
		for i, c := range cc {
			if c.ClusterID != radosNamespace.Namespace {
				continue
			}
			for _, ns := range c.RadosNamespaces {
				if ns.Pool == radosNamespace.Spec.BlockPoolName && ns.Namespace == radosNamespace.Name {
					logger.Infof("rados namespace %q in pool %q already in csi configmap", ns.Namespace, ns.Pool)
					return cc, nil
				}
			}
			// Add the rados namespace to the list since it wasn't found
			newNamespace := CsiRadosNamespace{Namespace: radosNamespace.Name, Pool: radosNamespace.Spec.BlockPoolName}
			c.RadosNamespaces = append(c.RadosNamespaces, newNamespace)
			cc[i] = c
			logger.Infof("added rados namespace %q in pool %q to csi configmap", radosNamespace.Name, radosNamespace.Spec.BlockPoolName)
			return cc, nil
		}
		return cc, errors.Errorf("failed to find cluster %q in configmap", radosNamespace.Namespace)
	}

	return updateClusterConfig(clientset, clusterInfo, customUpdate)
}

func RemoveRadosNamespace(clientset kubernetes.Interface, clusterInfo *cephclient.ClusterInfo, radosNamespace *cephv1.CephBlockPoolRadosNamespace) error {
	customUpdate := func(cc []CsiClusterConfigEntry) ([]CsiClusterConfigEntry, error) {
		for i, c := range cc {
			if c.ClusterID != radosNamespace.Namespace {
				continue
			}
			for j, ns := range c.RadosNamespaces {
				if ns.Pool == radosNamespace.Spec.BlockPoolName && ns.Namespace == radosNamespace.Name {
					c.RadosNamespaces = append(c.RadosNamespaces[:j], c.RadosNamespaces[j+1:]...)
					cc[i] = c
					logger.Infof("removed rados namespace %q in pool %q from csi configmap", ns.Namespace, ns.Pool)
					return cc, nil
				}
			}
			logger.Warningf("rados namespace %q in pool %q not found in csi configmap", radosNamespace.Name, radosNamespace.Spec.BlockPoolName)
			return cc, nil
		}
		logger.Warningf("could not find cluster %q in csi configmap to remove rados namespace %q in pool %q", radosNamespace.Namespace, radosNamespace.Name, radosNamespace.Spec.BlockPoolName)
		return cc, nil
	}

	return updateClusterConfig(clientset, clusterInfo, customUpdate)
}

func RemoveSubvolumeGroup(clientset kubernetes.Interface, clusterInfo *cephclient.ClusterInfo, subvolumeGroup *cephv1.CephFilesystemSubVolumeGroup) error {
	customUpdate := func(cc []CsiClusterConfigEntry) ([]CsiClusterConfigEntry, error) {
		for i, c := range cc {
			if c.ClusterID != subvolumeGroup.Namespace {
				continue
			}
			for j, group := range c.SubvolumeGroups {
				if group.Name == subvolumeGroup.Name && group.Filesystem == subvolumeGroup.Spec.FilesystemName {
					c.SubvolumeGroups = append(c.SubvolumeGroups[:j], c.SubvolumeGroups[j+1:]...)
					cc[i] = c
					logger.Infof("removed subvolume group %q in filesystem %q from csi configmap", group.Name, group.Filesystem)
					return cc, nil
				}
			}
			logger.Warningf("subvolume group %q in filesystem %q not found in csi configmap", subvolumeGroup.Name, subvolumeGroup.Spec.FilesystemName)
			return cc, nil
		}
		logger.Warningf("could not find cluster %q in csi configmap to remove subvolume group %q in filesystem %q", subvolumeGroup.Namespace, subvolumeGroup.Name, subvolumeGroup.Spec.FilesystemName)
		return cc, nil
	}

	return updateClusterConfig(clientset, clusterInfo, customUpdate)
}

func AddSubvolumeGroup(clientset kubernetes.Interface, clusterInfo *cephclient.ClusterInfo, subvolumeGroup *cephv1.CephFilesystemSubVolumeGroup) error {
	customUpdate := func(cc []CsiClusterConfigEntry) ([]CsiClusterConfigEntry, error) {
		for i, c := range cc {
			if c.ClusterID != subvolumeGroup.Namespace {
				continue
			}
			for _, group := range c.SubvolumeGroups {
				if group.Filesystem == subvolumeGroup.Spec.FilesystemName && group.Name == subvolumeGroup.Name {
					logger.Infof("subvolumegroup %q in filesystem %q already in csi configmap", group.Name, group.Filesystem)
					return cc, nil
				}
			}
			// Add the subvolume group to the list since it wasn't found
			newGroup := CsiSubvolumeGroup{Name: subvolumeGroup.Name, Filesystem: subvolumeGroup.Spec.FilesystemName}
			c.SubvolumeGroups = append(c.SubvolumeGroups, newGroup)
			cc[i] = c
			logger.Infof("added subvolumegroup %q in filesystem %q to csi configmap", subvolumeGroup.Name, subvolumeGroup.Spec.FilesystemName)
			return cc, nil
		}
		return nil, errors.Errorf("failed to find cluster %q in configmap", subvolumeGroup.Namespace)
	}

	return updateClusterConfig(clientset, clusterInfo, customUpdate)
}

func updateClusterConfig(clientset kubernetes.Interface, clusterInfo *cephclient.ClusterInfo,
	csiConfigCallback func(config []CsiClusterConfigEntry) ([]CsiClusterConfigEntry, error)) error {

	// csi is deployed into the same namespace as the operator
	csiNamespace := os.Getenv(k8sutil.PodNamespaceEnvVar)
	if csiNamespace == "" {
		logger.Warningf("cannot save csi config due to missing env var %q", k8sutil.PodNamespaceEnvVar)
		return nil
	}
	logger.Debugf("using %q for csi configmap namespace", csiNamespace)

	// ensure that only one update is made to the configmap at a time
	configMutex.Lock()
	defer configMutex.Unlock()

	// fetch current ConfigMap contents
	configMap, err := clientset.CoreV1().ConfigMaps(csiNamespace).Get(clusterInfo.Context, ConfigName, metav1.GetOptions{})
	if err != nil {
		return errors.Wrap(err, "failed to fetch current csi config map")
	}

	// update ConfigMap contents for current cluster
	currData := configMap.Data[ConfigKey]
	if currData == "" {
		currData = "[]"
	}
	currentConfig, err := parseCsiClusterConfig(currData)
	if err != nil {
		return errors.Wrap(err, "failed to parse current csi cluster config")
	}

	// execute the callback to update the configuration in memory
	newConfig, err := csiConfigCallback(currentConfig)
	if err != nil {
		return errors.Wrap(err, "failed to process csi config")
	}

	// serialize the configuration
	serialized, err := formatCsiClusterConfig(newConfig)
	if err != nil {
		return errors.Wrap(err, "failed to serialize csi config")
	}
	configMap.Data[ConfigKey] = serialized

	// update ConfigMap with new contents
	if _, err := clientset.CoreV1().ConfigMaps(csiNamespace).Update(clusterInfo.Context, configMap, metav1.UpdateOptions{}); err != nil {
		return errors.Wrap(err, "failed to update csi config map")
	}

	return nil
}
