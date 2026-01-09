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

// updateNetNamespaceFilePath modify the netNamespaceFilePath for all cluster IDs.
// If holderEnabled is set to true. Otherwise, removes the netNamespaceFilePath value
// for all the clusterIDs.
func updateNetNamespaceFilePath(clusterNamespace string, cc csiClusterConfig) {
	var (
		cephFSNetNamespaceFilePath string
		rbdNetNamespaceFilePath    string
		nfsNetNamespaceFilePath    string
	)

	if IsHolderEnabled() {
		for _, centry := range cc {
			if centry.Namespace == clusterNamespace && centry.ClusterID == clusterNamespace {
				if centry.CephFS.NetNamespaceFilePath != "" {
					cephFSNetNamespaceFilePath = centry.CephFS.NetNamespaceFilePath
				}
				if centry.RBD.NetNamespaceFilePath != "" {
					rbdNetNamespaceFilePath = centry.RBD.NetNamespaceFilePath
				}
				if centry.NFS.NetNamespaceFilePath != "" {
					nfsNetNamespaceFilePath = centry.NFS.NetNamespaceFilePath
				}
			}
		}

		for i, centry := range cc {
			if centry.Namespace == clusterNamespace {
				cc[i].CephFS.NetNamespaceFilePath = cephFSNetNamespaceFilePath
				cc[i].RBD.NetNamespaceFilePath = rbdNetNamespaceFilePath
				cc[i].NFS.NetNamespaceFilePath = nfsNetNamespaceFilePath
			}
		}
	} else {
		for i := range cc {
			if cc[i].Namespace == clusterNamespace {
				cc[i].CephFS.NetNamespaceFilePath = ""
				cc[i].RBD.NetNamespaceFilePath = ""
				cc[i].NFS.NetNamespaceFilePath = ""
			}
		}
	}
}

// updateCsiClusterConfig returns a json-formatted string containing
// the cluster-to-mon mapping required to configure ceph csi.
func updateCsiClusterConfig(curr, clusterID string, clusterInfo *cephclient.ClusterInfo, newCsiClusterConfigEntry *CSIClusterConfigEntry) (string, error) {
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
		// Disable read affinity if needed for the ceph version
		if !ReadAffinityEnabled(newCsiClusterConfigEntry.ReadAffinity.Enabled, clusterInfo.CephVersion) {
			newCsiClusterConfigEntry.ReadAffinity = cephcsi.ReadAffinity{Enabled: false}
		}

		for i, centry := range cc {
			// there is a bug with an unknown source where the csi config can have an empty
			// namespace entry. detect and fix this scenario if it occurs
			if centry.Namespace == "" && centry.ClusterID == clusterID {
				logger.Infof("correcting CSI config map entry for cluster ID %q; empty namespace will be set to %q", clusterID, clusterInfo.Namespace)
				centry.Namespace = clusterInfo.Namespace
				cc[i] = centry
			}

			// If the clusterID belongs to the same cluster, update the entry.
			// update default clusterID's entry
			if clusterID == centry.Namespace {
				centry.Monitors = newCsiClusterConfigEntry.Monitors
				centry.ReadAffinity = newCsiClusterConfigEntry.ReadAffinity
				centry.CephFS.KernelMountOptions = newCsiClusterConfigEntry.CephFS.KernelMountOptions
				centry.CephFS.FuseMountOptions = newCsiClusterConfigEntry.CephFS.FuseMountOptions
				cc[i] = centry
			}
		}
	}
	for i, centry := range cc {
		if centry.ClusterID == clusterID {
			// If the new entry is nil, this means the entry is being deleted so remove it from the list
			if newCsiClusterConfigEntry == nil {
				cc = append(cc[:i], cc[i+1:]...)
				found = true
				break
			}
			centry.Monitors = newCsiClusterConfigEntry.Monitors
			// update subvolumegroup and cephfs netNamespaceFilePath only when either is specified
			// while always updating kernel and fuse mount options.
			if newCsiClusterConfigEntry.CephFS.SubvolumeGroup != "" || newCsiClusterConfigEntry.CephFS.NetNamespaceFilePath != "" {
				centry.CephFS = newCsiClusterConfigEntry.CephFS
			} else {
				centry.CephFS.KernelMountOptions = newCsiClusterConfigEntry.CephFS.KernelMountOptions
				centry.CephFS.FuseMountOptions = newCsiClusterConfigEntry.CephFS.FuseMountOptions
			}
			// update nfs netNamespaceFilePath only when specified.
			if newCsiClusterConfigEntry.NFS.NetNamespaceFilePath != "" {
				centry.NFS = newCsiClusterConfigEntry.NFS
			}
			// update radosNamespace and rbd netNamespaceFilePath only when either is specified.
			if newCsiClusterConfigEntry.RBD.RadosNamespace != "" || newCsiClusterConfigEntry.RBD.NetNamespaceFilePath != "" {
				centry.RBD = newCsiClusterConfigEntry.RBD
			}
			if len(newCsiClusterConfigEntry.ReadAffinity.CrushLocationLabels) != 0 {
				centry.ReadAffinity = newCsiClusterConfigEntry.ReadAffinity
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
			centry.ClusterID = clusterID
			centry.Namespace = clusterInfo.Namespace
			centry.Monitors = newCsiClusterConfigEntry.Monitors
			centry.RBD = newCsiClusterConfigEntry.RBD
			centry.CephFS = newCsiClusterConfigEntry.CephFS
			centry.NFS = newCsiClusterConfigEntry.NFS
			if len(newCsiClusterConfigEntry.ReadAffinity.CrushLocationLabels) != 0 {
				centry.ReadAffinity = newCsiClusterConfigEntry.ReadAffinity
			}
			cc = append(cc, centry)
		}
	}

	updateNetNamespaceFilePath(clusterID, cc)
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
		// CM already exists; update owner refs to it if needed
		// this corrects issues where the csi config map was sometimes created with CephCluster
		// owner ref, which would result in the cm being deleted if that cluster was deleted
		if err := updateCsiConfigMapOwnerRefs(ctx, namespace, clientset, ownerInfo); err != nil {
			return errors.Wrapf(err, "failed to ensure csi config map %q (in %q) owner references", configMap.Name, namespace)
		}
	} else {
		logger.Infof("successfully created csi config map %q", configMap.Name)
	}

	return nil
}

// check the owner references on the csi config map, and fix incorrect references if needed
func updateCsiConfigMapOwnerRefs(ctx context.Context, namespace string, clientset kubernetes.Interface, expectedOwnerInfo *k8sutil.OwnerInfo) error {
	cm, err := clientset.CoreV1().ConfigMaps(namespace).Get(ctx, ConfigName, metav1.GetOptions{})
	if err != nil {
		return errors.Wrapf(err, "failed to fetch csi config map %q (in %q) which already exists", ConfigName, namespace)
	}

	existingOwners := cm.GetOwnerReferences()
	var currentOwner *metav1.OwnerReference = nil
	if len(existingOwners) == 1 {
		currentOwner = &existingOwners[0] // currentOwner is nil unless there is exactly one owner on the cm
	}
	// if there is exactly one owner, and it is correct --> no fix needed
	if currentOwner != nil && (currentOwner.UID == expectedOwnerInfo.GetUID()) {
		logger.Debugf("csi config map %q (in %q) has the expected owner; owner id: %q", ConfigName, namespace, currentOwner.UID)
		return nil
	}

	// must fix owner refs
	logger.Infof("updating csi configmap %q (in %q) owner info", ConfigName, namespace)
	cm.OwnerReferences = []metav1.OwnerReference{}
	if err := expectedOwnerInfo.SetControllerReference(cm); err != nil {
		return errors.Wrapf(err, "failed to set updated owner reference on csi config map %q (in %q)", ConfigName, namespace)
	}
	_, err = clientset.CoreV1().ConfigMaps(namespace).Update(ctx, cm, metav1.UpdateOptions{})
	if err != nil {
		return errors.Wrapf(err, "failed to update csi config map %q (in %q) to update its owner reference", ConfigName, namespace)
	}

	return nil
}

// SaveClusterConfig updates the config map used to provide ceph-csi with
// basic cluster configuration. The clusterID, clusterNamespace, and clusterInfo are
// used to determine what "cluster" in the config map will be updated. clusterID should be the same
// as clusterNamespace for CephClusters, but for other resources (e.g., CephBlockPoolRadosNamespace,
// CephFilesystemSubVolumeGroup) or for other supplementary entries, the clusterID should be unique
// and different from the namespace so as not to disrupt CephCluster configurations.
func SaveClusterConfig(clientset kubernetes.Interface, clusterID, clusterNamespace string, clusterInfo *cephclient.ClusterInfo, newCsiClusterConfigEntry *CSIClusterConfigEntry) error {
	if EnableCSIOperator() {
		logger.Debugf("csi-operator is enabled no need to save/update csi config in configmap %q", configName)
		return nil
	}
	// csi is deployed into the same namespace as the operator
	csiNamespace := os.Getenv(k8sutil.PodNamespaceEnvVar)
	if csiNamespace == "" {
		logger.Warningf("cannot save csi config due to missing env var %q", k8sutil.PodNamespaceEnvVar)
		return nil
	}
	logger.Debugf("using %q for csi configmap namespace", csiNamespace)

	if newCsiClusterConfigEntry != nil {
		// set CSIDriverOptions
		newCsiClusterConfigEntry.ReadAffinity.Enabled = ReadAffinityEnabled(clusterInfo.CSIDriverSpec.ReadAffinity.Enabled, clusterInfo.CephVersion)
		newCsiClusterConfigEntry.ReadAffinity.CrushLocationLabels = clusterInfo.CSIDriverSpec.ReadAffinity.CrushLocationLabels

		newCsiClusterConfigEntry.CephFS.KernelMountOptions = clusterInfo.CSIDriverSpec.CephFS.KernelMountOptions
		newCsiClusterConfigEntry.CephFS.FuseMountOptions = clusterInfo.CSIDriverSpec.CephFS.FuseMountOptions
	}

	configMutex.Lock()
	defer configMutex.Unlock()

	// fetch current ConfigMap contents
	configMap, err := clientset.CoreV1().ConfigMaps(csiNamespace).Get(clusterInfo.Context, ConfigName, metav1.GetOptions{})
	if err != nil {
		if k8serrors.IsNotFound(err) {
			return errors.Wrap(err, "waiting for CSI config map to be created")
		}
		return errors.Wrap(err, "failed to fetch current csi config map")
	}

	// update ConfigMap contents for current cluster
	currData := configMap.Data[ConfigKey]
	if currData == "" {
		currData = "[]"
	}

	newData, err := updateCsiClusterConfig(currData, clusterID, clusterInfo, newCsiClusterConfigEntry)
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
func updateCSIDriverOptions(curr string, clusterInfo *cephclient.ClusterInfo,
	csiDriverOptions *cephv1.CSIDriverSpec,
) (string, error) {
	cc, err := parseCsiClusterConfig(curr)
	if err != nil {
		return "", errors.Wrap(err, "failed to parse current csi cluster config")
	}

	for i := range cc {
		// If the clusterID belongs to the same cluster, update the entry.
		if clusterInfo.Namespace == cc[i].Namespace {
			cc[i].ReadAffinity.Enabled = ReadAffinityEnabled(csiDriverOptions.ReadAffinity.Enabled, clusterInfo.CephVersion)
			cc[i].ReadAffinity.CrushLocationLabels = csiDriverOptions.ReadAffinity.CrushLocationLabels

			cc[i].CephFS.KernelMountOptions = csiDriverOptions.CephFS.KernelMountOptions
			cc[i].CephFS.FuseMountOptions = csiDriverOptions.CephFS.FuseMountOptions
		}
	}

	updateNetNamespaceFilePath(clusterInfo.Namespace, cc)
	return formatCsiClusterConfig(cc)
}

// SaveCSIDriverOptions, similar to SaveClusterConfig, updates the config map used by ceph-csi
// with CSI driver options such as read affinity, kernel mount options and fuse mount options.
func SaveCSIDriverOptions(clientset kubernetes.Interface, clusterNamespace string, clusterInfo *cephclient.ClusterInfo) error {
	if EnableCSIOperator() {
		logger.Debugf("csi-operator is enabled no need to save/update csi config in configmap %q", configName)
		return nil
	}

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

	newData, err := updateCSIDriverOptions(currData, clusterInfo, &clusterInfo.CSIDriverSpec)
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
