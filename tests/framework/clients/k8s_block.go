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
)

//BlockOperation is wrapper for k8s rook block operations
type BlockOperation struct {
	transportClient contracts.ITransportClient
	restClient      contracts.RestAPIOperator
}

var (
	writeDataToBlockPod  = []string{"sh", "-c", "WRITE_DATA_CMD"}
	readDataFromBlockPod = []string{"cat", "READ_DATA_CMD"}
)

// CreateK8BlockOperation - Constructor to create BlockOperation - client to perform rook Block operations on k8s
func CreateK8BlockOperation(client contracts.ITransportClient, rookRestClient contracts.RestAPIOperator) *BlockOperation {
	return &BlockOperation{transportClient: client, restClient: rookRestClient}
}

// BlockCreate Function to create a Block using Rook
// Input paramaters -
//name - path to a yaml file that creates a pvc in k8s - yaml should describe name and size of pvc being created
//size - not user for k8s implementation since its descried on the pvc yaml definition
//Output - k8s create pvc operation output and/or error
func (rb *BlockOperation) BlockCreate(name string, size int) (string, error) {
	cmdArgs := []string{name}
	out, err, status := rb.transportClient.Create(cmdArgs, nil)
	if status == 0 {
		return out, nil
	}
	return err, fmt.Errorf("Unable to create block -- : %s", err)

}

// BlockDelete Function to delete a Block using Rook
// Input paramaters -
//name - path to a yaml file that where pvc is described - delete is run on the the yaml definition
//Output  - k8s delete pvc operation output and/or error
func (rb *BlockOperation) BlockDelete(name string) (string, error) {
	cmdArgs := []string{name}
	out, err, status := rb.transportClient.Delete(cmdArgs, nil)
	if status == 0 {
		return out, nil
	}
	return err, fmt.Errorf("Unable to delete block -- : %s", err)

}

// BlockList Function to list all the blocks created/being managed by rook
func (rb *BlockOperation) BlockList() ([]model.BlockImage, error) {
	return rb.restClient.GetBlockImages()

}

// BlockMap Function to map a Block using Rook
// Input paramaters -
//name - path to a yaml file that creates a pod  - pod should be defined to use a pvc that was created earlier
//mountpath - not used in this impl since mountpath is defined in the pod definition
//Output  - k8s create pod operation output and/or error
func (rb *BlockOperation) BlockMap(name string, mountpath string) (string, error) {
	cmdArgs := []string{name}
	out, err, status := rb.transportClient.Create(cmdArgs, nil)
	if status == 0 {
		return out, nil
	}
	return err, fmt.Errorf("Unable to Map block -- : %s", err)

}

//BlockWrite Function to write  data to block created by rook ,i.e. write data to a pod that is using a pvc
// Input paramaters -
//name - path to a yaml file that creates a pod  - pod should be defined to use a pvc that was created earlier
//mountpath - folder on the pod were data is supposed to be written(should match the mountpath described in the pod definition)
//data - data to be written
//filename - file where data is written to
//namespace - optional param - namespace of the pod
//Output  - k8s exec pod operation output and/or error
func (rb *BlockOperation) BlockWrite(name string, mountpath string, data string, filename string, namespace string) (string, error) {
	wt := "echo \"" + data + "\">" + mountpath + "/" + filename
	writeDataToBlockPod[2] = wt
	option := []string{name}
	if namespace != "" {
		option = append(option, namespace)
	}
	out, err, status := rb.transportClient.Execute(writeDataToBlockPod, option)
	if status == 0 {
		return out, nil
	}
	return err, fmt.Errorf("Unable to write data to pod: %s --> %s", err, out)

}

// BlockRead Function to read from block created by rook ,i.e. Read data from a pod that is using a pvc
// Input paramaters -
//name - path to a yaml file that creates a pod  - pod should be defined to use a pvc that was created earlier
//mountpath - folder on the pod were data is supposed to be written(should match the mountpath described in the pod definition)
//filename - file to be read
//namespace - optional param - namespace of the pod
//Output  - k8s exec pod operation output and/or error
func (rb *BlockOperation) BlockRead(name string, mountpath string, filename string, namespace string) (string, error) {
	rd := mountpath + "/" + filename
	readDataFromBlockPod[1] = rd
	option := []string{name}
	if namespace != "" {
		option = append(option, namespace)
	}
	out, err, status := rb.transportClient.Execute(readDataFromBlockPod, option)
	if status == 0 {
		return out, nil
	}
	return err, fmt.Errorf("Unable to read data to pod -- : %s", err)

}

// BlockUnmap Function to map a Block using Rook
// Input paramaters -
//name - path to a yaml file - the pod described in yam file is deleted
//mountpath - not used in this impl since mountpath is defined in the pod definition
//Output  - k8s delete pod operation output and/or error
func (rb *BlockOperation) BlockUnmap(name string, mountpath string) (string, error) {
	cmdArgs := []string{name}
	out, err, status := rb.transportClient.Delete(cmdArgs, nil)
	if status == 0 {
		return out, nil
	}
	return err, fmt.Errorf("Unable to unmap block -- : %s", err)

}
