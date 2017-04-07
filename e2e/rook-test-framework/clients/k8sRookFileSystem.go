package clients

import (
	"errors"
	"github.com/dangula/rook/e2e/rook-test-framework/contracts"
)

type k8sRookFileSystem struct {
	transportClient contracts.ITransportClient
}

var (
	fileSystemCreate = []string{"rook", "filesystem", "create", "--name", "NAME"}
	fileSystemDelete = []string{"rook", "filesystem", "delete", "--name", "NAME"}
	fileSystemList   = []string{"rook", "filesystem", "ls"}
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
	out, err, status := rfs.transportClient.Create(fileSystemCreate, nil)
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
	fileSystemCreate[4] = name
	out, err, status := rfs.transportClient.Delete(fileSystemDelete, nil)
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

func (rfs *k8sRookFileSystem) FS_Mount(name string, path string) (string, error) {
	cmdArgs := []string{name}
	out, err, status := rfs.transportClient.Create(cmdArgs, nil)
	if status == 0 {
		return out, nil
	} else {
		return err, errors.New("Unable to mount FileSystem")
	}

}

func (rfs *k8sRookFileSystem) FS_Write(name string, mountpath string, data string, filename string, namespace string) (string, error) {
	//TODO - implement
	return "Not YET IMPLEMENTED", errors.New("NOT YET IMPLEMENTED")
}

func (rfs *k8sRookFileSystem) FS_Read(name string, mountpath string, filename string, namespace string) (string, error) {
	//TODO - implement
	return "Not YET IMPLEMENTED", errors.New("NOT YET IMPLEMENTED")
}

func (rfs *k8sRookFileSystem) FS_Unmount(name string) (string, error) {
	cmdArgs := []string{name}
	out, err, status := rfs.transportClient.Delete(cmdArgs, nil)
	if status == 0 {
		return out, nil
	} else {
		return err, errors.New("Unable to unmount FileSystem")
	}

}
