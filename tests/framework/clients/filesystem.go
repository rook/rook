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
	"context"
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
func (f *FilesystemOperation) Create(name, namespace string, activeCount int) error {
	logger.Infof("creating the filesystem via CRD")
	if err := f.k8sh.ResourceOperation("apply", f.manifests.GetFilesystem(name, activeCount)); err != nil {
		return err
	}

	logger.Infof("Make sure rook-ceph-mds pod is running")
	err := f.k8sh.WaitForLabeledPodsToRun(fmt.Sprintf("rook_file_system=%s", name), namespace)
	assert.Nil(f.k8sh.T(), err)

	assert.True(f.k8sh.T(), f.k8sh.CheckPodCountAndState("rook-ceph-mds", namespace, activeCount*2, "Running"),
		"Make sure there are four rook-ceph-mds pods present in Running state")

	return nil
}

// CreateStorageClass creates a storage class for CephFS clients
func (f *FilesystemOperation) CreateStorageClass(fsName, systemNamespace, namespace, storageClassName string) error {
	return f.k8sh.ResourceOperation("apply", f.manifests.GetFileStorageClass(fsName, storageClassName))
}

// CreateSnapshotClass creates a snapshot class for CephFS clients
func (f *FilesystemOperation) CreateSnapshotClass(snapshotClassName, reclaimPolicy, namespace string) error {
	return f.k8sh.ResourceOperation("apply", f.manifests.GetFileStorageSnapshotClass(snapshotClassName, reclaimPolicy))
}

// CreatePVCRestore creates a pvc from snapshot
func (f *FilesystemOperation) CreatePVCRestore(namespace, claimName, snapshotName, storageClassName, mode, size string) error {
	return f.k8sh.ResourceOperation("apply", installer.GetPVCRestore(claimName, snapshotName, namespace, storageClassName, mode, size))
}

// CreatePVCClone creates a pvc from pvc
func (f *FilesystemOperation) CreatePVCClone(namespace, cloneClaimName, parentClaimName, storageClassName, mode, size string) error {
	return f.k8sh.ResourceOperation("apply", installer.GetPVCClone(cloneClaimName, parentClaimName, namespace, storageClassName, mode, size))
}

// CreateSnapshot creates a snapshot from pvc
func (f *FilesystemOperation) CreateSnapshot(snapshotName, claimName, snapshotClassName, namespace string) error {
	return f.k8sh.ResourceOperation("apply", installer.GetSnapshot(snapshotName, claimName, snapshotClassName, namespace))
}

// DeleteSnapshot deletes the snapshot
func (f *FilesystemOperation) DeleteSnapshot(snapshotName, claimName, snapshotClassName, namespace string) error {
	return f.k8sh.ResourceOperation("delete", installer.GetSnapshot(snapshotName, claimName, snapshotClassName, namespace))
}

func (f *FilesystemOperation) DeletePVC(namespace, claimName string) error {
	ctx := context.TODO()
	logger.Infof("deleting pvc %q from namespace %q", claimName, namespace)
	return f.k8sh.Clientset.CoreV1().PersistentVolumeClaims(namespace).Delete(ctx, claimName, metav1.DeleteOptions{})
}

func (f *FilesystemOperation) DeleteStorageClass(storageClassName string) error {
	ctx := context.TODO()
	logger.Infof("deleting storage class %q", storageClassName)
	err := f.k8sh.Clientset.StorageV1().StorageClasses().Delete(ctx, storageClassName, metav1.DeleteOptions{})
	if err != nil && !errors.IsNotFound(err) {
		return fmt.Errorf("failed to delete storage class %q. %v", storageClassName, err)
	}

	return nil
}

func (f *FilesystemOperation) CreatePVC(namespace, claimName, storageClassName, mode, size string) error {
	return f.k8sh.ResourceOperation("apply", installer.GetPVC(claimName, namespace, storageClassName, mode, size))
}

func (f *FilesystemOperation) CreatePod(podName, claimName, namespace, mountPoint string, readOnly bool) error {
	return f.k8sh.ResourceOperation("apply", installer.GetPodWithVolume(podName, claimName, namespace, mountPoint, readOnly))
}

func (f *FilesystemOperation) DeleteSnapshotClass(snapshotClassName, deletePolicy, namespace string) error {
	return f.k8sh.ResourceOperation("delete", f.manifests.GetFileStorageSnapshotClass(snapshotClassName, deletePolicy))
}

// ScaleDown scales down the number of active metadata servers of a filesystem in Rook
func (f *FilesystemOperation) ScaleDown(name, namespace string) error {
	logger.Infof("scaling down the number of filesystem active metadata servers via CRD")
	if err := f.k8sh.ResourceOperation("apply", f.manifests.GetFilesystem(name, 1)); err != nil {
		return err
	}

	assert.True(f.k8sh.T(), f.k8sh.CheckPodCountAndState("rook-ceph-mds", namespace, 2, "Running"),
		"Make sure there are two rook-ceph-mds pods present in Running state")

	return nil
}

// Delete deletes a filesystem in Rook
func (f *FilesystemOperation) Delete(name, namespace string) error {
	ctx := context.TODO()
	options := &metav1.DeleteOptions{}
	logger.Infof("Deleting filesystem %s in namespace %s", name, namespace)
	err := f.k8sh.RookClientset.CephV1().CephFilesystems(namespace).Delete(ctx, name, *options)
	if err != nil && !errors.IsNotFound(err) {
		return err
	}

	crdCheckerFunc := func() error {
		_, err := f.k8sh.RookClientset.CephV1().CephFilesystems(namespace).Get(ctx, name, metav1.GetOptions{})
		return err
	}

	logger.Infof("Deleted filesystem %s in namespace %s", name, namespace)
	return f.k8sh.WaitForCustomResourceDeletion(namespace, name, crdCheckerFunc)
}

// List lists filesystems in Rook
func (f *FilesystemOperation) List(namespace string) ([]client.CephFilesystem, error) {
	context := f.k8sh.MakeContext()
	clusterInfo := client.AdminClusterInfo(namespace)
	filesystems, err := client.ListFilesystems(context, clusterInfo)
	if err != nil {
		return nil, fmt.Errorf("failed to list pools: %+v", err)
	}
	return filesystems, nil
}
