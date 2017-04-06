package clients

import (
	"errors"
	"github.com/dangula/rook/e2e/rook-test-framework/contracts"
)

var (
	STATUS_CMD  = []string{"rook", "status"}
	VERSION_CMD = []string{"rook", "version"}
	NODE_CMD    = []string{"rook", "node"}
)

type RookClient struct {
	platform string
	transportClient  contracts.ITransportClient
	block_client contracts.Irook_block
	fs_client contracts.Irook_filesystem
	object_client contracts.Irook_object
	pool_client contracts.Irook_pool
}

func CreateRook_Client(platformval string,transport contracts.ITransportClient) (RookClient, error) {

	return RookClient{
		 platformval,
		 transport,
		CreateK8sRookBlock(transport),
		CreateK8sRookFileSystem(transport),
		nil,
		nil,


	}, nil
}

func (Client RookClient) Status() (string, error) {
	out, err, status := Client.transportClient.Execute(STATUS_CMD)
	if status == 0 {
		return out, nil
	} else {
		return err, errors.New("Unable to check rook status - please check of rook is up and running")
	}
}

func (Client RookClient) Version() (string, error) {
	out, err, status := Client.transportClient.Execute(VERSION_CMD)
	if status == 0 {
		return out, nil
	} else {
		return err, errors.New("Unable to check rook status - please check of rook is up and running")
	}
}

func (Client RookClient) Node() (string, error) {
	out, err, status := Client.transportClient.Execute(NODE_CMD)
	if status == 0 {
		return out, nil
	} else {
		return err, errors.New("Unable to check rook status - please check of rook is up and running")
	}
}

func (Client RookClient) Get_Block_client() contracts.Irook_block {
	return Client.block_client
}

func (Client RookClient) Get_FileSystem_client() contracts.Irook_filesystem{
	return Client.fs_client
}

func (Client RookClient) Get_Object_client() contracts.Irook_object{
	return Client.object_client
}

func (Client RookClient) Get_Pool_client() contracts.Irook_pool{
	return Client.pool_client
}

