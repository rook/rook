/*
Copyright 2021 The Rook Authors. All rights reserved.

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

package installer

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/pkg/errors"
	"github.com/stretchr/testify/assert"
	"gopkg.in/yaml.v2"
	kerrors "k8s.io/apimachinery/pkg/api/errors"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	OperatorChartName    = "rook-ceph"
	CephClusterChartName = "rook-ceph-cluster"

	cephCsiOperatorHelmRepoURL  = "https://ceph.github.io/ceph-csi-operator"
	cephCsiOperatorHelmRepoName = "ceph-csi-operator"
	cephCsiDriversChartName     = "ceph-csi-drivers"
	cephCsiDriversChartVersion  = "0.6.0"
	cephCsiDriversReleaseName   = "ceph-csi-drivers"
)

// The Ceph Storage CustomResource and StorageClass names used in testing
const (
	BlockPoolName     = "ceph-block-test"
	BlockPoolSCName   = "ceph-block-test-sc"
	FilesystemName    = "ceph-filesystem-test"
	FilesystemSCName  = "ceph-filesystem-test-sc"
	ObjectStoreName   = "ceph-objectstore-test"
	ObjectStoreSCName = "ceph-bucket-test-sc"
)

// CreateRookOperatorViaHelm creates rook operator via Helm chart named local/rook present in local repo
func (h *CephInstaller) CreateRookOperatorViaHelm() error {
	return h.configureRookOperatorViaHelm(false)
}

func (h *CephInstaller) UpgradeRookOperatorViaHelm() error {
	return h.configureRookOperatorViaHelm(true)
}

func (h *CephInstaller) configureRookOperatorViaHelm(upgrade bool) error {
	imageValues := map[string]interface{}{"tag": h.settings.RookVersion}
	if h.settings.RookVersion == Version1_18 {
		repo, tag := splitImageRef(PreUpgradeRookImage)
		imageValues = map[string]interface{}{"repository": repo, "tag": tag}
	}
	values := map[string]interface{}{
		"enableDiscoveryDaemon": h.settings.EnableDiscovery,
		"image":                 imageValues,
		"monitoring":            map[string]interface{}{"enabled": true},
		"revisionHistoryLimit":  "3",
		"enforceHostNetwork":    "false",
	}
	// create the operator namespace
	if err := h.k8shelper.CreateNamespace(h.settings.OperatorNamespace); err != nil {
		return errors.Errorf("failed to create namespace %s. %v", h.settings.Namespace, err)
	}

	if upgrade {
		// h.adoptRookOperatorHelmResourcesForUpgrade()
	}

	if h.settings.RookVersion == LocalBuildTag {
		if err := h.helmHelper.InstallLocalHelmChart(upgrade, h.settings.OperatorNamespace, OperatorChartName, values); err != nil {
			return errors.Errorf("failed to install rook operator via helm, err : %v", err)
		}
	} else {
		// Install a specific version of the chart, from which the test will upgrade later
		if err := h.helmHelper.InstallVersionedChart(h.settings.OperatorNamespace, OperatorChartName, h.settings.RookVersion, values); err != nil {
			return errors.Errorf("failed to install rook operator via helm, err : %v", err)
		}
	}

	return nil
}

func (h *CephInstaller) adoptRookOperatorHelmResourcesForUpgrade() {
	ns := h.settings.OperatorNamespace
	h.adoptResourceForHelm("configmap", "rook-csi-operator-image-set-configmap", ns, OperatorChartName, ns)
	rel := cephCsiDriversReleaseName
	h.adoptResourceForHelm("operatorconfigs.csi.ceph.io", "ceph-csi-operator-config", ns, rel, ns)
	h.adoptResourceForHelm("drivers.csi.ceph.io", ns+".rbd.csi.ceph.com", ns, rel, ns)
	h.adoptResourceForHelm("drivers.csi.ceph.io", ns+".cephfs.csi.ceph.com", ns, rel, ns)
	if h.settings.TestNFSCSI {
		h.adoptResourceForHelm("drivers.csi.ceph.io", ns+".nfs.csi.ceph.com", ns, rel, ns)
	}
}

// CreateRookCephClusterViaHelm creates rook cluster via Helm
func (h *CephInstaller) CreateRookCephClusterViaHelm() error {
	return h.configureRookCephClusterViaHelm(false)
}

func (h *CephInstaller) UpgradeRookCephClusterViaHelm() error {
	return h.configureRookCephClusterViaHelm(true)
}

func (h *CephInstaller) configureRookCephClusterViaHelm(upgrade bool) error {
	clusterImage := "rook/ceph:" + h.settings.RookVersion
	if h.settings.RookVersion == Version1_18 {
		clusterImage = PreUpgradeRookImage
	}
	values := map[string]interface{}{
		"image": clusterImage,
	}

	// Set the host path the first time, but use the same path for an upgrade
	if h.settings.DataDirHostPath == "" {
		var err error
		h.settings.DataDirHostPath, err = h.initTestDir(h.settings.Namespace)
		if err != nil {
			return err
		}
	}

	var clusterCRD map[string]interface{}
	if err := yaml.Unmarshal([]byte(h.Manifests.GetCephCluster()), &clusterCRD); err != nil {
		return err
	}
	values["cephClusterSpec"] = clusterCRD["spec"]

	values["operatorNamespace"] = h.settings.OperatorNamespace
	values["configOverride"] = clusterCustomSettings
	values["toolbox"] = map[string]interface{}{
		"enabled":   true,
		"resources": nil,
	}
	values["monitoring"] = map[string]interface{}{
		"enabled":               true,
		"createPrometheusRules": true,
	}
	values["ingress"] = map[string]interface{}{
		"dashboard": map[string]interface{}{
			"annotations": map[string]interface{}{
				"kubernetes.io/ingress-class":                "nginx",
				"nginx.ingress.kubernetes.io/rewrite-target": "/ceph-dashboard/$2",
			},
			"host": map[string]interface{}{
				"name":     "localhost",
				"path":     "/ceph-dashboard(/|$)(.*)",
				"pathType": "ImplementationSpecific",
			},
		},
	}

	if err := h.CreateBlockPoolConfiguration(values, BlockPoolName, BlockPoolSCName); err != nil {
		return err
	}
	if err := h.CreateFileSystemConfiguration(values, FilesystemName, FilesystemSCName); err != nil {
		return err
	}
	if err := h.CreateObjectStoreConfiguration(values, ObjectStoreName, ObjectStoreSCName); err != nil {
		return err
	}

	logger.Infof("Creating ceph cluster using Helm with values: %+v", values)
	if h.settings.RookVersion == LocalBuildTag {
		if err := h.helmHelper.InstallLocalHelmChart(upgrade, h.settings.Namespace, CephClusterChartName, values); err != nil {
			return err
		}
	} else {
		// Install official version of the chart
		if err := h.helmHelper.InstallVersionedChart(h.settings.Namespace, CephClusterChartName, h.settings.RookVersion, values); err != nil {
			return err
		}
	}

	if h.settings.RookVersion == LocalBuildTag && h.settings.UseHelm {
		if err := h.InstallCephCsiDriversViaHelm(); err != nil {
			return errors.Wrap(err, "failed to install ceph-csi-drivers chart")
		}
	}

	return nil
}

func csiDriverChartValues(driverName string) map[string]interface{} {
	return map[string]interface{}{
		"name":           driverName,
		"enabled":        true,
		"snapshotPolicy": "volumeSnapshot",
		"imageSet": map[string]interface{}{
			"name": "rook-csi-operator-image-set-configmap",
		},
		"nodePlugin": map[string]interface{}{
			"kubeletDirPath":         "/var/lib/kubelet",
			"priorityClassName":      "system-node-critical",
			"enableSeLinuxHostMount": false,
		},
		"controllerPlugin": map[string]interface{}{
			"priorityClassName": "system-cluster-critical",
			"replicas":          2,
			"deploymentStrategy": map[string]interface{}{
				"type": "Recreate",
			},
		},
	}
}

func (h *CephInstaller) InstallCephCsiDriversViaHelm() error {
	if err := h.k8shelper.CreateSnapshotCRD("create"); err != nil {
		return errors.Wrap(err, "failed to install snapshot CRDs")
	}
	if err := h.k8shelper.CreateSnapshotController("create"); err != nil {
		return errors.Wrap(err, "failed to install snapshot controller")
	}
	if err := h.k8shelper.WaitForSnapshotController(15); err != nil {
		return errors.Wrap(err, "snapshot controller is not ready")
	}
	op := h.settings.OperatorNamespace

	drivers := map[string]interface{}{
		"rbd":    csiDriverChartValues(fmt.Sprintf("%s.rbd.csi.ceph.com", op)),
		"cephfs": csiDriverChartValues(fmt.Sprintf("%s.cephfs.csi.ceph.com", op)),
		"nvmeof": map[string]interface{}{"enabled": false},
	}
	if h.settings.TestNFSCSI {
		drivers["nfs"] = csiDriverChartValues(fmt.Sprintf("%s.nfs.csi.ceph.com", op))
	} else {
		drivers["nfs"] = map[string]interface{}{"enabled": false}
	}
	values := map[string]interface{}{
		"operatorConfig": map[string]interface{}{
			"name":      "ceph-csi-operator-config",
			"namespace": op,
			"create":    true,
			"driverSpecDefaults": map[string]interface{}{
				"log": map[string]interface{}{"verbosity": 0},
				"imageSet": map[string]interface{}{
					"name": "rook-csi-operator-image-set-configmap",
				},
				"snapshotPolicy":   "volumeSnapshot",
				"enableMetadata":   false,
				"generateOMapInfo": false,
				"fsGroupPolicy":    "File",
				"deployCsiAddons":  false,
				"cephFsClientType": "kernel",
				"nodePlugin": map[string]interface{}{
					"kubeletDirPath":         "/var/lib/kubelet",
					"priorityClassName":      "system-node-critical",
					"enableSeLinuxHostMount": false,
				},
				"controllerPlugin": map[string]interface{}{
					"priorityClassName": "system-cluster-critical",
					"replicas":          2,
					"deploymentStrategy": map[string]interface{}{
						"type": "Recreate",
					},
				},
			},
		},
		"drivers": drivers,
	}
	return h.helmHelper.InstallOrUpgradeHelmRepoChart(
		op,
		cephCsiDriversReleaseName,
		cephCsiOperatorHelmRepoURL,
		cephCsiOperatorHelmRepoName,
		cephCsiDriversChartName,
		cephCsiDriversChartVersion,
		values,
	)
}

// adoptResourceForHelm annotates an existing resource so Helm can adopt it on upgrade.
func (h *CephInstaller) adoptResourceForHelm(resourceType, name, resourceNamespace, releaseName, releaseNamespace string) {
	annotateArgs := []string{
		"annotate", resourceType, name,
		"meta.helm.sh/release-name=" + releaseName,
		"meta.helm.sh/release-namespace=" + releaseNamespace,
		"--overwrite",
	}
	labelArgs := []string{
		"label", resourceType, name,
		"app.kubernetes.io/managed-by=Helm",
		"--overwrite",
	}
	// CRDs are cluster-scoped; omit -n so kubectl does not suggest a namespaced context.
	if resourceType != "crd" {
		annotateArgs = append([]string{"-n", resourceNamespace}, annotateArgs...)
		labelArgs = append([]string{"-n", resourceNamespace}, labelArgs...)
	}
	_, err := h.k8shelper.Kubectl(annotateArgs...)
	if err != nil {
		logger.Warningf("could not annotate %s/%s for helm adoption (may not exist yet): %v", resourceType, name, err)
	}
	_, err = h.k8shelper.Kubectl(labelArgs...)
	if err != nil {
		logger.Warningf("could not label %s/%s for helm adoption (may not exist yet): %v", resourceType, name, err)
	}
}

// removeCephClusterHelmResources tidies up the helm created CRs and Storage Classes, as they interfere with other tests.
func (h *CephInstaller) removeCephClusterHelmResources() {
	if err := h.k8shelper.Clientset.StorageV1().StorageClasses().Delete(context.TODO(), BlockPoolSCName, v1.DeleteOptions{}); err != nil {
		assert.True(h.T(), kerrors.IsNotFound(err))
	}
	if err := h.k8shelper.Clientset.StorageV1().StorageClasses().Delete(context.TODO(), FilesystemSCName, v1.DeleteOptions{}); err != nil {
		assert.True(h.T(), kerrors.IsNotFound(err))
	}
	if err := h.k8shelper.Clientset.StorageV1().StorageClasses().Delete(context.TODO(), ObjectStoreSCName, v1.DeleteOptions{}); err != nil {
		assert.True(h.T(), kerrors.IsNotFound(err))
	}
	if err := h.k8shelper.RookClientset.CephV1().CephBlockPools(h.settings.Namespace).Delete(context.TODO(), BlockPoolName, v1.DeleteOptions{}); err != nil {
		assert.True(h.T(), kerrors.IsNotFound(err))
	}
	if err := h.k8shelper.RookClientset.CephV1().CephFilesystemSubVolumeGroups(h.settings.Namespace).Delete(context.TODO(), FilesystemName+"-csi", v1.DeleteOptions{}); err != nil {
		assert.True(h.T(), kerrors.IsNotFound(err))
	}
	if err := h.k8shelper.RookClientset.CephV1().CephFilesystems(h.settings.Namespace).Delete(context.TODO(), FilesystemName, v1.DeleteOptions{}); err != nil {
		assert.True(h.T(), kerrors.IsNotFound(err))
	}
	if err := h.k8shelper.RookClientset.CephV1().CephObjectStores(h.settings.Namespace).Delete(context.TODO(), ObjectStoreName, v1.DeleteOptions{}); err != nil {
		assert.True(h.T(), kerrors.IsNotFound(err))
	}
	if !h.k8shelper.WaitUntilPodWithLabelDeleted(fmt.Sprintf("rook_object_store=%s", ObjectStoreName), h.settings.Namespace) {
		assert.Fail(h.T(), "rgw did not stop via helm uninstall")
	}
}

// ConfirmHelmClusterInstalledCorrectly runs some validation to check whether the helm chart installed correctly.
func (h *CephInstaller) ConfirmHelmClusterInstalledCorrectly() error {
	storageClassList, err := h.k8shelper.Clientset.StorageV1().StorageClasses().List(context.TODO(), v1.ListOptions{})
	if err != nil {
		return err
	}

	foundStorageClasses := 0
	for _, storageClass := range storageClassList.Items {
		switch storageClass.Name {
		case BlockPoolSCName:
			foundStorageClasses++
		case FilesystemSCName:
			foundStorageClasses++
		case ObjectStoreSCName:
			foundStorageClasses++
		}
	}
	if foundStorageClasses != 3 {
		return fmt.Errorf("did not find the three storage classes which should have been deployed")
	}

	// check that ObjectStore is created
	logger.Infof("Check that RGW pods are Running")
	for i := 0; i < 24 && !h.k8shelper.CheckPodCountAndState("rook-ceph-rgw", h.settings.Namespace, 2, "Running"); i++ {
		logger.Infof("(%d) RGW pod check sleeping for 5 seconds ...", i)
		time.Sleep(5 * time.Second)
	}
	if !h.k8shelper.CheckPodCountAndState("rook-ceph-rgw", h.settings.Namespace, 2, "Running") {
		return fmt.Errorf("did not find the rados gateway pod, which should have been deployed")
	}
	return nil
}

// CreateBlockPoolConfiguration creates a block store configuration
func (h *CephInstaller) CreateBlockPoolConfiguration(values map[string]interface{}, name, scName string) error {
	testBlockPoolBytes := []byte(h.Manifests.GetBlockPool("testPool", "1"))
	var testBlockPoolCRD map[string]interface{}
	if err := yaml.Unmarshal(testBlockPoolBytes, &testBlockPoolCRD); err != nil {
		return err
	}

	storageClassBytes := []byte(h.Manifests.GetBlockStorageClass(name, scName, "Delete"))
	var testBlockSC map[string]interface{}
	if err := yaml.Unmarshal(storageClassBytes, &testBlockSC); err != nil {
		return err
	}

	values["cephBlockPools"] = []map[string]interface{}{
		{
			"name": name,
			"spec": testBlockPoolCRD["spec"],
			"storageClass": map[string]interface{}{
				"enabled":              true,
				"isDefault":            true,
				"name":                 scName,
				"parameters":           testBlockSC["parameters"],
				"reclaimPolicy":        "Delete",
				"allowVolumeExpansion": true,
			},
		},
	}
	return nil
}

// CreateFileSystemConfiguration creates a filesystem configuration
func (h *CephInstaller) CreateFileSystemConfiguration(values map[string]interface{}, name, scName string) error {
	testFilesystemBytes := []byte(h.Manifests.GetFilesystem("testFilesystem", 1))
	var testFilesystemCRD map[string]interface{}
	if err := yaml.Unmarshal(testFilesystemBytes, &testFilesystemCRD); err != nil {
		return err
	}

	storageClassBytes := []byte(h.Manifests.GetFileStorageClass(name, scName))
	var testFileSystemSC map[string]interface{}
	if err := yaml.Unmarshal(storageClassBytes, &testFileSystemSC); err != nil {
		return err
	}

	values["cephFileSystems"] = []map[string]interface{}{
		{
			"name": name,
			"spec": testFilesystemCRD["spec"],
			"storageClass": map[string]interface{}{
				"enabled":       true,
				"name":          scName,
				"parameters":    testFileSystemSC["parameters"],
				"reclaimPolicy": "Delete",
			},
		},
	}
	return nil
}

// CreateObjectStoreConfiguration creates an object store configuration
func (h *CephInstaller) CreateObjectStoreConfiguration(values map[string]interface{}, name, scName string) error {
	testObjectStoreBytes := []byte(h.Manifests.GetObjectStore(name, 2, 8080, false, false))
	var testObjectStoreCRD map[string]interface{}
	if err := yaml.Unmarshal(testObjectStoreBytes, &testObjectStoreCRD); err != nil {
		return err
	}

	storageClassBytes := []byte(h.Manifests.GetBucketStorageClass(name, scName, "Delete"))
	var testObjectStoreSC map[string]interface{}
	if err := yaml.Unmarshal(storageClassBytes, &testObjectStoreSC); err != nil {
		return err
	}

	values["cephObjectStores"] = []map[string]interface{}{
		{
			"name": name,
			"spec": testObjectStoreCRD["spec"],
			"storageClass": map[string]interface{}{
				"enabled":       true,
				"name":          scName,
				"parameters":    testObjectStoreSC["parameters"],
				"reclaimPolicy": "Delete",
			},
		},
	}
	return nil
}

func splitImageRef(ref string) (repository, tag string) {
	i := strings.LastIndex(ref, ":")
	if i < 0 {
		return ref, "latest"
	}
	after := ref[i+1:]
	if strings.Contains(after, "/") {
		return ref, "latest"
	}
	return ref[:i], after
}
