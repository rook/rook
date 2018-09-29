/*
Copyright 2018 The Kubernetes Authors.

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

package pluginwatcher

import (
	"fmt"
	"net"
	"sync"
	"time"

	"github.com/golang/glog"
	"golang.org/x/net/context"
	"google.golang.org/grpc"

	registerapi "k8s.io/kubernetes/pkg/kubelet/apis/pluginregistration/v1alpha1"
	v1beta1 "k8s.io/kubernetes/pkg/kubelet/util/pluginwatcher/example_plugin_apis/v1beta1"
	v1beta2 "k8s.io/kubernetes/pkg/kubelet/util/pluginwatcher/example_plugin_apis/v1beta2"
)

const (
	PluginName = "example-plugin"
	PluginType = "example-plugin-type"
)

// examplePlugin is a sample plugin to work with plugin watcher
type examplePlugin struct {
	grpcServer         *grpc.Server
	wg                 sync.WaitGroup
	registrationStatus chan registerapi.RegistrationStatus // for testing
	endpoint           string                              // for testing
}

type pluginServiceV1Beta1 struct {
	server *examplePlugin
}

func (s *pluginServiceV1Beta1) GetExampleInfo(ctx context.Context, rqt *v1beta1.ExampleRequest) (*v1beta1.ExampleResponse, error) {
	glog.Infof("GetExampleInfo v1beta1field: %s", rqt.V1Beta1Field)
	return &v1beta1.ExampleResponse{}, nil
}

func (s *pluginServiceV1Beta1) RegisterService() {
	v1beta1.RegisterExampleServer(s.server.grpcServer, s)
}

type pluginServiceV1Beta2 struct {
	server *examplePlugin
}

func (s *pluginServiceV1Beta2) GetExampleInfo(ctx context.Context, rqt *v1beta2.ExampleRequest) (*v1beta2.ExampleResponse, error) {
	glog.Infof("GetExampleInfo v1beta2_field: %s", rqt.V1Beta2Field)
	return &v1beta2.ExampleResponse{}, nil
}

func (s *pluginServiceV1Beta2) RegisterService() {
	v1beta2.RegisterExampleServer(s.server.grpcServer, s)
}

// NewExamplePlugin returns an initialized examplePlugin instance
func NewExamplePlugin() *examplePlugin {
	return &examplePlugin{}
}

// NewTestExamplePlugin returns an initialized examplePlugin instance for testing
func NewTestExamplePlugin(endpoint string) *examplePlugin {
	return &examplePlugin{
		registrationStatus: make(chan registerapi.RegistrationStatus),
		endpoint:           endpoint,
	}
}

// GetInfo is the RPC invoked by plugin watcher
func (e *examplePlugin) GetInfo(ctx context.Context, req *registerapi.InfoRequest) (*registerapi.PluginInfo, error) {
	return &registerapi.PluginInfo{
		Type:              PluginType,
		Name:              PluginName,
		Endpoint:          e.endpoint,
		SupportedVersions: []string{"v1beta1", "v1beta2"},
	}, nil
}

func (e *examplePlugin) NotifyRegistrationStatus(ctx context.Context, status *registerapi.RegistrationStatus) (*registerapi.RegistrationStatusResponse, error) {
	if e.registrationStatus != nil {
		e.registrationStatus <- *status
	}
	if !status.PluginRegistered {
		glog.Errorf("Registration failed: %s\n", status.Error)
	}
	return &registerapi.RegistrationStatusResponse{}, nil
}

// Serve starts example plugin grpc server
func (e *examplePlugin) Serve(socketPath string) error {
	glog.Infof("starting example server at: %s\n", socketPath)
	lis, err := net.Listen("unix", socketPath)
	if err != nil {
		return err
	}
	glog.Infof("example server started at: %s\n", socketPath)
	e.grpcServer = grpc.NewServer()
	// Registers kubelet plugin watcher api.
	registerapi.RegisterRegistrationServer(e.grpcServer, e)
	// Registers services for both v1beta1 and v1beta2 versions.
	v1beta1 := &pluginServiceV1Beta1{server: e}
	v1beta1.RegisterService()
	v1beta2 := &pluginServiceV1Beta2{server: e}
	v1beta2.RegisterService()

	// Starts service
	e.wg.Add(1)
	go func() {
		defer e.wg.Done()
		// Blocking call to accept incoming connections.
		if err := e.grpcServer.Serve(lis); err != nil {
			glog.Errorf("example server stopped serving: %v", err)
		}
	}()
	return nil
}

func (e *examplePlugin) Stop() error {
	glog.Infof("Stopping example server\n")
	e.grpcServer.Stop()
	c := make(chan struct{})
	go func() {
		defer close(c)
		e.wg.Wait()
	}()
	select {
	case <-c:
		return nil
	case <-time.After(time.Second):
		glog.Errorf("Timed out on waiting for stop completion")
		return fmt.Errorf("Timed out on waiting for stop completion")
	}
}
