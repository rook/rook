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

func CreateK8sRookFileSystem(client contracts.ITransportClient) k8sRookFileSystem {
	return k8sRookFileSystem{transportClient: client}
}

func (rb *k8sRookFileSystem) FS_Create(name string) (string, error) {
	fileSystemCreate[4] = name
	out, err, status := rb.transportClient.Create(fileSystemCreate)
	if status == 0 {
		return out, nil
	} else {
		return err, errors.New("Unabale to create FileSystem")
	}
}

func (rb *k8sRookFileSystem) FS_Delete(name string) (string, error) {
	fileSystemCreate[4] = name
	out, err, status := rb.transportClient.Delete(fileSystemDelete)
	if status == 0 {
		return out, nil
	} else {
		return err, errors.New("Unable to delete FileSystem")
	}
}

func (rb *k8sRookFileSystem) FS_List() (string, error) {
	out, err, status := rb.transportClient.Execute(fileSystemList)
	if status == 0 {
		return out, nil
	} else {
		return err, errors.New("Unable to list FileSystem")
	}

}

func (rb *k8sRookFileSystem) FS_Mount(name string, path string) (string, error) {
	cmdArgs := []string{name}
	out, err, status := rb.transportClient.Create(cmdArgs)
	if status == 0 {
		return out, nil
	} else {
		return err, errors.New("Unable to mount FileSystem")
	}

}

func (rb *k8sRookFileSystem) FS_Write(name string, mountpath string, data string, filename string) (string, error) {
	//TODO - implement
	return "Not YET IMPLEMENTED", errors.New("NOT YET IMPLEMENTED")
}

func (rb *k8sRookFileSystem) FS_Read(name string, mountpath string, filename string) (string, error) {
	//TODO - implement
	return "Not YET IMPLEMENTED", errors.New("NOT YET IMPLEMENTED")
}

func (rb *k8sRookFileSystem) FS_Unmount(name string) (string, error) {
	cmdArgs := []string{name}
	out, err, status := rb.transportClient.Delete(cmdArgs)
	if status == 0 {
		return out, nil
	} else {
		return err, errors.New("Unable to unmount FileSystem")
	}

}
