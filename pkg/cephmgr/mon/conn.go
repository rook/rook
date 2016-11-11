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
package mon

import (
	"github.com/rook/rook/pkg/cephmgr/client"
	"github.com/rook/rook/pkg/clusterd"
)

type ConnectionFactory interface {
	ConnectAsAdmin(context *clusterd.Context, cephFactory client.ConnectionFactory) (client.Connection, error)
}

type rookConnFactory struct {
}

func NewConnectionFactory() ConnectionFactory { return &rookConnFactory{} }

func (c *rookConnFactory) ConnectAsAdmin(
	context *clusterd.Context, cephFactory client.ConnectionFactory) (client.Connection, error) {

	// load information about the cluster
	cluster, err := LoadClusterInfo(context.EtcdClient)
	if err != nil {
		return nil, err
	}

	// open an admin connection to the cluster
	return ConnectToClusterAsAdmin(context, cephFactory, cluster)
}
