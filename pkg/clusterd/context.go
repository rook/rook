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
	rookclient "github.com/rook/rook/pkg/client/clientset/versioned"
	"github.com/rook/rook/pkg/util/exec"
	"github.com/rook/rook/pkg/util/sys"
	apiextensionsclient "k8s.io/apiextensions-apiserver/pkg/client/clientset/clientset"
	"k8s.io/client-go/kubernetes"
)

// The context for loading or applying the configuration state of a service.
type Context struct {
	// Clientset is a connection to the core kubernetes API
	Clientset kubernetes.Interface

	// APIExtensionClientset is a connection to the API Extension kubernetes API
	APIExtensionClientset apiextensionsclient.Interface

	// RookClientset is a typed connection to the rook API
	RookClientset rookclient.Interface

	// The implementation of executing a console command
	Executor exec.Executor

	// The root configuration directory used by services
	ConfigDir string

	// A value indicating the desired logging/tracing level
	LogLevel capnslog.LogLevel

	// The full path to a config file that can be used to override generated settings
	ConfigFileOverride string

	// Information about the network for this machine and its cluster
	NetworkInfo NetworkInfo

	// The local devices detected on the node
	Devices []*sys.LocalDisk
}
