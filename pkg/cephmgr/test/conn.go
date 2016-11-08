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
package test

import (
	"github.com/rook/rook/pkg/cephmgr/client"
	"github.com/rook/rook/pkg/clusterd"
)

type MockConnectionFactory struct {
	MockConnectAsAdmin func(context *clusterd.Context, cephFactory client.ConnectionFactory) (client.Connection, error)
}

func (m *MockConnectionFactory) ConnectAsAdmin(
	context *clusterd.Context, cephFactory client.ConnectionFactory) (client.Connection, error) {

	if m.MockConnectAsAdmin != nil {
		return m.MockConnectAsAdmin(context, cephFactory)
	}

	return cephFactory.NewConnWithClusterAndUser("mycluster", "admin")
}
