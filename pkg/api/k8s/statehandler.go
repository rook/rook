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
package thirdparty

import (
	"github.com/rook/rook/pkg/cephmgr/mon"
	"github.com/rook/rook/pkg/model"
	"k8s.io/client-go/1.5/kubernetes"
)

type stateHandler struct {
	clientset *kubernetes.Clientset
}

func New(clientset *kubernetes.Clientset) *stateHandler {
	return &stateHandler{clientset: clientset}
}

func (s *stateHandler) EnableObjectStore() error {
	logger.Infof("TODO: Enable the object store")
	return nil
}

func (s *stateHandler) RemoveObjectStore() error {
	logger.Infof("TODO: Remove the object store")
	return nil
}

func (e *stateHandler) GetObjectStoreConnectionInfo() (*model.ObjectStoreS3Info, bool, error) {
	logger.Infof("TODO: Get the object store connection info")
	return nil, true, nil
}

func (e *stateHandler) CreateFileSystem(fs *model.FilesystemRequest) error {
	logger.Infof("TODO: Create file system")
	return nil
}

func (e *stateHandler) RemoveFileSystem(fs *model.FilesystemRequest) error {
	logger.Infof("TODO: Remove file system")
	return nil
}

func (e *stateHandler) GetMonitors() (map[string]*mon.CephMonitorConfig, error) {
	logger.Infof("TODO: Get monitors")
	mons := map[string]*mon.CephMonitorConfig{}

	return mons, nil
}

func (e *stateHandler) GetNodes() ([]model.Node, error) {
	logger.Infof("TODO: Get nodes")
	nodes := []model.Node{}

	return nodes, nil
}
