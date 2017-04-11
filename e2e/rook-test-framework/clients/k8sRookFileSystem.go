package clients

import (
	"errors"
	"github.com/dangula/rook/e2e/rook-test-framework/contracts"
)

type k8sRookFileSystem struct {
	transportClient contracts.ITransportClient
}

var (
	fileSystemCreate    = []string{"rook", "filesystem", "create", "--name", "NAME"}
	fileSystemDelete    = []string{"rook", "filesystem", "delete", "--name", "NAME"}
	fileSystemList      = []string{"rook", "filesystem", "ls"}
	wideDataToFilePod   = []string{"sh", "-c", "WRITE_DATA_CMD"}
	readDataFromFilePod = []string{"cat", "READ_DATA_CMD"}
)

// Constructor to create k8sRookFileSystem - client to perform rook file system operations on k8s
func CreateK8sRookFileSystem(client contracts.ITransportClient) *k8sRookFileSystem {
	return &k8sRookFileSystem{transportClient: client}
}

//Function to create a fileSystem in rook
//Input paramatres -
// name -  name of the shared file system to be created
//Output - output returned by rook cli and/or error
func (rfs *k8sRookFileSystem) FS_Create(name string) (string, error) {
	fileSystemCreate[4] = name
	out, err, status := rfs.transportClient.Execute(fileSystemCreate, nil)
	if status == 0 {
		return out, nil
	} else {
		return err, errors.New("Unabale to create FileSystem")
	}
}

//Function to delete a fileSystem in rook
//Input paramatres -
// name -  name of the shared file system to be deleted
//Output - output returned by rook cli and/or error
func (rfs *k8sRookFileSystem) FS_Delete(name string) (string, error) {
	fileSystemDelete[4] = name
	out, err, status := rfs.transportClient.Execute(fileSystemDelete, nil)
	if status == 0 {
		return out, nil
	} else {
		return err, errors.New("Unable to delete FileSystem")
	}
}

//Function to list a fileSystem in rook
//Output - output returned by rook cli and/or error
func (rfs *k8sRookFileSystem) FS_List() (string, error) {
	out, err, status := rfs.transportClient.Execute(fileSystemList, nil)
	if status == 0 {
		return out, nil
	} else {
		return err, errors.New("Unable to list FileSystem")
	}

}

//Function to Mount a file system created by rook(on a pod)
//Input paramaters -
//name - path to the yaml defintion file - definition of pod to be created that mounts existing file system
//path - ignored in this case - moount path is defined in the path definition
//output - output returned by k8s create pod operaton and/or error
func (rfs *k8sRookFileSystem) FS_Mount(name string, path string) (string, error) {
	cmdArgs := []string{name}
	out, err, status := rfs.transportClient.Create(cmdArgs, nil)
	if status == 0 {
		return out, nil
	} else {
		return err, errors.New("Unable to mount FileSystem")
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
func (rfs *k8sRookFileSystem) FS_Write(name string, mountpath string, data string, filename string, namespace string) (string, error) {
	wt := "echo \"" + data + "\">" + mountpath + "/" + filename
	wideDataToFilePod[2] = wt
	option := []string{name}
	if namespace != "" {
		option = append(option, namespace)
	}
	out, err, status := rfs.transportClient.Execute(wideDataToFilePod, option)
	if status == 0 {
		return out, nil
	} else {
		return err, errors.New("Unable to write data to pod")
	}
}

// Function to write  read from file system  created by rook ,i.e. Read data from a pod that filesystem mounted
// Input paramaters -
//name - path to a yaml file that creates a pod  - pod should be defined to use a pvc that was created earlier
//mountpath - folder on the pod were data is supposed to be written(should match the mountpath described in the pod definition)
//filename - file to be read
//namespace - optional param - namespace of the pod
//Output  - k8s exec pod operation output and/or error
func (rfs *k8sRookFileSystem) FS_Read(name string, mountpath string, filename string, namespace string) (string, error) {
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
		return err, errors.New("Unable to write data to pod")
	}
}

//Function to UnMount a file system created by rook(delete pod)
//Input paramaters -
//name - path to the yaml defintion file - definition of pod to be deleted that has a file system mounted
//path - ignored in this case - moount path is defined in the path definition
//output - output returned by k8s delete pod operaton and/or error
func (rfs *k8sRookFileSystem) FS_Unmount(name string) (string, error) {
	cmdArgs := []string{name}
	out, err, status := rfs.transportClient.Delete(cmdArgs, nil)
	if status == 0 {
		return out, nil
	} else {
		return err, errors.New("Unable to unmount FileSystem")
	}

}
