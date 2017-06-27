/*
Copyright 2016 The Rook Authors. All rights reserved.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

	http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package objects

import "github.com/rook/rook/tests/framework/contracts"

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
