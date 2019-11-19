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
	"strings"
	"time"

	cephclient "github.com/rook/rook/pkg/daemon/ceph/client"
	"github.com/rook/rook/tests/framework/clients"
	"github.com/rook/rook/tests/framework/installer"
	"github.com/rook/rook/tests/framework/utils"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	fileMountPath        = "/tmp/rookfs"
	filePodName          = "file-test"
	fileMountUserPodName = "file-mountuser-test"
	fileMountUser        = "filemountuser"
	fileMountSecret      = "file-mountuser-cephkey"
)

// Smoke Test for File System Storage - Test check the following operations on Filesystem Storage in order
// Create,Mount,Write,Read,Unmount and Delete.
func runFileE2ETest(helper *clients.TestClient, k8sh *utils.K8sHelper, s suite.Suite, namespace string, filesystemName string) {
	defer fileTestDataCleanUp(helper, k8sh, s, filePodName, namespace, filesystemName)
	logger.Infof("Running on Rook Cluster %s", namespace)
	logger.Infof("File Storage End To End Integration Test - create, mount, write to, read from, and unmount")

	createFilesystem(helper, k8sh, s, namespace, filesystemName)

	// Create a test pod where CephFS is consumed without user creds
	createFilesystemConsumerPod(helper, k8sh, s, namespace, filesystemName)

	// Create a test pod where CephFS is consumed with a mountUser and mountSecret specified.
	createFilesystemMountCephCredentials(helper, k8sh, s, namespace, filesystemName)
	createFilesystemMountUserConsumerPod(helper, k8sh, s, namespace, filesystemName)

	// Test reading and writing to the first pod
	err := writeAndReadToFilesystem(helper, k8sh, s, namespace, filePodName, "test_file")
	assert.NoError(s.T(), err)

	// Test reading and writing to the second pod
	err = writeAndReadToFilesystem(helper, k8sh, s, namespace, fileMountUserPodName, "canttouchthis")
	assert.Error(s.T(), err, "we should not be able to write to file canttouchthis on CephFS `/`")
	err = writeAndReadToFilesystem(helper, k8sh, s, namespace, fileMountUserPodName, "foo/test_file")
	assert.NoError(s.T(), err, "we should be able to write to the `/foo` directory on CephFS")

	// Start the NFS daemons
	testNFSDaemons(helper, k8sh, s, namespace, filesystemName)

	// Cleanup the filesystem and its clients
	cleanupFilesystemConsumer(k8sh, s, namespace, filePodName)
	cleanupFilesystemConsumer(k8sh, s, namespace, fileMountUserPodName)
	downscaleMetadataServers(helper, k8sh, s, namespace, filesystemName)
	cleanupFilesystem(helper, k8sh, s, namespace, filesystemName)
}

func testNFSDaemons(helper *clients.TestClient, k8sh *utils.K8sHelper, s suite.Suite, namespace string, filesystemName string) {
	name := "my-nfs"
	err := helper.NFSClient.Create(namespace, name, filesystemName+"-data0", 2)
	require.Nil(s.T(), err)

	err = helper.NFSClient.Delete(namespace, name)
	assert.Nil(s.T(), err)
}

func createFilesystemConsumerPod(helper *clients.TestClient, k8sh *utils.K8sHelper, s suite.Suite, namespace string, filesystemName string) {
	err := createPodWithFilesystem(k8sh, s, filePodName, namespace, filesystemName, false)
	require.NoError(s.T(), err)
	filePodRunning := k8sh.IsPodRunning(filePodName, namespace)
	require.True(s.T(), filePodRunning, "make sure file-test pod is in running state")
	logger.Infof("File system mounted successfully")
}

func writeAndReadToFilesystem(helper *clients.TestClient, k8sh *utils.K8sHelper, s suite.Suite, namespace, podName, filename string) error {
	logger.Infof("Write to file system")
	message := "Test Data for file system storage"
	if err := k8sh.WriteToPod(namespace, podName, filename, message); err != nil {
		return err
	}

	return k8sh.ReadFromPod(namespace, podName, filename, message)
}

func downscaleMetadataServers(helper *clients.TestClient, k8sh *utils.K8sHelper, s suite.Suite, namespace, fsName string) {
	logger.Infof("downscaling file system metadata servers")
	err := helper.FSClient.ScaleDown(fsName, namespace)
	require.Nil(s.T(), err)
}

func cleanupFilesystemConsumer(k8sh *utils.K8sHelper, s suite.Suite, namespace string, podName string) {
	logger.Infof("Delete file System consumer")
	err := k8sh.DeletePod(namespace, podName)
	require.Nil(s.T(), err)
	require.True(s.T(), k8sh.IsPodTerminated(podName, namespace), fmt.Sprintf("make sure %s pod is terminated", podName))
	logger.Infof("File system consumer deleted")
}

// cleanupFilesystem cleans up the filesystem and checks if all mds pods are terminated before continuing
func cleanupFilesystem(helper *clients.TestClient, k8sh *utils.K8sHelper, s suite.Suite, namespace string, filesystemName string) {
	logger.Infof("Deleting file system")
	err := helper.FSClient.Delete(filesystemName, namespace)
	require.Nil(s.T(), err)
	logger.Infof("File system %s deleted", filesystemName)
}

// Test File System Creation on Rook that was installed on a custom namespace i.e. Namespace != "rook" and delete it again
func runFileE2ETestLite(helper *clients.TestClient, k8sh *utils.K8sHelper, s suite.Suite, namespace string, filesystemName string) {
	logger.Infof("File Storage End to End Integration Test - create Filesystem and make sure mds pod is running")
	logger.Infof("Running on Rook Cluster %s", namespace)
	createFilesystem(helper, k8sh, s, namespace, filesystemName)
	cleanupFilesystem(helper, k8sh, s, namespace, filesystemName)
}

func createFilesystem(helper *clients.TestClient, k8sh *utils.K8sHelper, s suite.Suite, namespace, filesystemName string) {
	logger.Infof("Create file System")
	fscErr := helper.FSClient.Create(filesystemName, namespace)
	require.Nil(s.T(), fscErr)
	logger.Infof("File system %s created", filesystemName)

	filesystemList, _ := helper.FSClient.List(namespace)
	require.Equal(s.T(), 1, len(filesystemList), "There should be one shared file system present")
}

func fileTestDataCleanUp(helper *clients.TestClient, k8sh *utils.K8sHelper, s suite.Suite, podName string, namespace string, filesystemName string) {
	logger.Infof("Cleaning up file system")
	err := k8sh.DeletePod(namespace, podName)
	assert.NoError(s.T(), err)
	helper.FSClient.Delete(filesystemName, namespace)
}

func createPodWithFilesystem(k8sh *utils.K8sHelper, s suite.Suite, podName, namespace, filesystemName string, mountUser bool) error {
	driverName := installer.SystemNamespace(namespace)
	testPodManifest := getFilesystemTestPod(podName, namespace, filesystemName, driverName, mountUser)
	if err := k8sh.ResourceOperation("create", testPodManifest); err != nil {
		return fmt.Errorf("failed to create pod -- %s. %+v", testPodManifest, err)
	}
	return nil
}

func getFilesystemTestPod(podName, namespace, filesystemName, driverName string, mountUser bool) string {
	mountUserInsert := ""
	if mountUser {
		mountUserInsert = `
        mountUser: ` + fileMountUser + `
        mountSecret: ` + fileMountSecret
	}
	// Bash's sleep signal handling: http://mywiki.wooledge.org/SignalTrap#When_is_the_signal_handled.3F
	return `apiVersion: v1
kind: Pod
metadata:
  name: ` + podName + `
  namespace: ` + namespace + `
spec:
  containers:
  - name: ` + podName + `
    image: krallin/ubuntu-tini
    command:
        - "/usr/local/bin/tini"
        - "-g"
        - "--"
        - "sleep"
        - "1800"
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
        clusterNamespace: ` + namespace + mountUserInsert + `
  restartPolicy: Always
`
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
	mtfsErr := createPodWithFilesystem(k8sh, s, fileMountUserPodName, namespace, filesystemName, true)
	require.Nil(s.T(), mtfsErr)
	filePodRunning := k8sh.IsPodRunning(fileMountUserPodName, namespace)
	require.True(s.T(), filePodRunning, "make sure file-mountuser-test pod is in running state")
	logger.Infof("File system mounted successfully")
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

func waitForFilesystemActive(k8sh *utils.K8sHelper, namespace, filesystemName string) error {
	cmd := cephclient.NewCephCommand(k8sh.MakeContext(), namespace, []string{"fs", "status", filesystemName})
	var stat []byte
	var err error
	logger.Infof("waiting for filesystem %q to be active", filesystemName)
	for i := 0; i < utils.RetryLoop; i++ {
		stat, err := cmd.Run()
		if err != nil {
			logger.Warningf("failed to get filesystem %q status. %+v", filesystemName, err)
		}
		// as long as at least one mds is active, it's okay
		if strings.Contains(string(stat), "active") {
			logger.Infof("done waiting for filesystem %q to be active", filesystemName)
			return nil
		}
		logger.Infof("waiting for filesystem %q to be active", filesystemName)
		time.Sleep(utils.RetryInterval * time.Second)
	}
	return fmt.Errorf("gave up waiting to get filesystem %q status [err: %+v] Status returned:\n%s", filesystemName, err, string(stat))
}
