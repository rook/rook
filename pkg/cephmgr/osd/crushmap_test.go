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
package osd

import (
	"encoding/json"
	"fmt"
	"os"
	"testing"

	"github.com/rook/rook/pkg/cephmgr/client"

	testceph "github.com/rook/rook/pkg/cephmgr/client/test"
	"github.com/rook/rook/pkg/clusterd"
	"github.com/rook/rook/pkg/util"
	"github.com/stretchr/testify/assert"
)

func TestCrushMap(t *testing.T) {

	etcdClient := util.NewMockEtcdClient()
	context := &clusterd.Context{EtcdClient: etcdClient, NodeID: "node1"}
	factory := &testceph.MockConnectionFactory{Fsid: "fsid", SecretKey: "key"}
	conn, _ := factory.NewConnWithClusterAndUser("cluster", "user")
	conn.(*testceph.MockConnection).MockMonCommand = func(buf []byte) (buffer []byte, info string, err error) {
		var request client.MonStatusRequest
		err = json.Unmarshal(buf, &request)
		assert.Nil(t, err)
		assert.Equal(t, "json", request.Format)
		assert.Equal(t, "osd crush create-or-move", request.Prefix)
		assert.Equal(t, 23, request.ID)
		assert.NotEqual(t, 0.0, request.Weight)
		assert.Equal(t, 3, len(request.Args), fmt.Sprintf("args=%v", request.Args))

		// verify the contents of the CRUSH location args
		argsSet := util.CreateSet(request.Args)
		assert.True(t, argsSet.Contains("root=default"))
		assert.True(t, argsSet.Contains("dc=datacenter1"))
		assert.True(t, argsSet.Contains("hostName=node1"))

		return []byte{}, "", nil
	}

	location := "root=default,dc=datacenter1,hostName=node1"

	err := addOSDToCrushMap(conn, context, 23, "/", location)
	assert.Nil(t, err)

	// location should have been stored in etcd as well
	assert.Equal(t, location, etcdClient.GetValue("/rook/nodes/config/node1/location"))
}

func TestGetCrushMap(t *testing.T) {
	factory := &testceph.MockConnectionFactory{Fsid: "fsid", SecretKey: "key"}
	conn, _ := factory.NewConnWithClusterAndUser("cluster", "user")
	response, err := GetCrushMap(conn)

	assert.Nil(t, err)
	assert.Equal(t, "", response)
}

func TestCrushLocation(t *testing.T) {
	loc := "dc=datacenter1"
	hostName, err := os.Hostname()
	assert.Nil(t, err)

	// test that host name and root will get filled in with default/runtime values
	res, err := formatLocation(loc)
	assert.Nil(t, err)
	assert.Equal(t, 3, len(res))
	locSet := util.CreateSet(res)
	assert.True(t, locSet.Contains("root=default"))
	assert.True(t, locSet.Contains("dc=datacenter1"))
	assert.True(t, locSet.Contains(fmt.Sprintf("hostName=%s", hostName)))

	// test that if host name and root are already set they will be honored
	loc = "root=otherRoot,dc=datacenter2,hostName=node123"
	res, err = formatLocation(loc)
	assert.Nil(t, err)
	assert.Equal(t, 3, len(res))
	locSet = util.CreateSet(res)
	assert.True(t, locSet.Contains("root=otherRoot"))
	assert.True(t, locSet.Contains("dc=datacenter2"))
	assert.True(t, locSet.Contains("hostName=node123"))

	// test an invalid CRUSH location format
	loc = "root=default,prop:value"
	_, err = formatLocation(loc)
	assert.NotNil(t, err)
	assert.Contains(t, err.Error(), "is not in a valid format")
}
