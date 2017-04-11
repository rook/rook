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

type RookClient struct {
	platform        enums.RookPlatformType
	transportClient contracts.ITransportClient
	block_client    contracts.Irook_block
	fs_client       contracts.Irook_filesystem
	object_client   contracts.Irook_object
	pool_client     contracts.Irook_pool
}

func CreateRook_Client(platform enums.RookPlatformType) (*RookClient, error) {
	var transportClient contracts.ITransportClient
	var block_client contracts.Irook_block
	var fs_client contracts.Irook_filesystem
	var object_client contracts.Irook_object
	var pool_client contracts.Irook_pool

	switch {
	case platform == enums.Kubernetes:
		transportClient = transport.CreateNewk8sTransportClient()
		block_client = CreateK8sRookBlock(transportClient)
		fs_client = CreateK8sRookFileSystem(transportClient)
		object_client = CreateK8sRookObject(transportClient)
		pool_client = CreateK8sPool(transportClient)
	case platform == enums.StandAlone:
		transportClient = transport.CreateNewStandAloneTransportClient()
		block_client = nil  //TODO- Not yet implemented
		fs_client = nil     //TODO- Not yet implemented
		object_client = nil //TODO- Not yet implemented
		pool_client = nil   //TODO- Not yet implemented
	default:
		return &RookClient{}, errors.New("Unsupported Rook Platform Type")
	}

	return &RookClient{
		platform,
		transportClient,
		block_client,
		fs_client,
		object_client,
		pool_client,
	}, nil

}

func (Client RookClient) Status() (string, error) {
	out, err, status := Client.transportClient.Execute(STATUS_CMD, nil)
	if status == 0 {
		return out, nil
	} else {
		return err, errors.New("Unable to check rook status - please check of rook is up and running")
	}
}

func (Client RookClient) Version() (string, error) {
	out, err, status := Client.transportClient.Execute(VERSION_CMD, nil)
	if status == 0 {
		return out, nil
	} else {
		return err, errors.New("Unable to check rook status - please check of rook is up and running")
	}
}

func (Client RookClient) Node() (string, error) {
	out, err, status := Client.transportClient.Execute(NODE_CMD, nil)
	if status == 0 {
		return out, nil
	} else {
		return err, errors.New("Unable to check rook status - please check of rook is up and running")
	}
}

func (Client RookClient) Get_Block_client() contracts.Irook_block {
	return Client.block_client
}

func (Client RookClient) Get_FileSystem_client() contracts.Irook_filesystem {
	return Client.fs_client
}

func (Client RookClient) Get_Object_client() contracts.Irook_object {
	return Client.object_client
}

func (Client RookClient) Get_Pool_client() contracts.Irook_pool {
	return Client.pool_client
}
