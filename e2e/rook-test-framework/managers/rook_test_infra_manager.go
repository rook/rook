package managers

import (

	"github.com/dangula/rook/e2e/rook-test-framework/contracts"
	"github.com/dangula/rook/e2e/rook-test-framework/enums"
	"errors"
	"github.com/dangula/rook/e2e/rook-test-framework/transport"
	"github.com/dangula/rook/e2e/rook-test-framework/objects"
	"fmt"
)

type rookTestInfraManager struct {
	transportClient contracts.ITransportClient
	platformType enums.RookPlatformType
	dockerized bool
	dockerContext objects.DockerContext
}

func GetRookTestInfraManager(platformType enums.RookPlatformType, isDockerized bool) (error, rookTestInfraManager) {
	var transportClient contracts.ITransportClient
	var dockerContext objects.DockerContext
	var dockerized bool = isDockerized


	//TODO this needs to come from user IF remote or using docker-machine
	dockerEnv := []string {
		"DOCKER_TLS_VERIFY=1",
		"DOCKER_HOST=tcp://192.168.99.100:2376",
		"DOCKER_CERT_PATH=/Users/tyjohnson/.docker/machine/machines/default",
		"DOCKER_MACHINE_NAME=default"}

	if isDockerized {
		dockerContext = objects.SetDockerContext(transport.CreateDockerClient(dockerEnv))
	}

	switch {
	case platformType == enums.Kubernetes:
		transportClient = transport.CreateNewk8sTransportClient()
	case platformType == enums.StandAlone:
		transportClient = transport.CreateNewStandAloneTransportClient()
	default:
		return errors.New("Unsupported Rook Platform Type"), rookTestInfraManager{}
	}

	return nil, rookTestInfraManager{
		platformType: platformType,
		transportClient: transportClient,
		dockerized: dockerized,
		dockerContext: dockerContext,
	}
}

func (r rookTestInfraManager) ValidateAndPrepareEnvironment() error	{
	if r.dockerized {
		//validate docker is available

		//verify docker container is not already running
		//execute command to init docker container
		_, dockerClient := r.dockerContext.Get_DockerClient()

		cmd := []string {
			"--rm", "-i", "--net=host", "-e \"container=docker\"", "--privileged", "-d", "--security-opt=seccomp:unconfined",
			"--cap-add=SYS_ADMIN", "-v=/dev:/dev", "-v=/sys:/sys", "-v=/sys/fs/cgroup:/sys/fs/cgroup", "-v=/sbin/modprobe:/sbin/modprobe",
			"-v=/lib/modules:/lib/modules:rw", "-v=/var/run/docker.sock:/test", "-P", "quay.io/quantum/rook-test", "/sbin/init",
		}

		stdout, stderr, err := dockerClient.Run(cmd)

		if err != nil {
			return fmt.Errorf("%v --> %v --> %v", err, errors.New(stdout), errors.New(stderr))
		}

		//save containerId to struct --> TODO fix
		r.dockerContext.Set_ContainerId(stderr)

		//STEP 1 --> Create symlink from /docker.sock to /var/run/docker.sock
		dockerClient.Execute([]string{"-it", stderr, "ln -s /test /var/run/docker.sock"})

		//STEP 2 --> Bring up k8s cluster
		//download script to container
		//run script

		//STEP 3 --> Untaint master node
		// kubectl taint nodes --all dedicated-

		//STEP 4 --> Drain node 2 --> TODO: fix script not to create 1st and 2nd node
		// kubectl drain kube-node-2 --force --ignore-daemonsets

		//STEP 5 --> Delete 2nd unneeded node --> TODO: fix script not to create 1st and 2nd node
		// kubectl delete node kube-node-2 --force

		//STEP 6 --> Patch controller --> TODO: pre-patch image
		//download controller.json
		// yes | cp -rf kube-controller-manager.json $(find /var/lib/docker/aufs/mnt -type f -name kube-controller-manager.json)

		//STEP 7 --> Install Ceph --> TODO fix so images are already patched with ceph
		//curl --unix-socket /var/run/docker.sock http:/containers/json | jq -r '.[].Id' | xargs -i docker exec -i {} bash -c 'apt-get -y update && apt-get install -qqy ceph-common'



	} else {
		//validate local env
	}

	return nil
}

func (r rookTestInfraManager) InstallRook(tag string) (error, client contracts.Irook_client)	{
	//if k8
	//STEP 1 --> Create rook operator
	// create pod spec
	//wait for up

	//STEP 2 --> Create rook cluster
	//create pod spec
	//wait for up

	//STEP 3 --> Create rook client
	//create pod spec
	//wait for up


	return nil, nil
}

func (r rookTestInfraManager) TearDownRook(client contracts.Irook_client) error	{

	return nil
}

func (r rookTestInfraManager) TearDownInfrastructureCreatedEnvironment() error {
	return nil
}

func (r rookTestInfraManager) isRookInstalled() bool {
	return false
}

func (r rookTestInfraManager) CanConnectToDocker() bool {
	return false
}

func (r rookTestInfraManager) CanConnectToK8s() bool {
	return false
}