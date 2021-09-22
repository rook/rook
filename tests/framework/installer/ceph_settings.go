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
	"fmt"
	"os"
	"strings"

	cephv1 "github.com/rook/rook/pkg/apis/ceph.rook.io/v1"
)

// TestCephSettings struct for handling panic and test suite tear down
type TestCephSettings struct {
	DataDirHostPath             string
	ClusterName                 string
	Namespace                   string
	OperatorNamespace           string
	StorageClassName            string
	UseHelm                     bool
	RetainHelmDefaultStorageCRs bool
	UsePVC                      bool
	Mons                        int
	UseCrashPruner              bool
	MultipleMgrs                bool
	SkipOSDCreation             bool
	EnableDiscovery             bool
	EnableAdmissionController   bool
	IsExternal                  bool
	SkipClusterCleanup          bool
	SkipCleanupPolicy           bool
	DirectMountToolbox          bool
	EnableVolumeReplication     bool
	RookVersion                 string
	CephVersion                 cephv1.CephVersionSpec
}

func (s *TestCephSettings) ApplyEnvVars() {
	// skip the cleanup by default
	s.SkipClusterCleanup = true
	if os.Getenv("SKIP_TEST_CLEANUP") == "false" {
		s.SkipClusterCleanup = false
	}
	s.SkipCleanupPolicy = true
	if os.Getenv("SKIP_CLEANUP_POLICY") == "false" {
		s.SkipCleanupPolicy = false
	}
}

func (s *TestCephSettings) readManifest(filename string) string {
	manifest := readManifest("ceph", filename)
	return replaceNamespaces(manifest, manifest, s.OperatorNamespace, s.Namespace)
}

func (s *TestCephSettings) readManifestFromGithub(filename string) string {
	return s.readManifestFromGithubWithClusterNamespace(filename, s.Namespace)
}

func (s *TestCephSettings) readManifestFromGithubWithClusterNamespace(filename, clusterNamespace string) string {
	manifest := readManifestFromGithub(s.RookVersion, "ceph", filename)
	return replaceNamespaces(filename, manifest, s.OperatorNamespace, clusterNamespace)
}

func (s *TestCephSettings) replaceOperatorSettings(manifest string) string {
	manifest = strings.ReplaceAll(manifest, `# CSI_LOG_LEVEL: "0"`, `CSI_LOG_LEVEL: "5"`)
	manifest = strings.ReplaceAll(manifest, `ROOK_ENABLE_DISCOVERY_DAEMON: "false"`, fmt.Sprintf(`ROOK_ENABLE_DISCOVERY_DAEMON: "%t"`, s.EnableDiscovery))
	manifest = strings.ReplaceAll(manifest, `CSI_ENABLE_VOLUME_REPLICATION: "false"`, fmt.Sprintf(`CSI_ENABLE_VOLUME_REPLICATION: "%t"`, s.EnableVolumeReplication))
	return manifest
}

func replaceNamespaces(name, manifest, operatorNamespace, clusterNamespace string) string {
	// RBAC and related namespaces
	manifest = strings.ReplaceAll(manifest, "rook-ceph # namespace:operator", operatorNamespace)
	manifest = strings.ReplaceAll(manifest, "rook-ceph # namespace:cluster", clusterNamespace)
	manifest = strings.ReplaceAll(manifest, "rook-ceph-external # namespace:cluster", clusterNamespace)
	// Double space only needed for v1.5 upgrade test
	manifest = strings.ReplaceAll(manifest, "rook-ceph  # namespace:operator", operatorNamespace)

	// SCC namespaces for operator and Ceph daemons
	manifest = strings.ReplaceAll(manifest, "rook-ceph:rook-ceph-system # serviceaccount:namespace:operator", operatorNamespace+":rook-ceph-system")
	manifest = strings.ReplaceAll(manifest, "rook-ceph:rook-ceph-mgr # serviceaccount:namespace:cluster", clusterNamespace+":rook-ceph-mgr")
	manifest = strings.ReplaceAll(manifest, "rook-ceph:rook-ceph-osd # serviceaccount:namespace:cluster", clusterNamespace+":rook-ceph-osd")

	// SCC namespaces for CSI driver
	manifest = strings.ReplaceAll(manifest, "rook-ceph:rook-csi-rbd-plugin-sa # serviceaccount:namespace:operator", operatorNamespace+":rook-csi-rbd-plugin-sa")
	manifest = strings.ReplaceAll(manifest, "rook-ceph:rook-csi-rbd-provisioner-sa # serviceaccount:namespace:operator", operatorNamespace+":rook-csi-rbd-provisioner-sa")
	manifest = strings.ReplaceAll(manifest, "rook-ceph:rook-csi-cephfs-plugin-sa # serviceaccount:namespace:operator", operatorNamespace+":rook-csi-cephfs-plugin-sa")
	manifest = strings.ReplaceAll(manifest, "rook-ceph:rook-csi-cephfs-provisioner-sa # serviceaccount:namespace:operator", operatorNamespace+":rook-csi-cephfs-provisioner-sa")

	// CSI Drivers
	manifest = strings.ReplaceAll(manifest, "rook-ceph.cephfs.csi.ceph.com # driver:namespace:operator", operatorNamespace+".cephfs.csi.ceph.com")
	manifest = strings.ReplaceAll(manifest, "rook-ceph.rbd.csi.ceph.com # driver:namespace:operator", operatorNamespace+".rbd.csi.ceph.com")

	// Bucket storage class
	manifest = strings.ReplaceAll(manifest, "rook-ceph.ceph.rook.io/bucket # driver:namespace:cluster", clusterNamespace+".ceph.rook.io/bucket")
	if strings.Contains(manifest, "namespace:operator") || strings.Contains(manifest, "namespace:cluster") || strings.Contains(manifest, "driver:namespace:") || strings.Contains(manifest, "serviceaccount:namespace:") {
		logger.Infof("BAD MANIFEST:\n%s", manifest)
		panic(fmt.Sprintf("manifest %s still contains a namespace identifier", name))
	}
	return manifest
}
