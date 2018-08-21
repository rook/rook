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

	"github.com/rook/rook/pkg/daemon/ceph/client"
	"github.com/rook/rook/tests/framework/installer"
	"github.com/rook/rook/tests/framework/utils"
	"github.com/stretchr/testify/assert"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

//FilesystemOperation is a wrapper for k8s rook file operations
type FilesystemOperation struct {
	k8sh      *utils.K8sHelper
	manifests installer.CephManifests
}

// CreateFilesystemOperation Constructor to create FilesystemOperation - client to perform rook file system operations on k8s
func CreateFilesystemOperation(k8sh *utils.K8sHelper, manifests installer.CephManifests) *FilesystemOperation {
	return &FilesystemOperation{k8sh, manifests}
}

// FSCreate Function to create a filesystem in rook
// Input parameters -
//  name -  name of the shared file system to be created
//  Output - output returned by the ceph command
func (f *FilesystemOperation) Create(name, namespace string) error {

	logger.Infof("creating the filesystem via CRD")
	if _, err := f.k8sh.ResourceOperation("create", f.manifests.GetFilesystem(namespace, name)); err != nil {
		return err
	}

	logger.Infof("Make sure rook-ceph-mds pod is running")
	err := f.k8sh.WaitForLabeledPodToRun(fmt.Sprintf("rook_file_system=%s", name), namespace)
	assert.Nil(f.k8sh.T(), err)

	assert.True(f.k8sh.T(), f.k8sh.CheckPodCountAndState("rook-ceph-mds", namespace, 2, "Running"),
		"Make sure there are two rook-ceph-mds pods present in Running state")

	return nil
}

// Delete Function to delete a filesystem in rook
// Input parameters -
// name -  name of the shared file system to be deleted
// Output - output returned by the call
func (f *FilesystemOperation) Delete(name, namespace string) error {
	options := &metav1.DeleteOptions{}
	return f.k8sh.RookClientset.RookV1alpha1().Filesystems(namespace).Delete(name, options)
}

// List Function to list a filesystem in rook
// Output - output returned by the call
func (f *FilesystemOperation) List(namespace string) ([]client.CephFilesystem, error) {
	context := f.k8sh.MakeContext()
	filesystems, err := client.ListFilesystems(context, namespace)
	if err != nil {
		return nil, fmt.Errorf("failed to list pools: %+v", err)
	}
	return filesystems, nil
}

// Write Function to write  data to file system created by rook ,i.e. write data to a pod that has filesystem mounted
// Input parameters -
// name - path to a yaml file that creates a pod  - pod should be defined to use a pvc that was created earlier
// mountpath - folder on the pod were data is supposed to be written(should match the mountpath described in the pod definition)
// data - data to be written
// filename - file where data is written to
// namespace - optional param - namespace of the pod
// Output  - k8s exec pod operation output and/or error
func (f *FilesystemOperation) Write(name string, data string, filename string, namespace string) (string, error) {
	return writeToPod(f.k8sh, name, data, filename, namespace)
}

// Read Function to write read from file system  created by rook ,i.e. Read data from a pod that filesystem mounted
// Input parameters -
// name - path to a yaml file that creates a pod  - pod should be defined to use a pvc that was created earlier
// mountpath - folder on the pod were data is supposed to be written(should match the mountpath described in the pod definition)
// filename - file to be read
// namespace - optional param - namespace of the pod
// Output  - k8s exec pod operation output and/or error
func (f *FilesystemOperation) Read(name string, filename string, namespace string) (string, error) {
	return readFromPod(f.k8sh, name, filename, namespace)
}
