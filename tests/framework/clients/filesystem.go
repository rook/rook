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

package clients

import (
	"fmt"

	"github.com/rook/rook/pkg/model"
	"github.com/rook/rook/tests/framework/contracts"
	"github.com/rook/rook/tests/framework/utils"
	"github.com/stretchr/testify/assert"
)

//FilesystemOperation is a wrapper for k8s rook file operations
type FilesystemOperation struct {
	k8sClient  *utils.K8sHelper
	restClient contracts.RestAPIOperator
}

// CreateK8sFilesystemOperation Constructor to create FilesystemOperation - client to perform rook file system operations on k8s
func CreateK8sFilesystemOperation(k8shelp *utils.K8sHelper, rookRestClient contracts.RestAPIOperator) *FilesystemOperation {
	return &FilesystemOperation{k8sClient: k8shelp, restClient: rookRestClient}
}

//FSCreate Function to create a filesystem in rook
//Input parameters -
// name -  name of the shared file system to be created
//Output - output returned by rook rest API client
func (rfs *FilesystemOperation) FSCreate(name, namespace string, callAPI bool, k8sh *utils.K8sHelper) error {
	createFilesystem := model.FilesystemRequest{
		Name:         name,
		MetadataPool: model.Pool{ReplicatedConfig: model.ReplicatedPoolConfig{Size: 1}},
		DataPools: []model.Pool{
			{ReplicatedConfig: model.ReplicatedPoolConfig{Size: 1}},
		},
		MetadataServer: model.MetadataServer{
			ActiveCount: 1,
		},
	}

	if callAPI {
		logger.Infof("creating the filesystem via the rest API")
		if _, err := rfs.restClient.CreateFilesystem(createFilesystem); err != nil {
			return err
		}
	} else {
		logger.Infof("creating the filesystem via CRD")
		fsSpec := fmt.Sprintf(`apiVersion: rook.io/v1alpha1
kind: Filesystem
metadata:
  name: %s
  namespace: %s
spec:
  metadataPool:
    replicated:
      size: 1
  dataPools:
  - replicated:
      size: 1
  metadataServer:
    activeCount: 1
    activeStandby: true
`, name, namespace)

		if _, err := k8sh.ResourceOperation("create", fsSpec); err != nil {
			return err
		}
	}

	logger.Infof("Make sure rook-ceph-mds pod is running")
	assert.True(k8sh.T(), k8sh.IsPodWithLabelRunning(fmt.Sprintf("rook_file_system=%s", name), namespace))

	assert.True(k8sh.T(), k8sh.CheckPodCountAndState("rook-ceph-mds", namespace, 2, "Running"),
		"Make sure there are two rook-ceph-mds pods present in Running state")

	return nil
}

//FSDelete Function to delete a filesystem in rook
//Input parameters -
// name -  name of the shared file system to be deleted
//Output - output returned by rook rest API client
func (rfs *FilesystemOperation) FSDelete(name string) (string, error) {
	deleteFilesystem := model.FilesystemRequest{Name: name}
	return rfs.restClient.DeleteFilesystem(deleteFilesystem)
}

//FSList Function to list a filesystem in rook
//Output - output returned by rook rest API client
func (rfs *FilesystemOperation) FSList() ([]model.Filesystem, error) {
	return rfs.restClient.GetFilesystems()

}

//FSMount Function to Mount a file system created by rook(on a pod)
//Input parameters -
//name - path to the yaml definition file - definition of pod to be created that mounts existing file system
//path - ignored in this case - moount path is defined in the path definition
//output - output returned by k8s create pod operaton and/or error
func (rfs *FilesystemOperation) FSMount(name string, path string) (string, error) {
	cmdArgs := []string{"create", "-f", "name"}
	result, err := rfs.k8sClient.Kubectl(cmdArgs...)

	if err != nil {
		return "", fmt.Errorf("Unable to mount Filesystem -- : %s", err)
	}
	return result, nil

}

//FSWrite Function to write  data to file system created by rook ,i.e. write data to a pod that has filesystem mounted
// Input parameters -
//name - path to a yaml file that creates a pod  - pod should be defined to use a pvc that was created earlier
//mountpath - folder on the pod were data is supposed to be written(should match the mountpath described in the pod definition)
//data - data to be written
//filename - file where data is written to
//namespace - optional param - namespace of the pod
//Output  - k8s exec pod operation output and/or error
func (rfs *FilesystemOperation) FSWrite(name string, mountpath string, data string, filename string, namespace string) (string, error) {
	wt := "echo \"" + data + "\">" + mountpath + "/" + filename
	args := []string{"exec", name}

	if namespace != "" {
		args = append(args, "-n", namespace)
	}
	args = append(args, "--", "sh", "-c", wt)

	result, err := rfs.k8sClient.Kubectl(args...)
	if err != nil {
		return "", fmt.Errorf("Unable to write data to pod -- : %s", err)

	}
	return result, nil

}

//FSRead Function to write read from file system  created by rook ,i.e. Read data from a pod that filesystem mounted
// Input parameters -
//name - path to a yaml file that creates a pod  - pod should be defined to use a pvc that was created earlier
//mountpath - folder on the pod were data is supposed to be written(should match the mountpath described in the pod definition)
//filename - file to be read
//namespace - optional param - namespace of the pod
//Output  - k8s exec pod operation output and/or error
func (rfs *FilesystemOperation) FSRead(name string, mountpath string, filename string, namespace string) (string, error) {
	rd := mountpath + "/" + filename

	args := []string{"exec", name}

	if namespace != "" {
		args = append(args, "-n", namespace)
	}
	args = append(args, "--", "cat", rd)

	result, err := rfs.k8sClient.Kubectl(args...)
	if err != nil {
		return "", fmt.Errorf("Unable to write data to pod -- : %s", err)

	}
	return result, nil

}

//FSUnmount Function to UnMount a file system created by rook(delete pod)
//Input parameters -
//name - path to the yaml definition file - definition of pod to be deleted that has a file system mounted
//path - ignored in this case - moount path is defined in the path definition
//output - output returned by k8s delete pod operaton and/or error
func (rfs *FilesystemOperation) FSUnmount(name string) (string, error) {
	args := []string{"delete", "-f", name}
	result, err := rfs.k8sClient.Kubectl(args...)
	if err != nil {
		return "", fmt.Errorf("Unable to unmount Filesystem -- : %s", err)

	}
	return result, nil
}
