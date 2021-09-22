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
	"time"

	"github.com/pkg/errors"
	"gopkg.in/yaml.v2"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	OperatorChartName    = "rook-ceph"
	CephClusterChartName = "rook-ceph-cluster"
)

// The Ceph Storage CustomResource and StorageClass names used in testing
const (
	blockPoolName     = "ceph-block-test"
	blockPoolSCName   = "ceph-block-test-sc"
	filesystemName    = "ceph-filesystem-test"
	filesystemSCName  = "ceph-filesystem-test-sc"
	objectStoreName   = "ceph-objectstore-test"
	objectStoreSCName = "ceph-bucket-test-sc"
)

// CreateRookOperatorViaHelm creates rook operator via Helm chart named local/rook present in local repo
func (h *CephInstaller) CreateRookOperatorViaHelm(values map[string]interface{}) error {
	// create the operator namespace before the admission controller is created
	if err := h.k8shelper.CreateNamespace(h.settings.OperatorNamespace); err != nil {
		return errors.Errorf("failed to create namespace %s. %v", h.settings.Namespace, err)
	}
	if err := h.k8shelper.CreateOpConfigMap(h.settings.OperatorNamespace); err != nil {
		return errors.Errorf("failed to create operator config map. %v", err)
	}
	if err := h.startAdmissionController(); err != nil {
		return errors.Errorf("failed to start admission controllers. %v", err)
	}
	if err := h.helmHelper.InstallLocalRookHelmChart(h.settings.OperatorNamespace, OperatorChartName, values); err != nil {
		return errors.Errorf("failed to install rook operator via helm, err : %v", err)
	}

	return nil
}

// CreateRookCephClusterViaHelm creates rook cluster via Helm
func (h *CephInstaller) CreateRookCephClusterViaHelm(values map[string]interface{}) error {
	var err error
	h.settings.DataDirHostPath, err = h.initTestDir(h.settings.Namespace)
	if err != nil {
		return err
	}

	var clusterCRD map[string]interface{}
	if err := yaml.Unmarshal([]byte(h.Manifests.GetCephCluster()), &clusterCRD); err != nil {
		return err
	}

	values["operatorNamespace"] = h.settings.OperatorNamespace
	values["configOverride"] = clusterCustomSettings
	values["toolbox"] = map[string]interface{}{
		"enabled": true,
		"image":   "rook/ceph:" + LocalBuildTag,
	}
	values["cephClusterSpec"] = clusterCRD["spec"]

	if err := h.CreateBlockPoolConfiguration(values, blockPoolName, blockPoolSCName); err != nil {
		return err
	}
	if err := h.CreateFileSystemConfiguration(values, filesystemName, filesystemSCName); err != nil {
		return err
	}
	if err := h.CreateObjectStoreConfiguration(values, objectStoreName, objectStoreSCName); err != nil {
		return err
	}

	logger.Infof("Creating ceph cluster using Helm with values: %+v", values)
	if err := h.helmHelper.InstallLocalRookHelmChart(h.settings.Namespace, CephClusterChartName, values); err != nil {
		return err
	}

	return nil
}

// RemoveRookCephClusterHelmDefaultCustomResources tidies up the helm created CRs and Storage Classes, as they interfere with other tests.
func (h *CephInstaller) RemoveRookCephClusterHelmDefaultCustomResources() error {
	if err := h.k8shelper.Clientset.StorageV1().StorageClasses().Delete(context.TODO(), blockPoolSCName, v1.DeleteOptions{}); err != nil {
		return err
	}
	if err := h.k8shelper.Clientset.StorageV1().StorageClasses().Delete(context.TODO(), filesystemSCName, v1.DeleteOptions{}); err != nil {
		return err
	}
	if err := h.k8shelper.Clientset.StorageV1().StorageClasses().Delete(context.TODO(), objectStoreSCName, v1.DeleteOptions{}); err != nil {
		return err
	}
	if err := h.k8shelper.RookClientset.CephV1().CephBlockPools(h.settings.Namespace).Delete(context.TODO(), blockPoolName, v1.DeleteOptions{}); err != nil {
		return err
	}
	if err := h.k8shelper.RookClientset.CephV1().CephFilesystems(h.settings.Namespace).Delete(context.TODO(), filesystemName, v1.DeleteOptions{}); err != nil {
		return err
	}
	if err := h.k8shelper.RookClientset.CephV1().CephObjectStores(h.settings.Namespace).Delete(context.TODO(), objectStoreName, v1.DeleteOptions{}); err != nil {
		return err
	}
	if !h.k8shelper.WaitUntilPodWithLabelDeleted(fmt.Sprintf("rook_object_store=%s", objectStoreName), h.settings.Namespace) {
		return fmt.Errorf("rgw did not stop via crd")
	}
	return nil
}

// ConfirmHelmClusterInstalledCorrectly runs some validation to check whether the helm chart installed correctly.
func (h *CephInstaller) ConfirmHelmClusterInstalledCorrectly() error {
	storageClassList, err := h.k8shelper.Clientset.StorageV1().StorageClasses().List(context.TODO(), v1.ListOptions{})
	if err != nil {
		return err
	}

	foundStorageClasses := 0
	for _, storageClass := range storageClassList.Items {
		if storageClass.Name == blockPoolSCName {
			foundStorageClasses++
		} else if storageClass.Name == filesystemSCName {
			foundStorageClasses++
		} else if storageClass.Name == objectStoreSCName {
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
	testObjectStoreBytes := []byte(h.Manifests.GetObjectStore(name, 2, 8080, false))
	var testObjectStoreCRD map[string]interface{}
	if err := yaml.Unmarshal(testObjectStoreBytes, &testObjectStoreCRD); err != nil {
		return err
	}

	storageClassBytes := []byte(h.Manifests.GetBucketStorageClass(name, scName, "Delete", "us-east-1"))
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
