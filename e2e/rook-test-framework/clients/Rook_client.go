package clients

import (
	"errors"
	"github.com/dangula/rook/e2e/rook-test-framework/contracts"
	"github.com/dangula/rook/e2e/rook-test-framework/enums"
	"github.com/dangula/rook/e2e/rook-test-framework/transport"
)

var (
	STATUS_CMD  = []string{"rook", "status"}
	VERSION_CMD = []string{"rook", "version"}
	NODE_CMD    = []string{"rook", "node"}
)

type rookClient struct {
	platform        enums.RookPlatformType
	transportClient contracts.ITransportClient
	block_client    contracts.Irook_block
	fs_client       contracts.Irook_filesystem
	object_client   contracts.Irook_object
	pool_client     contracts.Irook_pool
}

func CreateRook_Client(platform enums.RookPlatformType) (rookClient, error) {
	var transportClient contracts.ITransportClient

	switch {
	case platform == enums.Kubernetes:
		transportClient = transport.CreateNewk8sTransportClient()
	case platform == enums.StandAlone:
		transportClient = transport.CreateNewStandAloneTransportClient()
	default:
		return nil, errors.New("Unsupported Rook Platform Type")
	}

	return rookClient{
		platform,
		transportClient,
		CreateK8sRookBlock(transportClient),		//TODO the name of this says K8s, why then do we need to pass a transport, it should know it
		CreateK8sRookFileSystem(transportClient),
		CreateK8sRookObject(transportClient),
		CreateK8sPool(transportClient),
	}, nil


}

func (Client rookClient) Status() (string, error) {
	out, err, status := Client.transportClient.Execute(STATUS_CMD)
	if status == 0 {
		return out, nil
	} else {
		return err, errors.New("Unable to check rook status - please check of rook is up and running")
	}
}

func (Client rookClient) Version() (string, error) {
	out, err, status := Client.transportClient.Execute(VERSION_CMD)
	if status == 0 {
		return out, nil
	} else {
		return err, errors.New("Unable to check rook status - please check of rook is up and running")
	}
}

func (Client rookClient) Node() (string, error) {
	out, err, status := Client.transportClient.Execute(NODE_CMD)
	if status == 0 {
		return out, nil
	} else {
		return err, errors.New("Unable to check rook status - please check of rook is up and running")
	}
}

func (Client rookClient) Get_Block_client() contracts.Irook_block {
	return Client.block_client
}

func (Client rookClient) Get_FileSystem_client() contracts.Irook_filesystem {
	return Client.fs_client
}

func (Client rookClient) Get_Object_client() contracts.Irook_object {
	return Client.object_client
}

func (Client rookClient) Get_Pool_client() contracts.Irook_pool {
	return Client.pool_client
}
