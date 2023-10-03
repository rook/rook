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
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// BlockOperation is wrapper for k8s rook block operations
type BlockOperation struct {
	k8sClient *utils.K8sHelper
	manifests installer.CephManifests
}

type BlockImage struct {
	Name       string `json:"imageName"`
	PoolName   string `json:"poolName"`
	Size       uint64 `json:"size"`
	Device     string `json:"device"`
	MountPoint string `json:"mountPoint"`
}

// CreateBlockOperation - Constructor to create BlockOperation - client to perform rook Block operations on k8s
func CreateBlockOperation(k8shelp *utils.K8sHelper, manifests installer.CephManifests) *BlockOperation {
	return &BlockOperation{k8shelp, manifests}
}

// BlockCreate Function to create a Block using Rook
// Input parameters -
// manifest - pod definition that creates a pvc in k8s - yaml should describe name and size of pvc being created
// size - not user for k8s implementation since its described on the pvc yaml definition
// Output - k8s create pvc operation output and/or error
func (b *BlockOperation) Create(manifest string, size int) (string, error) {
	args := []string{"apply", "-f", "-"}
	result, err := b.k8sClient.KubectlWithStdin(manifest, args...)
	if err != nil {
		return "", fmt.Errorf("Unable to create block -- : %s", err)

	}
	return result, nil

}

func (b *BlockOperation) CreatePoolAndStorageClass(pvcNamespace, poolName, storageClassName, reclaimPolicy string) error {
	if err := b.k8sClient.ResourceOperation("apply", b.manifests.GetBlockPool(poolName, "1")); err != nil {
		return err
	}
	return b.k8sClient.ResourceOperation("apply", b.manifests.GetBlockStorageClass(poolName, storageClassName, reclaimPolicy))
}

func (b *BlockOperation) CreatePVC(namespace, claimName, storageClassName, mode, size string) error {
	return b.k8sClient.ResourceOperation("apply", installer.GetPVC(claimName, namespace, storageClassName, mode, size))
}

func (b *BlockOperation) CreatePod(podName, claimName, namespace, mountPoint string, readOnly bool) error {
	return b.k8sClient.ResourceOperation("apply", installer.GetPodWithVolume(podName, claimName, namespace, mountPoint, readOnly))
}

func (b *BlockOperation) CreateStorageClass(csi bool, poolName, storageClassName, reclaimPolicy, namespace string) error {
	return b.k8sClient.ResourceOperation("apply", b.manifests.GetBlockStorageClass(poolName, storageClassName, reclaimPolicy))
}

func (b *BlockOperation) DeletePVC(namespace, claimName string) error {
	ctx := context.TODO()
	logger.Infof("deleting pvc %q from namespace %q", claimName, namespace)
	return b.k8sClient.Clientset.CoreV1().PersistentVolumeClaims(namespace).Delete(ctx, claimName, metav1.DeleteOptions{})
}

func (b *BlockOperation) CreatePVCRestore(namespace, claimName, snapshotName, storageClassName, mode, size string) error {
	return b.k8sClient.ResourceOperation("apply", installer.GetPVCRestore(claimName, snapshotName, namespace, storageClassName, mode, size))
}

func (b *BlockOperation) CreatePVCClone(namespace, cloneClaimName, parentClaimName, storageClassName, mode, size string) error {
	return b.k8sClient.ResourceOperation("apply", installer.GetPVCClone(cloneClaimName, parentClaimName, namespace, storageClassName, mode, size))
}

func (b *BlockOperation) CreateSnapshotClass(snapshotClassName, deletePolicy, namespace string) error {
	return b.k8sClient.ResourceOperation("apply", b.manifests.GetBlockSnapshotClass(snapshotClassName, deletePolicy))
}

func (b *BlockOperation) DeleteSnapshotClass(snapshotClassName, deletePolicy, namespace string) error {
	return b.k8sClient.ResourceOperation("delete", b.manifests.GetBlockSnapshotClass(snapshotClassName, deletePolicy))
}

func (b *BlockOperation) CreateSnapshot(snapshotName, claimName, snapshotClassName, namespace string) error {
	return b.k8sClient.ResourceOperation("apply", installer.GetSnapshot(snapshotName, claimName, snapshotClassName, namespace))
}

func (b *BlockOperation) DeleteSnapshot(snapshotName, claimName, snapshotClassName, namespace string) error {
	return b.k8sClient.ResourceOperation("delete", installer.GetSnapshot(snapshotName, claimName, snapshotClassName, namespace))
}

func (b *BlockOperation) DeleteStorageClass(storageClassName string) error {
	ctx := context.TODO()
	logger.Infof("deleting storage class %q", storageClassName)
	err := b.k8sClient.Clientset.StorageV1().StorageClasses().Delete(ctx, storageClassName, metav1.DeleteOptions{})
	if err != nil && !errors.IsNotFound(err) {
		return fmt.Errorf("failed to delete storage class %q. %v", storageClassName, err)
	}

	return nil
}

// BlockDelete Function to delete a Block using Rook
// Input parameters -
// manifest - pod definition  where pvc is described - delete is run on the yaml definition
// Output  - k8s delete pvc operation output and/or error
func (b *BlockOperation) DeleteBlock(manifest string) (string, error) {
	args := []string{"delete", "-f", "-"}
	result, err := b.k8sClient.KubectlWithStdin(manifest, args...)
	if err != nil {
		return "", fmt.Errorf("Unable to delete block -- : %s", err)

	}
	return result, nil

}

// List Function to list all the block images in all pools
func (b *BlockOperation) ListAllImages(clusterInfo *client.ClusterInfo) ([]BlockImage, error) {
	// first list all the pools so that we can retrieve images from all pools
	pools, err := client.ListPoolSummaries(b.k8sClient.MakeContext(), clusterInfo)
	if err != nil {
		return nil, fmt.Errorf("failed to list pools: %+v", err)
	}

	// for each pool, get further details about all the images in the pool
	images := []BlockImage{}
	for _, p := range pools {
		cephImages, err := b.ListImagesInPool(clusterInfo, p.Name)
		if err != nil {
			return nil, fmt.Errorf("failed to get images from pool %s: %+v", p.Name, err)
		}
		images = append(images, cephImages...)
	}
	return images, nil
}

// List Function to list all the block images in a pool
func (b *BlockOperation) ListImagesInPool(clusterInfo *client.ClusterInfo, poolName string) ([]BlockImage, error) {
	// for each pool, get further details about all the images in the pool
	images := []BlockImage{}
	cephImages, err := client.ListImages(b.k8sClient.MakeContext(), clusterInfo, poolName)
	if err != nil {
		return nil, fmt.Errorf("failed to get images from pool %s: %+v", poolName, err)
	}

	for _, image := range cephImages {
		// add the current image's details to the result set
		newImage := BlockImage{
			Name:     image.Name,
			PoolName: poolName,
			Size:     image.Size,
		}
		images = append(images, newImage)
	}

	return images, nil
}

// DeleteBlockImage Function to list all the blocks created/being managed by rook
func (b *BlockOperation) DeleteBlockImage(clusterInfo *client.ClusterInfo, image BlockImage) error {
	context := b.k8sClient.MakeContext()
	return client.DeleteImage(context, clusterInfo, image.Name, image.PoolName)
}

// CreateClientPod starts a pod that should have a block PVC.
func (b *BlockOperation) CreateClientPod(manifest string) error {
	return b.k8sClient.ResourceOperation("apply", manifest)
}
