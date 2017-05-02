package clients

import (
	"fmt"
	"github.com/rook/rook/e2e/framework/contracts"
)

type k8sRookBlock struct {
	transportClient contracts.ITransportClient
}

var (
	listCmd              = []string{"rook", "block", "ls"}
	writeDataToBlockPod  = []string{"sh", "-c", "WRITE_DATA_CMD"}
	readDataFromBlockPod = []string{"cat", "READ_DATA_CMD"}
)

// Constructor to create k8sRookBlock - client to perform rook Block operations on k8s
func CreateK8SRookBlock(client contracts.ITransportClient) *k8sRookBlock {
	return &k8sRookBlock{transportClient: client}
}

// Function to create a Block using Rook
// Input paramaters -
//name - path to a yaml file that creates a pvc in k8s - yaml should describe name and size of pvc being created
//size - not user for k8s implementation since its descried on the pvc yaml definition
//Output - k8s create pvc operation output and/or error
func (rb *k8sRookBlock) BlockCreate(name string, size int) (string, error) {
	cmdArgs := []string{name}
	out, err, status := rb.transportClient.Create(cmdArgs, nil)
	if status == 0 {
		return out, nil
	} else {
		return err, fmt.Errorf("Unable to create block -- : %s", err)
	}
}

// Function to delete a Block using Rook
// Input paramaters -
//name - path to a yaml file that where pvc is desirbed - delete is run on the the yaml definition
//Output  - k8s delete pvc operation output and/or error
func (rb *k8sRookBlock) BlockDelete(name string) (string, error) {
	cmdArgs := []string{name}
	out, err, status := rb.transportClient.Delete(cmdArgs, nil)
	if status == 0 {
		return out, nil
	} else {
		return err, fmt.Errorf("Unable to delete block -- : %s", err)
	}
}

// Function to list all the blocks created/being managed by rook
//Returns a ouput  for rook cli for block list
func (rb *k8sRookBlock) BlockList() (string, error) {
	out, err, status := rb.transportClient.Execute(listCmd, nil)
	if status == 0 {
		return out, nil
	} else {
		return err, fmt.Errorf("Unable to list all blocks -- : %s", err)
	}
}

// Function to map a Block using Rook
// Input paramaters -
//name - path to a yaml file that creates a pod  - pod should be defined to use a pvc that was created earlier
//mountpath - not used in this impl since mountpath is defined in the pod definition
//Output  - k8s create pod operation output and/or error
func (rb *k8sRookBlock) BlockMap(name string, mountpath string) (string, error) {
	cmdArgs := []string{name}
	out, err, status := rb.transportClient.Create(cmdArgs, nil)
	if status == 0 {
		return out, nil
	} else {
		return err, fmt.Errorf("Unable to Map block -- : %s", err)
	}

}

// Function to write  data to block created by rook ,i.e. write data to a pod that is using a pvc
// Input paramaters -
//name - path to a yaml file that creates a pod  - pod should be defined to use a pvc that was created earlier
//mountpath - folder on the pod were data is supposed to be written(should match the mountpath described in the pod definition)
//data - data to be written
//filename - file where data is written to
//namespace - optional param - namespace of the pod
//Output  - k8s exec pod operation output and/or error
func (rb *k8sRookBlock) BlockWrite(name string, mountpath string, data string, filename string, namespace string) (string, error) {
	wt := "echo \"" + data + "\">" + mountpath + "/" + filename
	writeDataToBlockPod[2] = wt
	option := []string{name}
	if namespace != "" {
		option = append(option, namespace)
	}
	out, err, status := rb.transportClient.Execute(writeDataToBlockPod, option)
	if status == 0 {
		return out, nil
	} else {
		return err, fmt.Errorf("Unable to write data to pod: %s --> %s", err, out)
	}
}

// Function to read from block created by rook ,i.e. Read data from a pod that is using a pvc
// Input paramaters -
//name - path to a yaml file that creates a pod  - pod should be defined to use a pvc that was created earlier
//mountpath - folder on the pod were data is supposed to be written(should match the mountpath described in the pod definition)
//filename - file to be read
//namespace - optional param - namespace of the pod
//Output  - k8s exec pod operation output and/or error
func (rb *k8sRookBlock) BlockRead(name string, mountpath string, filename string, namespace string) (string, error) {
	rd := mountpath + "/" + filename
	readDataFromBlockPod[1] = rd
	option := []string{name}
	if namespace != "" {
		option = append(option, namespace)
	}
	out, err, status := rb.transportClient.Execute(readDataFromBlockPod, option)
	if status == 0 {
		return out, nil
	} else {
		return err, fmt.Errorf("Unable to read data to pod -- : %s", err)
	}
}

// Function to map a Block using Rook
// Input paramaters -
//name - path to a yaml file - the pod described in yam file is deleted
//mountpath - not used in this impl since mountpath is defined in the pod definition
//Output  - k8s delete pod operation output and/or error
func (rb *k8sRookBlock) BlockUnmap(name string, mountpath string) (string, error) {
	cmdArgs := []string{name}
	out, err, status := rb.transportClient.Delete(cmdArgs, nil)
	if status == 0 {
		return out, nil
	} else {
		return err, fmt.Errorf("Unable to unmap block -- : %s", err)
	}

}
