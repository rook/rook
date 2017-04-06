package clients

import (
	"errors"
	"github.com/dangula/rook/e2e/rook-test-framework/contracts"
)

type k8sRookBlock struct {
	transportClient contracts.ITransportClient
}

var (
	listCmd = []string{"rook", "block", "list"}
)

func CreateK8sRookBlock(client contracts.ITransportClient) *k8sRookBlock {
	return &k8sRookBlock{transportClient: client}
}

func (rb *k8sRookBlock) Block_Create(name string, size int) (string, error) {
	cmdArgs := []string{name}
	out, err, status := rb.transportClient.Create(cmdArgs)
	if status == 0 {
		return out, nil
	} else {
		return err, errors.New("Unabale to create block")
	}
}

func (rb *k8sRookBlock) Block_Delete(name string) (string, error) {
	cmdArgs := []string{name}
	out, err, status := rb.transportClient.Delete(cmdArgs)
	if status == 0 {
		return out, nil
	} else {
		return err, errors.New("Unable to delete Block")
	}
}

func (rb *k8sRookBlock) Block_List() (string, error) {
	out, err, status := rb.transportClient.Execute(listCmd)
	if status == 0 {
		return out, nil
	} else {
		return err, errors.New("Unable to list all blocks")
	}
}

func (rb *k8sRookBlock) Block_Map(name string, mountpath string) (string, error) {
	cmdArgs := []string{name}
	out, err, status := rb.transportClient.Create(cmdArgs)
	if status == 0 {
		return out, nil
	} else {
		return err, errors.New("Unable to Map block")
	}

}

func (rb *k8sRookBlock) Block_Write(name string, mountpath string, data string, filename string) (string, error) {
	//TODO - implement
	return "Not YET IMPLEMENTED", errors.New("NOT YET IMPLEMENTED")
}

func (rb *k8sRookBlock) Block_Read(name string, mountpath string, filename string) (string, error) {
	//TODO - implement
	return "Not YET IMPLEMENTED", errors.New("NOT YET IMPLEMENTED")
}

func (rb *k8sRookBlock) Block_Unmap(name string, mountpath string) (string, error) {
	cmdArgs := []string{name}
	out, err, status := rb.transportClient.Delete(cmdArgs)
	if status == 0 {
		return out, nil
	} else {
		return err, errors.New("Unable to unmap block")
	}

}
