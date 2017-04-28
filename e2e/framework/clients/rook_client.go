package clients

import (
	"fmt"
	"github.com/rook/rook/e2e/framework/contracts"
	"github.com/rook/rook/e2e/framework/enums"
	"github.com/rook/rook/e2e/framework/transport"
)

var (
	STATUS_CMD  = []string{"rook", "status"}
	VERSION_CMD = []string{"rook", "version"}
	NODE_CMD    = []string{"rook", "node"}
)

type RookClient struct {
	platform        enums.RookPlatformType
	transportClient contracts.ITransportClient
	blockClient     contracts.IRookBlock
	fsClient        contracts.IRookFilesystem
	objectClient    contracts.IRookObject
	poolClient      contracts.IRookPool
}

const (
	unable_to_check_rook_status_msg = "Unable to check rook status - please check of rook is up and running"
)

func CreateRook_Client(platform enums.RookPlatformType) (*RookClient, error) {
	var transportClient contracts.ITransportClient
	var block_client contracts.IRookBlock
	var fs_client contracts.IRookFilesystem
	var object_client contracts.IRookObject
	var pool_client contracts.IRookPool

	switch {
	case platform == enums.Kubernetes:
		transportClient = transport.CreateNewk8sTransportClient()
		block_client = CreateK8SRookBlock(transportClient)
		fs_client = CreateK8sRookFileSystem(transportClient)
		object_client = CreateK8sRookObject(transportClient)
		pool_client = CreateK8sPool(transportClient)
	case platform == enums.StandAlone:
		transportClient = nil //TODO- Not yet implemented
		block_client = nil    //TODO- Not yet implemented
		fs_client = nil       //TODO- Not yet implemented
		object_client = nil   //TODO- Not yet implemented
		pool_client = nil     //TODO- Not yet implemented
	default:
		return &RookClient{}, fmt.Errorf("Unsupported Rook Platform Type")
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
		return err, fmt.Errorf(unable_to_check_rook_status_msg)
	}
}

func (Client RookClient) Version() (string, error) {
	out, err, status := Client.transportClient.Execute(VERSION_CMD, nil)
	if status == 0 {
		return out, nil
	} else {
		return err, fmt.Errorf(unable_to_check_rook_status_msg)
	}
}

func (Client RookClient) Node() (string, error) {
	out, err, status := Client.transportClient.Execute(NODE_CMD, nil)
	if status == 0 {
		return out, nil
	} else {
		return err, fmt.Errorf(unable_to_check_rook_status_msg)
	}
}

func (Client RookClient) GetBlockClient() contracts.IRookBlock {
	return Client.blockClient
}

func (Client RookClient) GetFileSystemClient() contracts.IRookFilesystem {
	return Client.fsClient
}

func (Client RookClient) GetObjectClient() contracts.IRookObject {
	return Client.objectClient
}

func (Client RookClient) GetPoolClient() contracts.IRookPool {
	return Client.poolClient
}
