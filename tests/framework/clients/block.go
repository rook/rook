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

var (
	writeDataToBlockPod  = []string{"sh", "-c", "WRITE_DATA_CMD"}
	readDataFromBlockPod = []string{"cat", "READ_DATA_CMD"}
)

// CreateBlockOperation - Constructor to create BlockOperation - client to perform rook Block operations on k8s
func CreateBlockOperation(k8shelp *utils.K8sHelper, manifests installer.CephManifests) *BlockOperation {
	return &BlockOperation{k8shelp, manifests}
}

// BlockCreate Function to create a Block using Rook
// Input parameters -
// manifest - pod definition that creates a pvc in k8s - yaml should describe name and size of pvc being created
// size - not user for k8s implementation since its descried on the pvc yaml definition
// Output - k8s create pvc operation output and/or error
func (b *BlockOperation) Create(manifest string, size int) (string, error) {
	args := []string{"apply", "-f", "-"}
	result, err := b.k8sClient.KubectlWithStdin(manifest, args...)
	if err != nil {
		return "", fmt.Errorf("Unable to create block -- : %s", err)

	}
	return result, nil

}

func (b *BlockOperation) CreatePvc(claimName, storageClassName, mode, size string) error {
	return b.k8sClient.ResourceOperation("apply", b.manifests.GetBlockPvcDef(claimName, storageClassName, mode, size))
}

func (b *BlockOperation) CreateStorageClass(poolName, storageClassName, reclaimPolicy, namespace string, varClusterName bool) error {
	return b.k8sClient.ResourceOperation("apply", b.manifests.GetBlockStorageClassDef(poolName, storageClassName, reclaimPolicy, namespace, varClusterName))
}

func (b *BlockOperation) DeletePvc(claimName, storageClassName, mode, size string) error {
	err := b.k8sClient.ResourceOperation("delete", b.manifests.GetBlockPvcDef(claimName, storageClassName, mode, size))
	return err
}

func (b *BlockOperation) DeleteStorageClass(poolName, storageClassName, reclaimPolicy, namespace string) error {
	err := b.k8sClient.ResourceOperation("delete", b.manifests.GetBlockStorageClassDef(poolName, storageClassName, reclaimPolicy, namespace, false))
	return err
}

// BlockDelete Function to delete a Block using Rook
// Input parameters -
// manifest - pod definition  where pvc is described - delete is run on the the yaml definition
// Output  - k8s delete pvc operation output and/or error
func (b *BlockOperation) DeleteBlock(manifest string) (string, error) {
	args := []string{"delete", "-f", "-"}
	result, err := b.k8sClient.KubectlWithStdin(manifest, args...)
	if err != nil {
		return "", fmt.Errorf("Unable to delete block -- : %s", err)

	}
	return result, nil

}

// List Function to list all the blocks created/being managed by rook
func (b *BlockOperation) List(namespace string) ([]BlockImage, error) {
	// first list all the pools so that we can retrieve images from all pools
	context := b.k8sClient.MakeContext()
	pools, err := client.ListPoolSummaries(context, namespace)
	if err != nil {
		return nil, fmt.Errorf("failed to list pools: %+v", err)
	}

	// for each pool, get further details about all the images in the pool
	images := []BlockImage{}
	for _, p := range pools {
		cephImages, err := client.ListImages(context, namespace, p.Name)
		if err != nil {
			return nil, fmt.Errorf("failed to get images from pool %s: %+v", p.Name, err)
		}

		for _, image := range cephImages {
			// add the current image's details to the result set
			newImage := BlockImage{
				Name:     image.Name,
				PoolName: p.Name,
				Size:     image.Size,
			}
			images = append(images, newImage)
		}
	}

	return images, nil
}

// DeleteBlockImage Function to list all the blocks created/being managed by rook
func (b *BlockOperation) DeleteBlockImage(image BlockImage, namespace string) error {
	context := b.k8sClient.MakeContext()
	return client.DeleteImage(context, namespace, image.Name, image.PoolName)
}

// BlockMap Function to map a Block using Rook
// Input parameters -
// manifest - Pod definition  - pod should be defined to use a pvc that was created earlier
// Output  - k8s create pod operation output and/or error
func (b *BlockOperation) BlockMap(manifest string) (string, error) {
	args := []string{"apply", "-f", "-"}
	result, err := b.k8sClient.KubectlWithStdin(manifest, args...)
	if err != nil {
		return "", fmt.Errorf("Unable to map block -- : %s", err)

	}
	return result, nil

}
