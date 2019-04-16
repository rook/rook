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
	"encoding/json"
	"strings"

	monclient "github.com/rook/rook/pkg/daemon/ceph/client"
	"github.com/rook/rook/tests/framework/clients"
	"github.com/rook/rook/tests/framework/installer"
	"github.com/rook/rook/tests/framework/utils"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	csiSecretName        = "ceph-csi-secret"
	csiSCRBD             = "ceph-csi-rbd"
	csiSCCephFS          = "ceph-csi-cephfs"
	csiPoolRBD           = "csi-rbd"
	csiPoolCephFS        = "csi-cephfs"
	csiTestRBDPodName    = "csi-test-rbd"
	csiTestCephFSPodName = "csi-test-cephfs"
)

func runCephCSIE2ETest(helper *clients.TestClient, k8sh *utils.K8sHelper, s suite.Suite, namespace string) {
	if isCSIRunnig(k8sh, namespace) {
		logger.Info("test Ceph CSI driver")
		createCephCSISecret(helper, k8sh, s, namespace)
		createCephPools(helper, s, namespace)
		createCSIStorageClass(k8sh, s, namespace)
		createCSIRBDTestPod(k8sh, s, namespace)
	}
}

func isCSIRunnig(k8sh *utils.K8sHelper, namespace string) bool {
	return k8sh.IsPodWithLabelPresent("app=csi-rbdplugin", installer.SystemNamespace(namespace))
}

func createCephCSISecret(helper *clients.TestClient, k8sh *utils.K8sHelper, s suite.Suite, namespace string) {
	commandArgs := []string{"-c", "ceph auth get-key client.admin"}
	keyResult, err := k8sh.Exec(namespace, "rook-ceph-tools", "bash", commandArgs)
	logger.Infof("Ceph get-key: %s", keyResult)
	require.Nil(s.T(), err)
	commandArgs = []string{"-c", "ceph mon_status"}
	monResult, err := k8sh.Exec(namespace, "rook-ceph-tools", "bash", commandArgs)
	logger.Infof("Ceph mon_status: %s", monResult)
	require.Nil(s.T(), err)

	var mon monclient.MonStatusResponse
	err = json.Unmarshal([]byte(monResult), &mon)
	require.Nil(s.T(), err)
	require.True(s.T(), len(mon.MonMap.Mons) > 0, "no mon found")
	monStr := strings.Split(mon.MonMap.Mons[0].Address, "/")[0]
	require.True(s.T(), len(monStr) > 0, "invalid mon addr")

	_, err = k8sh.Clientset.CoreV1().Secrets(namespace).Create(&v1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      csiSecretName,
			Namespace: namespace,
		},
		Data: map[string][]byte{
			"admin":    []byte(keyResult),
			"adminID":  []byte("admin"),
			"adminKey": []byte(keyResult),
			"monitors": []byte(monStr),
		},
	})
	require.Nil(s.T(), err)
	logger.Info("Created Ceph CSI Secret")
}

func createCephPools(helper *clients.TestClient, s suite.Suite, namespace string) {
	err := helper.PoolClient.Create(csiPoolRBD, namespace, 1)
	require.Nil(s.T(), err)

	err = helper.PoolClient.Create(csiPoolCephFS, namespace, 1)
	require.Nil(s.T(), err)
}

func createCSIStorageClass(k8sh *utils.K8sHelper, s suite.Suite, namespace string) {
	rbdSC := `
apiVersion: storage.k8s.io/v1
kind: StorageClass
metadata:
   name: ` + csiSCRBD + `
provisioner: rbd.csi.ceph.com
parameters:
    monValueFromSecret: "monitors"
    pool: ` + csiPoolRBD + `
    csi.storage.k8s.io/provisioner-secret-name: ` + csiSecretName + `
    csi.storage.k8s.io/provisioner-secret-namespace: ` + namespace + `
    csi.storage.k8s.io/node-publish-secret-name: ` + csiSecretName + `
    csi.storage.k8s.io/node-publish-secret-namespace: ` + namespace + `
    adminid: admin
    userid: admin
`
	cephFSSC := `
apiVersion: storage.k8s.io/v1
kind: StorageClass
metadata:
   name: ` + csiSCCephFS + `
provisioner: cephfs.csi.ceph.com
parameters:
    monValueFromSecret: "monitors"
    pool: ` + csiPoolCephFS + `
    csi.storage.k8s.io/provisioner-secret-name: ` + csiSecretName + `
    csi.storage.k8s.io/provisioner-secret-namespace: ` + namespace + `
    csi.storage.k8s.io/node-stage-secret-name: ` + csiSecretName + `
    csi.storage.k8s.io/node-stage-secret-namespace: ` + namespace + `
    adminid: admin
    userid: admin
`
	err := k8sh.ResourceOperation("apply", rbdSC)
	require.Nil(s.T(), err)

	err = k8sh.ResourceOperation("apply", cephFSSC)
	require.Nil(s.T(), err)
}

func createCSIRBDTestPod(k8sh *utils.K8sHelper, s suite.Suite, namespace string) {
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
	err := k8sh.ResourceOperation("create", pod)
	require.Nil(s.T(), err)
	isPodRunning := k8sh.IsPodRunning(csiTestRBDPodName, namespace)
	if !isPodRunning {
		k8sh.PrintPodDescribe(namespace, csiTestRBDPodName)
		k8sh.PrintPodStatus(namespace)
	} else {
		// cleanup the pod and pv
		err = k8sh.ResourceOperation("delete", pod)
	}

	require.True(s.T(), isPodRunning, "csi rbd test pod fails to run")
}
