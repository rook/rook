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

package clusterd

import (
	"github.com/coreos/pkg/capnslog"
	netclient "github.com/k8snetworkplumbingwg/network-attachment-definition-client/pkg/client/clientset/versioned/typed/k8s.cni.cncf.io/v1"
	rookclient "github.com/rook/rook/pkg/client/clientset/versioned"
	"github.com/rook/rook/pkg/util/exec"
	"github.com/rook/rook/pkg/util/sys"
	apiextensionsclient "k8s.io/apiextensions-apiserver/pkg/client/clientset/clientset"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// Context for loading or applying the configuration state of a service.
type Context struct {
	// The kubernetes config used for this context
	KubeConfig *rest.Config

	// Clientset is a connection to the core kubernetes API
	Clientset kubernetes.Interface

	// DynamicClientset is a dynamic connection to the Kubernetes API
	DynamicClientset dynamic.Interface

	// Represents the Client provided by the controller-runtime package to interact with Kubernetes objects
	Client client.Client

	// APIExtensionClientset is a connection to the API Extension kubernetes API
	APIExtensionClientset apiextensionsclient.Interface

	// RookClientset is a typed connection to the rook API
	RookClientset rookclient.Interface

	// The implementation of executing a console command
	Executor exec.Executor

	// The implementation of executing remotely a console command to a given pod
	RemoteExecutor exec.RemotePodCommandExecutor

	// The root configuration directory used by services
	ConfigDir string

	// A value indicating the desired logging/tracing level
	LogLevel capnslog.LogLevel

	// The full path to a config file that can be used to override generated settings
	ConfigFileOverride string

	// NetworkClient is a connection to the CNI plugin API
	NetworkClient netclient.K8sCniCncfIoV1Interface

	// The local devices detected on the node
	Devices []*sys.LocalDisk
}
