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
	"github.com/rook/rook/tests/framework/utils"
)

//BlockOperation is wrapper for k8s rook block operations
type BlockOperation struct {
	k8sClient *utils.K8sHelper
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

// CreateK8BlockOperation - Constructor to create BlockOperation - client to perform rook Block operations on k8s
func CreateK8BlockOperation(k8shelp *utils.K8sHelper) *BlockOperation {
	return &BlockOperation{k8sClient: k8shelp}
}

// BlockCreate Function to create a Block using Rook
// Input parameters -
// manifest - pod definition that creates a pvc in k8s - yaml should describe name and size of pvc being created
// size - not user for k8s implementation since its descried on the pvc yaml definition
// Output - k8s create pvc operation output and/or error
func (b *BlockOperation) Create(manifest string, size int) (string, error) {
	args := []string{"create", "-f", "-"}
	result, err := b.k8sClient.KubectlWithStdin(manifest, args...)
	if err != nil {
		return "", fmt.Errorf("Unable to create block -- : %s", err)

	}
	return result, nil

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
// mountpath - not used in this impl since mountpath is defined in the pod definition
// Output  - k8s create pod operation output and/or error
func (b *BlockOperation) BlockMap(manifest string, mountpath string) (string, error) {
	args := []string{"create", "-f", "-"}
	result, err := b.k8sClient.KubectlWithStdin(manifest, args...)
	if err != nil {
		return "", fmt.Errorf("Unable to map block -- : %s", err)

	}
	return result, nil

}

// Write Function to write  data to block created by rook ,i.e. write data to a pod that is using a pvc
// Input parameters -
// manifest - path to a yaml file that creates a pod  - pod should be defined to use a pvc that was created earlier
// mountpath - folder on the pod were data is supposed to be written(should match the mountpath described in the pod definition)
// data - data to be written
// filename - file where data is written to
// namespace - optional param - namespace of the pod
// Output  - k8s exec pod operation output and/or error
func (b *BlockOperation) Write(name string, mountpath string, data string, filename string, namespace string) (string, error) {
	wt := "echo \"" + data + "\">" + mountpath + "/" + filename
	args := []string{"exec", name}

	if namespace != "" {
		args = append(args, "-n", namespace)
	}
	args = append(args, "--", "sh", "-c", wt)

	result, err := b.k8sClient.Kubectl(args...)
	if err != nil {
		return "", fmt.Errorf("Unable to write data to pod --: %s", err)

	}
	return result, nil

}

// Read Function to read from block created by rook ,i.e. Read data from a pod that is using a pvc
// Input parameters -
// manifest - path to a yaml file that creates a pod  - pod should be defined to use a pvc that was created earlier
// mountpath - folder on the pod were data is supposed to be written(should match the mountpath described in the pod definition)
// filename - file to be read
// namespace - optional param - namespace of the pod
// Output  - k8s exec pod operation output and/or error
func (b *BlockOperation) Read(name string, mountpath string, filename string, namespace string) (string, error) {
	rd := mountpath + "/" + filename
	args := []string{"exec", name}

	if namespace != "" {
		args = append(args, "-n", namespace)
	}
	args = append(args, "--", "cat", rd)

	result, err := b.k8sClient.Kubectl(args...)
	if err != nil {
		return "", fmt.Errorf("Unable to read data to pod -- : %s", err)

	}
	return result, nil

}

// BlockUnmap Function to unmap a Block using Rook
// Input parameters -
// manifest - pod definition - the pod described in yaml file is deleted
// mountpath - not used in this impl since mountpath is defined in the pod definition
// Output  - k8s delete pod operation output and/or error
func (b *BlockOperation) BlockUnmap(name string, mountpath string) (string, error) {
	args := []string{"delete", "-f", "-"}
	result, err := b.k8sClient.KubectlWithStdin(name, args...)
	if err != nil {
		return "", fmt.Errorf("Unable to unmap block -- : %s", err)

	}
	return result, nil

}
