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
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// FilesystemOperation is a wrapper for k8s rook file operations
type FilesystemOperation struct {
	k8sh      *utils.K8sHelper
	manifests installer.CephManifests
}

// CreateFilesystemOperation Constructor to create FilesystemOperation - client to perform rook file system operations on k8s
func CreateFilesystemOperation(k8sh *utils.K8sHelper, manifests installer.CephManifests) *FilesystemOperation {
	return &FilesystemOperation{k8sh, manifests}
}

// Create creates a filesystem in Rook
func (f *FilesystemOperation) Create(name, namespace string) error {
	logger.Infof("creating the filesystem via CRD")
	if err := f.k8sh.ResourceOperation("apply", f.manifests.GetFilesystem(namespace, name, 2)); err != nil {
		return err
	}

	logger.Infof("Make sure rook-ceph-mds pod is running")
	err := f.k8sh.WaitForLabeledPodsToRun(fmt.Sprintf("rook_file_system=%s", name), namespace)
	assert.Nil(f.k8sh.T(), err)

	assert.True(f.k8sh.T(), f.k8sh.CheckPodCountAndState("rook-ceph-mds", namespace, 4, "Running"),
		"Make sure there are four rook-ceph-mds pods present in Running state")

	return nil
}

// ScaleDown scales down the number of active metadata servers of a filesystem in Rook
func (f *FilesystemOperation) ScaleDown(name, namespace string) error {
	logger.Infof("scaling down the number of filesystem active metadata servers via CRD")
	if err := f.k8sh.ResourceOperation("apply", f.manifests.GetFilesystem(namespace, name, 1)); err != nil {
		return err
	}

	assert.True(f.k8sh.T(), f.k8sh.CheckPodCountAndState("rook-ceph-mds", namespace, 2, "Running"),
		"Make sure there are two rook-ceph-mds pods present in Running state")

	return nil
}

// Delete deletes a filesystem in Rook
func (f *FilesystemOperation) Delete(name, namespace string) error {
	options := &metav1.DeleteOptions{}
	logger.Infof("Deleting filesystem %s in namespace %s", name, namespace)
	err := f.k8sh.RookClientset.CephV1().CephFilesystems(namespace).Delete(name, options)
	if err != nil && !errors.IsNotFound(err) {
		return err
	}

	logger.Infof("Deleted filesystem %s in namespace %s", name, namespace)
	return nil
}

// List lists filesystems in Rook
func (f *FilesystemOperation) List(namespace string) ([]client.CephFilesystem, error) {
	context := f.k8sh.MakeContext()
	filesystems, err := client.ListFilesystems(context, namespace)
	if err != nil {
		return nil, fmt.Errorf("failed to list pools: %+v", err)
	}
	return filesystems, nil
}
