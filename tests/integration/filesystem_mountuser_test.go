/*
Copyright 2016 The Rook Authors. All rights reserved.

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
	"fmt"

	"github.com/rook/rook/tests/framework/clients"
	"github.com/rook/rook/tests/framework/installer"
	"github.com/rook/rook/tests/framework/utils"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
	"k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	fileMountUserPodName = "file-mountuser-test"
	fileMountUser        = "filemountuser"
	fileMountSecret      = "file-mountuser-cephkey"
)

// Smoke Test for File System Storage mountUser and mountSecret parameter - Test check the
// following operations on Filesystem Storage in order Create,Mount,Write,Read,Unmount and Delete,
// with a mountUser and mountSecret specified.
func runFileMountUserE2ETest(helper *clients.TestClient, k8sh *utils.K8sHelper, s suite.Suite, namespace string, filesystemName string) {
	defer fileTestDataCleanUp(helper, k8sh, s, fileMountUserPodName, namespace, filesystemName)
	logger.Infof("Running on Rook Cluster %s", namespace)
	logger.Infof("File Storage MountUser/MountSecret End To End Integration Test - create, mount, write to, read from, and unmount")

	createFilesystem(helper, k8sh, s, namespace, filesystemName)
	createFilesystemMountCephCredentials(helper, k8sh, s, namespace, filesystemName)
	createFilesystemMountUserConsumerPod(helper, k8sh, s, namespace, filesystemName)
	err := writeAndReadToFilesystem(helper, k8sh, s, namespace, fileMountUserPodName, "canttouchthis")
	require.NotNil(s.T(), err, "we should not be able to write to file canttouchthis on CephFS `/`")
	err = writeAndReadToFilesystem(helper, k8sh, s, namespace, fileMountUserPodName, "foo/test_file")
	require.Nil(s.T(), err, "we should be able to write to the `/foo` directory on CephFS")
	cleanupFilesystemConsumer(helper, k8sh, s, namespace, filesystemName, fileMountUserPodName)
	cleanupFilesystem(helper, k8sh, s, namespace, filesystemName)
}

func createFilesystemMountCephCredentials(helper *clients.TestClient, k8sh *utils.K8sHelper, s suite.Suite, namespace string, filesystemName string) {
	// Create agent binding for access to Secrets
	err := k8sh.ResourceOperation("apply", getFilesystemAgentMountSecretsBinding(namespace))
	require.Nil(s.T(), err)
	// Mount CephFS in toolbox and create /foo directory on it
	logger.Info("Creating /foo directory on CephFS")
	_, err = k8sh.Exec(namespace, "rook-ceph-tools", "mkdir", []string{"-p", utils.TestMountPath})
	require.Nil(s.T(), err)
	_, err = k8sh.ExecWithRetry(3, namespace, "rook-ceph-tools", "bash", []string{"-c", fmt.Sprintf("mount -t ceph -o mds_namespace=%s,name=admin,secret=$(grep key /etc/ceph/keyring | awk '{print $3}') $(grep mon_host /etc/ceph/ceph.conf | awk '{print $3}'):/ %s", filesystemName, utils.TestMountPath)})
	require.Nil(s.T(), err)
	_, err = k8sh.Exec(namespace, "rook-ceph-tools", "mkdir", []string{"-p", fmt.Sprintf("%s/foo", utils.TestMountPath)})
	require.Nil(s.T(), err)
	_, err = k8sh.Exec(namespace, "rook-ceph-tools", "umount", []string{utils.TestMountPath})
	require.Nil(s.T(), err)
	logger.Info("Created /foo directory on CephFS")

	// Create Ceph credentials which allow CephFS access to `/foo` but not `/`.
	commandArgs := []string{
		"-c",
		fmt.Sprintf(
			`ceph auth get-or-create-key client.%s mon "allow r" osd "allow rw pool=%s-data0" mds "allow r, allow rw path=/foo"`,
			fileMountUser,
			filesystemName,
		),
	}
	logger.Infof("ceph credentials command args: %s", commandArgs[1])
	result, err := k8sh.Exec(namespace, "rook-ceph-tools", "bash", commandArgs)
	logger.Infof("Ceph filesystem credentials output: %s", result)
	logger.Info("Created Ceph credentials")
	require.Nil(s.T(), err)
	// Save Ceph credentials to Kubernetes
	_, err = k8sh.Clientset.CoreV1().Secrets(namespace).Create(&v1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      fileMountSecret,
			Namespace: namespace,
		},
		Data: map[string][]byte{
			"mykey": []byte(result),
		},
	})
	require.Nil(s.T(), err)
	logger.Info("Created Ceph credentials Secret in Kubernetes")
}

func createFilesystemMountUserConsumerPod(helper *clients.TestClient, k8sh *utils.K8sHelper, s suite.Suite, namespace string, filesystemName string) {
	mtfsErr := podWithFilesystem(k8sh, s, fileMountUserPodName, namespace, filesystemName, "create", getFilesystemMountUserTestPod)
	require.Nil(s.T(), mtfsErr)
	filePodRunning := k8sh.IsPodRunning(fileMountUserPodName, namespace)
	if !filePodRunning {
		k8sh.PrintPodDescribe(namespace, fileMountUserPodName)
		k8sh.PrintPodStatus(namespace)
		k8sh.PrintPodStatus(installer.SystemNamespace(namespace))
	}
	require.True(s.T(), filePodRunning, "make sure file-mountuser-test pod is in running state")
	logger.Infof("File system mounted successfully")
}

func getFilesystemMountUserTestPod(podName string, namespace string, filesystemName string, driverName string) string {
	return `apiVersion: v1
kind: Pod
metadata:
  name: ` + podName + `
  namespace: ` + namespace + `
spec:
  containers:
  - name: ` + podName + `
    image: busybox
    command:
        - sleep
        - "3600"
    imagePullPolicy: IfNotPresent
    env:
    volumeMounts:
    - mountPath: "` + utils.TestMountPath + `"
      name: ` + filesystemName + `
  volumes:
  - name: ` + filesystemName + `
    flexVolume:
      driver: ceph.rook.io/` + driverName + `
      fsType: ceph
      options:
        fsName: ` + filesystemName + `
        clusterNamespace: ` + namespace + `
        mountUser: ` + fileMountUser + `
        mountSecret: ` + fileMountSecret + `
  restartPolicy: Never
`
}

func getFilesystemAgentMountSecretsBinding(namespace string) string {
	return `apiVersion: rbac.authorization.k8s.io/v1beta1
kind: ClusterRoleBinding
metadata:
  name: rook-ceph-agent-mount
  labels:
    operator: rook
    storage-backend: ceph
roleRef:
  apiGroup: rbac.authorization.k8s.io
  kind: ClusterRole
  name: rook-ceph-agent-mount
subjects:
- kind: ServiceAccount
  name: rook-ceph-system
  namespace: ` + namespace + `
`
}
