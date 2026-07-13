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
	cephCsiDriversChartVersion  = "1.0.4"
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
	values := map[string]interface{}{
		"enableDiscoveryDaemon": h.settings.EnableDiscovery,
		"image":                 map[string]interface{}{"tag": h.settings.RookVersion},
		"monitoring":            map[string]interface{}{"enabled": true},
		"revisionHistoryLimit":  "3",
		"enforceHostNetwork":    "false",
	}
	// create the operator namespace
	if err := h.k8shelper.CreateNamespace(h.settings.OperatorNamespace); err != nil {
		return errors.Errorf("failed to create namespace %s. %v", h.settings.Namespace, err)
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

// CreateRookCephClusterViaHelm creates rook cluster via Helm
func (h *CephInstaller) CreateRookCephClusterViaHelm() error {
	return h.configureRookCephClusterViaHelm(false)
}

func (h *CephInstaller) UpgradeRookCephClusterViaHelm() error {
	return h.configureRookCephClusterViaHelm(true)
}

func (h *CephInstaller) configureRookCephClusterViaHelm(upgrade bool) error {
	values := map[string]interface{}{
		"image": "rook/ceph:" + h.settings.RookVersion,
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
	// the snapshot controller is installed while the cluster is busy rolling daemons during
	// upgrade tests, and its image pull plus rollout regularly exceeds 150s in CI
	if err := h.k8shelper.WaitForSnapshotController(90); err != nil {
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

	if h.settings.UseMultisiteObjectStore {
		h.removeMultisiteHelmResources(ObjectStoreName)
	}
}

// removeMultisiteHelmResources tears down the CephObjectRealm, CephObjectZoneGroup,
// CephObjectZone, and shared rgw pools that CreateObjectStoreConfiguration added
// to the chart. They are not part of the chart's default storage CRs, so they
// must be removed explicitly or they orphan and trip the teardown pool check.
// Order matters: the zone's deletion finalizer runs radosgw-admin against realm
// metadata stored in the .rgw.root pool, so the store (already deleted above),
// zone, zone group, and realm must all be gone before their pools are removed.
func (h *CephInstaller) removeMultisiteHelmResources(name string) {
	ctx := context.TODO()
	ceph := h.k8shelper.RookClientset.CephV1()
	ns := h.settings.Namespace

	deleteAndWait := func(kind, crName string, del func() error, get func() error) {
		if err := del(); err != nil {
			assert.True(h.T(), kerrors.IsNotFound(err), "unexpected error deleting %s %q", kind, crName)
			return
		}
		if err := h.k8shelper.WaitForCustomResourceDeletion(ns, crName, get); err != nil {
			assert.NoError(h.T(), err, "failed waiting for %s %q deletion", kind, crName)
		}
	}

	deleteAndWait("CephObjectZone", name,
		func() error { return ceph.CephObjectZones(ns).Delete(ctx, name, v1.DeleteOptions{}) },
		func() error { _, err := ceph.CephObjectZones(ns).Get(ctx, name, v1.GetOptions{}); return err })
	deleteAndWait("CephObjectZoneGroup", name,
		func() error { return ceph.CephObjectZoneGroups(ns).Delete(ctx, name, v1.DeleteOptions{}) },
		func() error { _, err := ceph.CephObjectZoneGroups(ns).Get(ctx, name, v1.GetOptions{}); return err })
	deleteAndWait("CephObjectRealm", name,
		func() error { return ceph.CephObjectRealms(ns).Delete(ctx, name, v1.DeleteOptions{}) },
		func() error { _, err := ceph.CephObjectRealms(ns).Get(ctx, name, v1.GetOptions{}); return err })

	for poolName := range rgwSharedPools(name) {
		deleteAndWait("CephBlockPool", poolName,
			func() error { return ceph.CephBlockPools(ns).Delete(ctx, poolName, v1.DeleteOptions{}) },
			func() error { _, err := ceph.CephBlockPools(ns).Get(ctx, poolName, v1.GetOptions{}); return err })
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

// rgwSharedPools maps the CephBlockPool CR name to its rados pool name (empty
// means the operator derives the pool name from the CR name) for the multisite
// object store configured by CreateObjectStoreConfiguration. .rgw.root is the
// realm-global metadata pool; the operator sets pg_num_min=8 on it, so it must
// be created with pg_num>=8 or the zone reconcile fails.
func rgwSharedPools(name string) map[string]string {
	return map[string]string{
		"rgw.root":                     ".rgw.root",
		name + ".rgw.control":          "",
		name + ".rgw.meta":             "",
		name + ".rgw.log":              "",
		name + ".rgw.otp":              "",
		name + ".rgw.buckets.index":    "",
		name + ".rgw.buckets.data":     "",
		name + ".rgw.buckets.data.foo": "",
	}
}

// CreateObjectStoreConfiguration adds the chart's object store and its bucket
// storage class to the Helm values. When UseMultisiteObjectStore is set it
// configures an RGW multisite zone; otherwise it configures a plain store.
func (h *CephInstaller) CreateObjectStoreConfiguration(values map[string]interface{}, name, scName string) error {
	if h.settings.UseMultisiteObjectStore {
		return h.createMultisiteObjectStoreConfiguration(values, name, scName)
	}

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

// createMultisiteObjectStoreConfiguration configures the object store as an RGW
// multisite zone, exercising the chart's CephObjectRealm, CephObjectZoneGroup,
// and CephObjectZone templates on install. The realm/zone group/zone/shared-pools
// layout mirrors the fixture in tests/integration/object/util/sharedstore.
func (h *CephInstaller) createMultisiteObjectStoreConfiguration(values map[string]interface{}, name, scName string) error {
	poolNames := rgwSharedPools(name)
	rgwPools := make([]map[string]interface{}, 0, len(poolNames))
	for poolName, radosName := range poolNames {
		pgNum := "1"
		if radosName == ".rgw.root" {
			pgNum = "8"
		}
		spec := map[string]interface{}{
			"replicated": map[string]interface{}{
				"size":                   1,
				"requireSafeReplicaSize": false,
			},
			"parameters": map[string]interface{}{
				"pg_autoscale_mode": "off",
				"pg_num":            pgNum,
			},
		}
		// spec.name is enum-validated (.rgw.root/.nfs/.mgr) and must be omitted
		// rather than set to "" when the pool name matches the CR name.
		if radosName != "" {
			spec["name"] = radosName
		}
		rgwPools = append(rgwPools, map[string]interface{}{
			"name": poolName,
			"spec": spec,
			"storageClass": map[string]interface{}{
				"enabled": false,
			},
		})
	}

	// Append the rgw pools to the block pools already configured for CSI so both
	// sets land in the chart's cephBlockPools list.
	existingPools, _ := values["cephBlockPools"].([]map[string]interface{})
	values["cephBlockPools"] = append(existingPools, rgwPools...)

	values["cephObjectRealms"] = []map[string]interface{}{
		{
			"name": name,
		},
	}
	values["cephObjectZoneGroups"] = []map[string]interface{}{
		{
			"name": name,
			"spec": map[string]interface{}{
				"realm": name,
			},
		},
	}
	values["cephObjectZones"] = []map[string]interface{}{
		{
			"name": name,
			"spec": map[string]interface{}{
				"zoneGroup": name,
				"sharedPools": map[string]interface{}{
					"poolPlacements": []map[string]interface{}{
						{
							"name":             "default",
							"default":          true,
							"metadataPoolName": name + ".rgw.buckets.index",
							"dataPoolName":     name + ".rgw.buckets.data",
							"storageClasses": []map[string]interface{}{
								{
									"name":         "FOO",
									"dataPoolName": name + ".rgw.buckets.data.foo",
								},
							},
						},
					},
				},
			},
		},
	}

	storageClassBytes := []byte(h.Manifests.GetBucketStorageClass(name, scName, "Delete"))
	var testObjectStoreSC map[string]interface{}
	if err := yaml.Unmarshal(storageClassBytes, &testObjectStoreSC); err != nil {
		return err
	}

	values["cephObjectStores"] = []map[string]interface{}{
		{
			"name": name,
			"spec": map[string]interface{}{
				"zone": map[string]interface{}{
					"name": name,
				},
				"gateway": map[string]interface{}{
					"port":      8080,
					"instances": 2,
					"resources": nil,
				},
			},
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
