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
package k8s

import (
	"fmt"

	"github.com/rook/rook/pkg/cephmgr/mon"
	"github.com/rook/rook/pkg/cephmgr/rgw"
	"github.com/rook/rook/pkg/clusterd"
	"github.com/rook/rook/pkg/model"
	"github.com/rook/rook/pkg/operator/k8sutil"
	"k8s.io/client-go/1.5/kubernetes"
)

type stateHandler struct {
	clientset   *kubernetes.Clientset
	context     *clusterd.DaemonContext
	clusterInfo *mon.ClusterInfo
}

func New(clientset *kubernetes.Clientset, context *clusterd.DaemonContext, clusterInfo *mon.ClusterInfo) *stateHandler {
	return &stateHandler{clientset: clientset, context: context, clusterInfo: clusterInfo}
}

func (s *stateHandler) EnableObjectStore() error {
	logger.Infof("Enabling the object store")
	//resource := &v1beta1.ThirdPartyResource{}
	//_, err := s.clientset.ThirdPartyResources().Create(resource)
	if err := createObjectUser(s.clientset, s.context, s.clusterInfo); err != nil {
		return fmt.Errorf("failed to create the object store. %+v", err)
	}

	return nil
}

func (s *stateHandler) RemoveObjectStore() error {
	logger.Infof("TODO: Remove the object store")
	return nil
}

func (s *stateHandler) GetObjectStoreConnectionInfo() (*model.ObjectStoreS3Info, bool, error) {
	logger.Infof("Getting the object store connection info")
	service, err := s.clientset.Services(k8sutil.Namespace).Get("ceph-rgw")
	if err != nil {
		return nil, false, fmt.Errorf("failed to get rgw service. %+v", err)
	}

	accessID, secret, err := getS3Creds(s.clientset)
	if err != nil {
		return nil, false, fmt.Errorf("failed to get s3 creds. %+v", err)
	}

	info := &model.ObjectStoreS3Info{
		Host:       "rook-rgw",
		IPEndpoint: rgw.GetRGWEndpoint(service.Spec.ClusterIP),
		AccessKey:  accessID,
		SecretKey:  secret,
	}
	logger.Infof("Object store connection: %+v", info)
	return info, true, nil
}

func (s *stateHandler) CreateFileSystem(fs *model.FilesystemRequest) error {
	logger.Infof("TODO: Create file system")
	return nil
}

func (s *stateHandler) RemoveFileSystem(fs *model.FilesystemRequest) error {
	logger.Infof("TODO: Remove file system")
	return nil
}

func (s *stateHandler) GetMonitors() (map[string]*mon.CephMonitorConfig, error) {
	logger.Infof("TODO: Get monitors")
	mons := map[string]*mon.CephMonitorConfig{}

	return mons, nil
}

func (s *stateHandler) GetNodes() ([]model.Node, error) {
	logger.Infof("Getting nodes")
	return getNodes(s.clientset)
}
