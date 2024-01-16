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
	"fmt"
	"os"
	"strings"
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

	cephcsi "github.com/ceph/ceph-csi/api/deploy/kubernetes"
)

var (
	logger      = capnslog.NewPackageLogger("github.com/rook/rook", "ceph-csi")
	configMutex sync.Mutex
)

type CSIClusterConfigEntry struct {
	cephcsi.ClusterInfo
	Namespace string `json:"namespace"`
}

type csiClusterConfig []CSIClusterConfigEntry

// FormatCsiClusterConfig returns a json-formatted string containing
// the cluster-to-mon mapping required to configure ceph csi.
func FormatCsiClusterConfig(
	clusterKey string, mons map[string]*cephclient.MonInfo,
) (string, error) {
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

func MonEndpoints(mons map[string]*cephclient.MonInfo, requireMsgr2 bool) []string {
	endpoints := make([]string, 0)
	for _, m := range mons {
		endpoint := m.Endpoint
		if requireMsgr2 {
			logger.Debugf("evaluating mon %q for msgr1 on endpoint %q", m.Name, m.Endpoint)
			msgr1Suffix := fmt.Sprintf(":%d", cephclient.Msgr1port)
			if strings.HasSuffix(m.Endpoint, msgr1Suffix) {
				address := m.Endpoint[0:strings.LastIndex(m.Endpoint, msgr1Suffix)]
				endpoint = fmt.Sprintf("%s:%d", address, cephclient.Msgr2port)
				logger.Debugf("mon %q will use the msgrv2 port: %q", m.Name, endpoint)
			}
		}
		endpoints = append(endpoints, endpoint)
	}
	return endpoints
}

// updateCsiClusterConfig returns a json-formatted string containing
// the cluster-to-mon mapping required to configure ceph csi.
func updateCsiClusterConfig(curr, clusterKey string, newCsiClusterConfigEntry *CSIClusterConfigEntry) (string, error) {
	var (
		cc     csiClusterConfig
		centry CSIClusterConfigEntry
		found  bool
	)

	cc, err := parseCsiClusterConfig(curr)
	if err != nil {
		return "", errors.Wrap(err, "failed to parse current csi cluster config")
	}

	// Regardless of which controllers call updateCsiClusterConfig(), the values will be preserved since
	// a lock is acquired for the update operation. So concurrent updates (rare event) will block and
	// wait for the other update to complete. Monitors and Subvolumegroup will be updated
	// independently and won't collide.
	if newCsiClusterConfigEntry != nil {
		for i, centry := range cc {
			// If the clusterID belongs to the same cluster, update the entry.
			// update default clusterID's entry
			if clusterKey == centry.Namespace {
				centry.Monitors = newCsiClusterConfigEntry.Monitors
				cc[i] = centry
			}
		}
	}
	for i, centry := range cc {
		if centry.ClusterID == clusterKey {
			// If the new entry is nil, this means the entry is being deleted so remove it from the list
			if newCsiClusterConfigEntry == nil {
				cc = append(cc[:i], cc[i+1:]...)
				found = true
				break
			}
			centry.Monitors = newCsiClusterConfigEntry.Monitors
			if newCsiClusterConfigEntry.CephFS.SubvolumeGroup != "" || newCsiClusterConfigEntry.CephFS.NetNamespaceFilePath != "" {
				centry.CephFS = newCsiClusterConfigEntry.CephFS
			}
			if newCsiClusterConfigEntry.NFS.NetNamespaceFilePath != "" {
				centry.NFS = newCsiClusterConfigEntry.NFS
			}
			if newCsiClusterConfigEntry.RBD.RadosNamespace != "" || newCsiClusterConfigEntry.RBD.NetNamespaceFilePath != "" {
				centry.RBD = newCsiClusterConfigEntry.RBD
			}
			found = true
			cc[i] = centry
			break
		}
	}
	if !found {
		// If it's the first time we create the cluster, the entry does not exist, so the removal
		// will fail with a dangling pointer
		if newCsiClusterConfigEntry != nil {
			centry.ClusterID = clusterKey
			centry.Namespace = newCsiClusterConfigEntry.Namespace
			centry.Monitors = newCsiClusterConfigEntry.Monitors
			if newCsiClusterConfigEntry.RBD.RadosNamespace != "" || newCsiClusterConfigEntry.CephFS.NetNamespaceFilePath != "" {
				centry.RBD = newCsiClusterConfigEntry.RBD
			}
			// Add a condition not to fill with empty values
			if newCsiClusterConfigEntry.CephFS.SubvolumeGroup != "" || newCsiClusterConfigEntry.CephFS.NetNamespaceFilePath != "" {
				centry.CephFS = newCsiClusterConfigEntry.CephFS
			}
			if newCsiClusterConfigEntry.NFS.NetNamespaceFilePath != "" {
				centry.NFS = newCsiClusterConfigEntry.NFS
			}
			cc = append(cc, centry)
		}
	}

	return formatCsiClusterConfig(cc)
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

	logger.Infof("successfully created csi config map %q", configMap.Name)
	return nil
}

// SaveClusterConfig updates the config map used to provide ceph-csi with
// basic cluster configuration. The clusterNamespace and clusterInfo are
// used to determine what "cluster" in the config map will be updated and
// the clusterNamespace value is expected to match the clusterID
// value that is provided to ceph-csi uses in the storage class.
// The locker l is typically a mutex and is used to prevent the config
// map from being updated for multiple clusters simultaneously.
func SaveClusterConfig(clientset kubernetes.Interface, clusterNamespace string, clusterInfo *cephclient.ClusterInfo, newCsiClusterConfigEntry *CSIClusterConfigEntry) error {
	// csi is deployed into the same namespace as the operator
	csiNamespace := os.Getenv(k8sutil.PodNamespaceEnvVar)
	if csiNamespace == "" {
		logger.Warningf("cannot save csi config due to missing env var %q", k8sutil.PodNamespaceEnvVar)
		return nil
	}
	logger.Debugf("using %q for csi configmap namespace", csiNamespace)

	configMutex.Lock()
	defer configMutex.Unlock()

	// fetch current ConfigMap contents
	configMap, err := clientset.CoreV1().ConfigMaps(csiNamespace).Get(clusterInfo.Context, ConfigName, metav1.GetOptions{})
	if err != nil {
		if k8serrors.IsNotFound(err) {
			err = CreateCsiConfigMap(clusterInfo.Context, csiNamespace, clientset, clusterInfo.OwnerInfo)
			if err != nil {
				return errors.Wrap(err, "failed creating csi config map")
			}
		}
		return errors.Wrap(err, "failed to fetch current csi config map")
	}

	// update ConfigMap contents for current cluster
	currData := configMap.Data[ConfigKey]
	if currData == "" {
		currData = "[]"
	}

	newData, err := updateCsiClusterConfig(currData, clusterNamespace, newCsiClusterConfigEntry)
	if err != nil {
		return errors.Wrap(err, "failed to update csi config map data")
	}
	configMap.Data[ConfigKey] = newData

	// update ConfigMap with new contents
	if _, err := clientset.CoreV1().ConfigMaps(csiNamespace).Update(clusterInfo.Context, configMap, metav1.UpdateOptions{}); err != nil {
		return errors.Wrap(err, "failed to update csi config map")
	}

	return nil
}

// updateCSIDriverOptions updates the CSI driver options, including read affinity, kernel mount options
// and fuse mount options, for all entries belonging to the same cluster.
func updateCSIDriverOptions(curr, clusterKey string,
	csiDriverOptions *cephv1.CSIDriverSpec,
) (string, error) {
	cc, err := parseCsiClusterConfig(curr)
	if err != nil {
		return "", errors.Wrap(err, "failed to parse current csi cluster config")
	}

	for i := range cc {
		// If the clusterID belongs to the same cluster, update the entry.
		if clusterKey == cc[i].Namespace {
			cc[i].ReadAffinity = cephcsi.ReadAffinity{
				Enabled:             csiDriverOptions.ReadAffinity.Enabled,
				CrushLocationLabels: csiDriverOptions.ReadAffinity.CrushLocationLabels,
			}
			cc[i].CephFS = cephcsi.CephFS{
				KernelMountOptions: csiDriverOptions.CephFS.KernelMountOptions,
				FuseMountOptions:   csiDriverOptions.CephFS.FuseMountOptions,
			}
		}
	}

	return formatCsiClusterConfig(cc)
}

// SaveCSIDriverOptions, similar to SaveClusterConfig, updates the config map used by ceph-csi
// with CSI driver options such as read affinity, kernel mount options and fuse mount options.
func SaveCSIDriverOptions(clientset kubernetes.Interface, clusterNamespace string, clusterInfo *cephclient.ClusterInfo) error {
	// csi is deployed into the same namespace as the operator
	csiNamespace := os.Getenv(k8sutil.PodNamespaceEnvVar)
	if csiNamespace == "" {
		logger.Warningf("cannot save csi config due to missing env var %q", k8sutil.PodNamespaceEnvVar)
		return nil
	}
	logger.Debugf("using %q for csi configmap namespace", csiNamespace)

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

	newData, err := updateCSIDriverOptions(currData, clusterNamespace, &clusterInfo.CSIDriverSpec)
	if err != nil {
		return errors.Wrap(err, "failed to update csi config map data")
	}
	if currData == newData {
		// no change
		return nil
	}

	// update ConfigMap with new contents
	configMap.Data[ConfigKey] = newData
	if _, err := clientset.CoreV1().ConfigMaps(csiNamespace).Update(clusterInfo.Context, configMap, metav1.UpdateOptions{}); err != nil {
		return errors.Wrap(err, "failed to update csi config map with csi driver options")
	}

	return nil
}
