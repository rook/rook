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

package integration

import (
	"testing"

	"github.com/rook/rook/tests/framework/clients"
	"github.com/rook/rook/tests/framework/installer"
	"github.com/rook/rook/tests/framework/utils"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	csiRBDNodeSecret           = "rook-csi-rbd-node"
	csiRBDProvisionerSecret    = "rook-csi-rbd-provisioner"
	csiCephFSNodeSecret        = "rook-csi-cephfs-node"
	csiCephFSProvisionerSecret = "rook-csi-cephfs-provisioner"
	csiSCRBD                   = "ceph-csi-rbd"
	csiSCCephFS                = "ceph-csi-cephfs"
	csiPoolRBD                 = "csi-rbd"
	csiPoolCephFS              = "csi-cephfs"
	csiTestRBDPodName          = "csi-test-rbd"
	csiTestCephFSPodName       = "csi-test-cephfs"
)

func runCephCSIE2ETest(helper *clients.TestClient, k8sh *utils.K8sHelper, s suite.Suite, t *testing.T, namespace string) {

	if !k8sh.VersionAtLeast("v1.13.0") {
		logger.Info("Skipping csi tests as kube version is less than 1.13.0")
		t.Skip()
	}

	logger.Info("test Ceph CSI driver")
	createCephPools(helper, s, namespace)
	createCSIStorageClass(k8sh, s, namespace)
	createAndDeleteCSIRBDTestPod(k8sh, s, namespace)
	createAndDeleteCSICephFSTestPod(k8sh, s, namespace)

	//cleanup resources created
	deleteCephPools(helper, namespace)
	deleteCSIStorageClass(k8sh, namespace)
}

func createCephPools(helper *clients.TestClient, s suite.Suite, namespace string) {
	err := helper.PoolClient.Create(csiPoolRBD, namespace, 1)
	require.Nil(s.T(), err)

	err = helper.FSClient.Create(csiPoolCephFS, namespace)
	require.Nil(s.T(), err)
}

func deleteCephPools(helper *clients.TestClient, namespace string) {
	err := helper.PoolClient.Delete(csiPoolRBD, namespace)
	if err != nil {
		logger.Errorf("failed to delete rbd pool %s with error %v", csiPoolRBD, err)
	}

	err = helper.FSClient.Delete(csiPoolCephFS, namespace)
	if err != nil {
		logger.Errorf("failed to delete cephfs pool %s with error %v", csiPoolCephFS, err)
		return
	}
	logger.Info("Deleted Ceph Pools")
}

func createCSIStorageClass(k8sh *utils.K8sHelper, s suite.Suite, namespace string) {
	rbdSC := `
apiVersion: storage.k8s.io/v1
kind: StorageClass
metadata:
   name: ` + csiSCRBD + `
provisioner: ` + installer.SystemNamespace(namespace) + `.rbd.csi.ceph.com
parameters:
    pool: ` + csiPoolRBD + `
    clusterID: ` + namespace + `
    csi.storage.k8s.io/provisioner-secret-name: ` + csiRBDProvisionerSecret + `
    csi.storage.k8s.io/provisioner-secret-namespace: ` + namespace + `
    csi.storage.k8s.io/node-stage-secret-name: ` + csiRBDNodeSecret + `
    csi.storage.k8s.io/node-stage-secret-namespace: ` + namespace + `
`
	cephFSSC := `
apiVersion: storage.k8s.io/v1
kind: StorageClass
metadata:
   name: ` + csiSCCephFS + `
provisioner: ` + installer.SystemNamespace(namespace) + `.cephfs.csi.ceph.com
parameters:
    clusterID: ` + namespace + `
    fsName: ` + csiPoolCephFS + `
    pool: ` + csiPoolCephFS + `-data0
    csi.storage.k8s.io/provisioner-secret-name: ` + csiCephFSProvisionerSecret + `
    csi.storage.k8s.io/provisioner-secret-namespace: ` + namespace + `
    csi.storage.k8s.io/node-stage-secret-name: ` + csiCephFSNodeSecret + `
    csi.storage.k8s.io/node-stage-secret-namespace: ` + namespace + `
`
	err := k8sh.ResourceOperation("apply", rbdSC)
	require.Nil(s.T(), err)

	err = k8sh.ResourceOperation("apply", cephFSSC)
	require.Nil(s.T(), err)
}

func deleteCSIStorageClass(k8sh *utils.K8sHelper, namespace string) {
	err := k8sh.Clientset.StorageV1().StorageClasses().Delete(csiSCRBD, &metav1.DeleteOptions{})
	if err != nil {
		logger.Errorf("failed to delete rbd storage class %s with error %v", csiSCRBD, err)
	}
	err = k8sh.Clientset.StorageV1().StorageClasses().Delete(csiSCCephFS, &metav1.DeleteOptions{})
	if err != nil {
		logger.Errorf("failed to delete cephfs storage class %s with error %v", csiSCCephFS, err)
		return
	}
	logger.Info("Deleted rbd and cephfs storageclass")
}

func createAndDeleteCSIRBDTestPod(k8sh *utils.K8sHelper, s suite.Suite, namespace string) {
	pod := `
apiVersion: v1
kind: PersistentVolumeClaim
metadata:
  name: rbd-pvc-csi
  namespace: ` + namespace + `
spec:
  accessModes:
  - ReadWriteOnce
  resources:
    requests:
      storage: 1Gi
  storageClassName: ` + csiSCRBD + `
---
apiVersion: v1
kind: Pod
metadata:
  name: ` + csiTestRBDPodName + `
  namespace: ` + namespace + `
spec:
  containers:
  - name: ` + csiTestRBDPodName + `
    image: busybox
    command:
        - sh
        - "-c"
        - "touch /test/csi.test && sleep 3600"
    imagePullPolicy: IfNotPresent
    env:
    volumeMounts:
    - mountPath: /test
      name: csivol
  volumes:
  - name: csivol
    persistentVolumeClaim:
       claimName: rbd-pvc-csi
       readOnly: false
  restartPolicy: Never
`
	err := k8sh.ResourceOperation("apply", pod)
	require.Nil(s.T(), err)
	isPodRunning := k8sh.IsPodRunning(csiTestRBDPodName, namespace)
	if !isPodRunning {
		k8sh.PrintPodDescribe(namespace, csiTestRBDPodName)
		k8sh.PrintPodStatus(namespace)
	}
	// cleanup the pod and pvc
	err = k8sh.ResourceOperation("delete", pod)
	assert.NoError(s.T(), err)
	assert.True(s.T(), isPodRunning, "csi rbd test pod fails to run")
}

func createAndDeleteCSICephFSTestPod(k8sh *utils.K8sHelper, s suite.Suite, namespace string) {
	pod := `
apiVersion: v1
kind: PersistentVolumeClaim
metadata:
  name: cephfs-pvc-csi
  namespace: ` + namespace + `
spec:
  accessModes:
  - ReadWriteOnce
  resources:
    requests:
      storage: 1Gi
  storageClassName: ` + csiSCCephFS + `
---
apiVersion: v1
kind: Pod
metadata:
  name: ` + csiTestCephFSPodName + `
  namespace: ` + namespace + `
spec:
  containers:
  - name: ` + csiTestCephFSPodName + `
    image: busybox
    command:
        - sh
        - "-c"
        - "touch /test/csi.test && sleep 3600"
    imagePullPolicy: IfNotPresent
    env:
    volumeMounts:
    - mountPath: /test
      name: csivol
  volumes:
  - name: csivol
    persistentVolumeClaim:
       claimName: cephfs-pvc-csi
       readOnly: false
  restartPolicy: Never
`
	err := k8sh.ResourceOperation("apply", pod)
	require.Nil(s.T(), err)
	isPodRunning := k8sh.IsPodRunning(csiTestCephFSPodName, namespace)
	if !isPodRunning {
		k8sh.PrintPodDescribe(namespace, csiTestCephFSPodName)
		k8sh.PrintPodStatus(namespace)
	}
	// cleanup the pod and pvc
	err = k8sh.ResourceOperation("delete", pod)
	assert.NoError(s.T(), err)
	assert.True(s.T(), isPodRunning, "csi cephfs test pod fails to run")
}
