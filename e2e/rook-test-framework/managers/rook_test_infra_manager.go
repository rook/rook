package managers

import (

	"github.com/dangula/rook/e2e/rook-test-framework/contracts"
	"github.com/dangula/rook/e2e/rook-test-framework/enums"
	"errors"
	"github.com/dangula/rook/e2e/rook-test-framework/transport"
)

type rookTestInfraManager struct {
	transportClient contracts.ITransportClient
	platformType enums.RookPlatformType
	dockerClient contracts.IDockerClient
}

func GetRookTestInfraManager(platformType enums.RookPlatformType, isDockerized bool) (error, rookTestInfraManager) {
	var transportClient contracts.ITransportClient
	var dockerClient contracts.IDockerClient = nil

	if(isDockerized) {
		dockerClient = transport.CreateDockerClient()
	}

	switch {
	case platformType == enums.Kubernetes:
		transportClient = transport.CreateNewk8sTransportClient()
	case platformType == enums.StandAlone:
		transportClient = transport.CreateNewStandAloneTransportClient()
	default:
		return errors.New("Unsupported Rook Platform Type"), nil
	}

	return nil, rookTestInfraManager{
		platformType: platformType,
		transportClient: transportClient,
		dockerClient: dockerClient,
	}
}

func (r rookTestInfraManager) SetupInfra() error	{
	if(r.dockerClient != nil) {
		//validate docker is available
		//verify docker is not already running
		//execute command to init docker container
	}

	return nil
}

func (r rookTestInfraManager) InstallRook(tag string) (error, client contracts.Irook_client)	{

	//if k8

	//if standalone
	return nil, nil
}

func (r rookTestInfraManager) TearDownRook(client contracts.Irook_client) error	{

	return nil
}

func (r rookTestInfraManager) TearDownInfra() error {
	return nil
}