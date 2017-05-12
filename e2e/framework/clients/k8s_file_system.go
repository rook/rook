package clients

import (
	"fmt"
	"github.com/rook/rook/e2e/framework/contracts"
	"github.com/rook/rook/pkg/model"
)

type fileSystemOperation struct {
	transportClient contracts.ITransportClient
	restClient      contracts.RestAPIOperator
}

var (
	writeDataToFilePod  = []string{"sh", "-c", "WRITE_DATA_CMD"}
	readDataFromFilePod = []string{"cat", "READ_DATA_CMD"}
)

// Constructor to create fileSystemOperation - client to perform rook file system operations on k8s
func CreateK8sFileSystemOperation(client contracts.ITransportClient, rookRestClient contracts.RestAPIOperator) *fileSystemOperation {
	return &fileSystemOperation{transportClient: client, restClient: rookRestClient}
}

//Function to create a fileSystem in rook
//Input paramatres -
// name -  name of the shared file system to be created
//Output - output returned by rook rest API client
func (rfs *fileSystemOperation) FSCreate(name string) (string, error) {
	createFilesystem := model.FilesystemRequest{Name: name}
	return rfs.restClient.CreateFilesystem(createFilesystem)
}

//Function to delete a fileSystem in rook
//Input paramatres -
// name -  name of the shared file system to be deleted
//Output - output returned by rook rest API client
func (rfs *fileSystemOperation) FSDelete(name string) (string, error) {
	deleteFilesystem := model.FilesystemRequest{Name: name}
	return rfs.restClient.DeleteFilesystem(deleteFilesystem)
}

//Function to list a fileSystem in rook
//Output - output returned by rook rest API client
func (rfs *fileSystemOperation) FSList() ([]model.Filesystem, error) {
	return rfs.restClient.GetFilesystems()

}

//Function to Mount a file system created by rook(on a pod)
//Input paramaters -
//name - path to the yaml defintion file - definition of pod to be created that mounts existing file system
//path - ignored in this case - moount path is defined in the path definition
//output - output returned by k8s create pod operaton and/or error
func (rfs *fileSystemOperation) FSMount(name string, path string) (string, error) {
	cmdArgs := []string{name}
	out, err, status := rfs.transportClient.Create(cmdArgs, nil)
	if status == 0 {
		return out, nil
	} else {
		return err, fmt.Errorf("Unable to mount FileSystem -- : %s", err)
	}

}

// Function to write  data to file system created by rook ,i.e. write data to a pod that has filesystem mounted
// Input paramaters -
//name - path to a yaml file that creates a pod  - pod should be defined to use a pvc that was created earlier
//mountpath - folder on the pod were data is supposed to be written(should match the mountpath described in the pod definition)
//data - data to be written
//filename - file where data is written to
//namespace - optional param - namespace of the pod
//Output  - k8s exec pod operation output and/or error
func (rfs *fileSystemOperation) FSWrite(name string, mountpath string, data string, filename string, namespace string) (string, error) {
	wt := "echo \"" + data + "\">" + mountpath + "/" + filename
	writeDataToFilePod[2] = wt
	option := []string{name}
	if namespace != "" {
		option = append(option, namespace)
	}
	out, err, status := rfs.transportClient.Execute(writeDataToFilePod, option)
	if status == 0 {
		return out, nil
	} else {
		return err, fmt.Errorf("Unable to write data to pod -- : %s", err)
	}
}

// Function to write  read from file system  created by rook ,i.e. Read data from a pod that filesystem mounted
// Input paramaters -
//name - path to a yaml file that creates a pod  - pod should be defined to use a pvc that was created earlier
//mountpath - folder on the pod were data is supposed to be written(should match the mountpath described in the pod definition)
//filename - file to be read
//namespace - optional param - namespace of the pod
//Output  - k8s exec pod operation output and/or error
func (rfs *fileSystemOperation) FSRead(name string, mountpath string, filename string, namespace string) (string, error) {
	rd := mountpath + "/" + filename
	readDataFromFilePod[1] = rd
	option := []string{name}
	if namespace != "" {
		option = append(option, namespace)
	}
	out, err, status := rfs.transportClient.Execute(readDataFromFilePod, option)
	if status == 0 {
		return out, nil
	} else {
		return err, fmt.Errorf("Unable to write data to pod -- : %s", err)
	}
}

//Function to UnMount a file system created by rook(delete pod)
//Input paramaters -
//name - path to the yaml defintion file - definition of pod to be deleted that has a file system mounted
//path - ignored in this case - moount path is defined in the path definition
//output - output returned by k8s delete pod operaton and/or error
func (rfs *fileSystemOperation) FSUnmount(name string) (string, error) {
	cmdArgs := []string{name}
	out, err, status := rfs.transportClient.Delete(cmdArgs, nil)
	if status == 0 {
		return out, nil
	} else {
		return err, fmt.Errorf("Unable to unmount FileSystem -- : %s", err)
	}

}
