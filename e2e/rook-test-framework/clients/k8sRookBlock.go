package clients

import (
	"errors"
	"github.com/dangula/rook/e2e/rook-test-framework/contracts"
)

type k8sRookBlock struct {
	transportClient contracts.ITransportClient
}

var (
	listCmd         = []string{"rook", "block", "list"}
	wideDataToPod   = []string{"bash", "-c", "WRITE_DATA_CMD"}
	readDataFromPod = []string{"cat", "READ_DATA_CMD"}
)

// Constructor to create k8sRookBlock - client to perform rook Block operations on k8s
func CreateK8sRookBlock(client contracts.ITransportClient) *k8sRookBlock {
	return &k8sRookBlock{transportClient: client}
}

// Function to create a Block using Rook
// Input paramaters -
//name - path to a yaml file that creates a pvc in k8s - yaml should describe name and size of pvc being created
//size - not user for k8s implementation since its descried on the pvc yaml definition
//Output - k8s create pvc operation output and/or error
func (rb *k8sRookBlock) Block_Create(name string, size int) (string, error) {
	cmdArgs := []string{name}
	out, err, status := rb.transportClient.Create(cmdArgs, nil)
	if status == 0 {
		return out, nil
	} else {
		return err, errors.New("Unabale to create block")
	}
}

// Function to delete a Block using Rook
// Input paramaters -
//name - path to a yaml file that where pvc is desirbed - delete is run on the the yaml definition
//Output  - k8s delete pvc operation output and/or error
func (rb *k8sRookBlock) Block_Delete(name string) (string, error) {
	cmdArgs := []string{name}
	out, err, status := rb.transportClient.Delete(cmdArgs, nil)
	if status == 0 {
		return out, nil
	} else {
		return err, errors.New("Unable to delete Block")
	}
}

// Function to list all the blocks created/being managed by rook
//Returns a ouput  for rook cli for block list
func (rb *k8sRookBlock) Block_List() (string, error) {
	out, err, status := rb.transportClient.Execute(listCmd, nil)
	if status == 0 {
		return out, nil
	} else {
		return err, errors.New("Unable to list all blocks")
	}
}

// Function to map a Block using Rook
// Input paramaters -
//name - path to a yaml file that creates a pod  - pod should be defined to use a pvc that was created earlier
//mountpath - not used in this impl since mountpath is defined in the pod definition
//Output  - k8s create pod operation output and/or error
func (rb *k8sRookBlock) Block_Map(name string, mountpath string) (string, error) {
	cmdArgs := []string{name}
	out, err, status := rb.transportClient.Create(cmdArgs, nil)
	if status == 0 {
		return out, nil
	} else {
		return err, errors.New("Unable to Map block")
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
func (rb *k8sRookBlock) Block_Write(name string, mountpath string, data string, filename string, namespace string) (string, error) {
	wt := "echo \"" + data + "\">" + mountpath + "/" + filename
	wideDataToPod[2] = wt
	option := []string{name}
	if namespace != "" {
		option = append(option, namespace)
	}
	out, err, status := rb.transportClient.Execute(wideDataToPod, option)
	if status == 0 {
		return out, nil
	} else {
		return err, errors.New("Unable to write data to pod")
	}
}

// Function to write  read from block created by rook ,i.e. Read data from a pod that is using a pvc
// Input paramaters -
//name - path to a yaml file that creates a pod  - pod should be defined to use a pvc that was created earlier
//mountpath - folder on the pod were data is supposed to be written(should match the mountpath described in the pod definition)
//filename - file to be read
//namespace - optional param - namespace of the pod
//Output  - k8s exec pod operation output and/or error
func (rb *k8sRookBlock) Block_Read(name string, mountpath string, filename string, namespace string) (string, error) {
	rd := mountpath + "/" + filename
	readDataFromPod[1] = rd
	option := []string{name}
	if namespace != "" {
		option = append(option, namespace)
	}
	out, err, status := rb.transportClient.Execute(readDataFromPod, option)
	if status == 0 {
		return out, nil
	} else {
		return err, errors.New("Unable to write data to pod")
	}
}

// Function to map a Block using Rook
// Input paramaters -
//name - path to a yaml file - the pod described in yam file is deleted
//mountpath - not used in this impl since mountpath is defined in the pod definition
//Output  - k8s delete pod operation output and/or error
func (rb *k8sRookBlock) Block_Unmap(name string, mountpath string) (string, error) {
	cmdArgs := []string{name}
	out, err, status := rb.transportClient.Delete(cmdArgs, nil)
	if status == 0 {
		return out, nil
	} else {
		return err, errors.New("Unable to unmap block")
	}

}
