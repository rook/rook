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

	"github.com/rook/rook/pkg/ceph/mon"
	cephrgw "github.com/rook/rook/pkg/ceph/rgw"
	"github.com/rook/rook/pkg/clusterd"
	"github.com/rook/rook/pkg/model"
	"github.com/rook/rook/pkg/operator/k8sutil"
	k8smds "github.com/rook/rook/pkg/operator/mds"
	k8srgw "github.com/rook/rook/pkg/operator/rgw"
	"k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type clusterHandler struct {
	context     *clusterd.Context
	clusterInfo *mon.ClusterInfo
	namespace   string
	versionTag  string
	hostNetwork bool
}

func New(context *clusterd.Context, clusterInfo *mon.ClusterInfo, namespace, versionTag string, hostNetwork bool) *clusterHandler {
	return &clusterHandler{context: context, clusterInfo: clusterInfo, namespace: namespace, versionTag: versionTag, hostNetwork: hostNetwork}
}

func (s *clusterHandler) GetClusterInfo() (*mon.ClusterInfo, error) {
	return s.clusterInfo, nil
}

func (s *clusterHandler) GetObjectStores() ([]model.ObjectStoreResponse, error) {
	response := []model.ObjectStoreResponse{}

	// require both the realm and k8s service to exist to consider an object store available
	realms, err := cephrgw.GetObjectStores(cephrgw.NewContext(s.context, "", s.clusterInfo.Name))
	if err != nil {
		return response, fmt.Errorf("failed to get rgw realms. %+v", err)
	}

	// get the rgw service for each realm
	for _, realm := range realms {
		service, err := s.context.Clientset.CoreV1().Services(s.clusterInfo.Name).Get(k8srgw.InstanceName(realm), metav1.GetOptions{})
		if err != nil {
			if !errors.IsNotFound(err) {
				return response, fmt.Errorf("failed to get rgw service %s. %+v", realm, err)
			}
			logger.Warningf("RGW realm found, but no k8s service found for %s", realm)
			continue
		}
		response = append(response, model.ObjectStoreResponse{
			Name:        realm,
			Ports:       service.Spec.Ports,
			ClusterIP:   service.Spec.ClusterIP,
			ExternalIPs: service.Spec.ExternalIPs,
		})
	}

	logger.Infof("Object stores: %+v", response)
	return response, nil
}

func (s *clusterHandler) EnableObjectStore(config model.ObjectStore) error {
	logger.Infof("Starting the Object store")

	// save the certificate in a secret if we weren't given a reference to a secret
	if config.Gateway.Certificate != "" && config.Gateway.CertificateRef == "" {
		certName := fmt.Sprintf("rook-rgw-%s-cert", config.Name)
		config.Gateway.CertificateRef = certName

		data := map[string][]byte{"cert": []byte(config.Gateway.Certificate)}
		certSecret := &v1.Secret{ObjectMeta: metav1.ObjectMeta{Name: certName, Namespace: s.namespace}, Data: data}

		_, err := s.context.Clientset.Core().Secrets(s.namespace).Create(certSecret)
		if err != nil {
			if !errors.IsAlreadyExists(err) {
				return fmt.Errorf("failed to create cert secret. %+v", err)
			}
			if _, err := s.context.Clientset.Core().Secrets(s.namespace).Update(certSecret); err != nil {
				return fmt.Errorf("failed to update secret. %+v", err)
			}
			logger.Infof("updated the certificate secret %s", certName)
		}
	}

	store := k8srgw.ModelToSpec(config, s.namespace)
	err := store.Update(s.context, s.versionTag, s.hostNetwork)
	if err != nil {
		return fmt.Errorf("failed to start rgw. %+v", err)
	}
	return nil
}

func (s *clusterHandler) RemoveObjectStore(name string) error {
	store := &k8srgw.ObjectStore{ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: s.clusterInfo.Name}}
	return store.Delete(s.context)
}

func (s *clusterHandler) GetObjectStoreConnectionInfo(name string) (*model.ObjectStoreConnectInfo, bool, error) {
	logger.Infof("Getting the object store connection info")
	service, err := s.context.Clientset.CoreV1().Services(s.namespace).Get(k8srgw.InstanceName(name), metav1.GetOptions{})
	if err != nil {
		return nil, false, fmt.Errorf("failed to get rgw service. %+v", err)
	}

	info := &model.ObjectStoreConnectInfo{
		Host:      k8srgw.InstanceName(name),
		IPAddress: service.Spec.ClusterIP,
		Ports:     []int32{},
	}

	// append all of the ports
	for _, port := range service.Spec.Ports {
		info.Ports = append(info.Ports, port.Port)
		return info, true, nil
	}

	logger.Debugf("Object store connection: %+v", info)
	return info, false, fmt.Errorf("no ports available for rgw")
}

func (s *clusterHandler) StartFileSystem(fs *model.FilesystemRequest) error {
	logger.Infof("Starting the MDS")
	// Passing an empty Placement{} as the api doesn't know about placement
	// information. This should be resolved with the transition to CRD (TPR).
	c := k8smds.New(s.context, s.namespace, s.versionTag, k8sutil.Placement{}, s.hostNetwork)
	return c.Start()
}

func (s *clusterHandler) RemoveFileSystem(fs *model.FilesystemRequest) error {
	logger.Infof("TODO: Remove file system")
	return nil
}

func (s *clusterHandler) GetMonitors() (map[string]*mon.CephMonitorConfig, error) {
	logger.Infof("TODO: Get monitors")
	mons := map[string]*mon.CephMonitorConfig{}

	return mons, nil
}

func (s *clusterHandler) GetNodes() ([]model.Node, error) {
	logger.Infof("Getting nodes")
	return getNodes(s.context.Clientset)
}
