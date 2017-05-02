package objects

import "github.com/rook/rook/e2e/framework/contracts"

type DockerContext struct {
	containerId  string
	dockerClient contracts.IDockerClient
}

func SetDockerContext(dockerClient contracts.IDockerClient) DockerContext {
	return DockerContext{dockerClient: dockerClient}
}

func (d *DockerContext) Get_ContainerId() string {
	return d.containerId
}

func (d *DockerContext) Set_ContainerId(containerId string) string {
	d.containerId = containerId

	return containerId
}

func (d *DockerContext) Get_DockerClient() contracts.IDockerClient {
	return d.dockerClient
}
