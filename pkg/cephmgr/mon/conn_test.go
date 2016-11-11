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
	"testing"

	testceph "github.com/rook/rook/pkg/cephmgr/client/test"
	"github.com/rook/rook/pkg/clusterd"
	"github.com/rook/rook/pkg/util"
	"github.com/stretchr/testify/assert"
)

func TestBasicConn(t *testing.T) {
	etcdClient := util.NewMockEtcdClient()
	createTestClusterInfo(etcdClient, []string{"a"})
	factory := NewConnectionFactory()
	fact := &testceph.MockConnectionFactory{Fsid: "myfsid", SecretKey: "mykey"}
	context := &clusterd.Context{EtcdClient: etcdClient, ConfigDir: "/tmp"}

	conn, err := factory.ConnectAsAdmin(context, fact)
	assert.Nil(t, err)
	assert.NotNil(t, conn)
}
