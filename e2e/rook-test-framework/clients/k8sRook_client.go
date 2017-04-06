package clients

import (
	"errors"
	"github.com/dangula/rook/e2e/rook-test-framework/contracts"
)

var (
	statusCmd  = []string{"rook", "status"}
	versioncmd = []string{"rook", "version"}
	nodecmd    = []string{"rook", "node"}
)

type k8sRookClient struct {
	transportClient  contracts.ITransportClient
	poolClient       k8sRookPool
	blockClient      k8sRookBlock
	fileSystemClient k8sRookFileSystem
	objectClient     k8sRookObject
}

func CreateRookClient(transport contracts.ITransportClient) (k8sRookClient, error) {

	return k8sRookClient{
		transportClient:  transport,
		poolClient:       CreateK8sPool(transport),
		blockClient:      CreateK8sRookBlock(transport),
		fileSystemClient: CreateK8sRookFileSystem(transport),
		objectClient:     CreateK8sRookObject(transport),
	}, nil
}

func (Client k8sRookClient) Status() (string, error) {
	out, err, status := Client.transportClient.Execute(statusCmd)
	if status == 0 {
		return out, nil
	} else {
		return err, errors.New("Unable to check rook status - please check of rook is up and running")
	}
}

func (Client k8sRookClient) Version() (string, error) {
	out, err, status := Client.transportClient.Execute(versioncmd)
	if status == 0 {
		return out, nil
	} else {
		return err, errors.New("Unable to check rook status - please check of rook is up and running")
	}
}

func (Client k8sRookClient) Node() (string, error) {
	out, err, status := Client.transportClient.Execute(nodecmd)
	if status == 0 {
		return out, nil
	} else {
		return err, errors.New("Unable to check rook status - please check of rook is up and running")
	}
}

func (Client k8sRookClient) Get_Block() k8sRookBlock {
	return Client.blockClient
}

func (Client k8sRookClient) Get_Pool() k8sRookPool {
	return Client.poolClient
}

func (Client k8sRookClient) Get_FileSystem() k8sRookFileSystem {
	return Client.fileSystemClient
}

func (Client k8sRookClient) Get_Object() k8sRookObject {
	return Client.objectClient
}
