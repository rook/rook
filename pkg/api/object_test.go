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
package api

import (
	"encoding/json"
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"testing"

	testceph "github.com/rook/rook/pkg/cephmgr/client/test"
	"github.com/rook/rook/pkg/cephmgr/test"
	"github.com/rook/rook/pkg/clusterd"
	"github.com/rook/rook/pkg/clusterd/inventory"
	"github.com/rook/rook/pkg/model"
	"github.com/rook/rook/pkg/util"
	"github.com/stretchr/testify/assert"
)

func TestCreateObjectStoreHandler(t *testing.T) {
	etcdClient := util.NewMockEtcdClient()
	context := &clusterd.Context{EtcdClient: etcdClient}

	req, err := http.NewRequest("POST", "http://10.0.0.100/objectstore", nil)
	if err != nil {
		logger.Fatal(err)
	}

	cephFactory := &testceph.MockConnectionFactory{Fsid: "myfsid", SecretKey: "mykey"}
	connFactory := &test.MockConnectionFactory{}

	// call the CreateObjectStore handler, which should return http 202 Accepted and record info
	// about the file system request in etcd
	w := httptest.NewRecorder()
	h := NewHandler(context, connFactory, cephFactory)
	h.CreateObjectStore(w, req)
	assert.Equal(t, http.StatusAccepted, w.Code)
	assert.Equal(t, "1", etcdClient.GetValue("/rook/services/ceph/object/desired/state"))
}

func TestRemoveObjectStoreHandler(t *testing.T) {
	// simulate object store already being installed by setting the desired key in etcd
	etcdClient := util.NewMockEtcdClient()
	etcdClient.SetValue("/rook/services/ceph/object/desired/state", "1")

	context := &clusterd.Context{EtcdClient: etcdClient}

	req, err := http.NewRequest("POST", "http://10.0.0.100/objectstore/remove", nil)
	if err != nil {
		logger.Fatal(err)
	}

	cephFactory := &testceph.MockConnectionFactory{Fsid: "myfsid", SecretKey: "mykey"}
	connFactory := &test.MockConnectionFactory{}

	// call RemoveObjectStore handler and verify the response is 202 Accepted and the desired
	// key has been deleted from etcd
	w := httptest.NewRecorder()
	h := NewHandler(context, connFactory, cephFactory)
	h.RemoveObjectStore(w, req)
	assert.Equal(t, http.StatusAccepted, w.Code)
	assert.Equal(t, 0, etcdClient.GetChildDirs("/rook/services/ceph/object/desired").Count())
}

func TestGetObjectStoreConnectionInfoHandler(t *testing.T) {
	etcdClient := util.NewMockEtcdClient()
	inventory.SetIPAddress(etcdClient, "123", "1.2.3.4", "2.3.4.5")
	context := &clusterd.Context{EtcdClient: etcdClient}

	req, err := http.NewRequest("GET", "http://10.0.0.100/objectstore/connectioninfo", nil)
	if err != nil {
		logger.Fatal(err)
	}

	cephFactory := &testceph.MockConnectionFactory{Fsid: "myfsid", SecretKey: "mykey"}
	connFactory := &test.MockConnectionFactory{}

	// before RGW has been installed or any user accounts have been created, the handler will return 404 not found
	w := httptest.NewRecorder()
	h := NewHandler(context, connFactory, cephFactory)
	h.GetObjectStoreConnectionInfo(w, req)
	assert.Equal(t, http.StatusNotFound, w.Code)

	// simulate RGW being installed and the built in user being created
	etcdClient.SetValue("/rook/services/ceph/rgw/applied/node/123", "")
	etcdClient.SetValue("/rook/services/ceph/object/applied/admin/id", "UST0JAP8CE61FDE0Q4BE")
	etcdClient.SetValue("/rook/services/ceph/object/applied/admin/_secret", "tVCuH20xTokjEpVJc7mKjL8PLTfGh4NZ3le3zg9X")

	w = httptest.NewRecorder()
	h = NewHandler(context, connFactory, cephFactory)
	h.GetObjectStoreConnectionInfo(w, req)
	assert.Equal(t, http.StatusOK, w.Code)

	expectedRespObj := model.ObjectStoreS3Info{
		Host:       "rook-rgw:53390",
		IPEndpoint: "1.2.3.4:53390",
		AccessKey:  "UST0JAP8CE61FDE0Q4BE",
		SecretKey:  "tVCuH20xTokjEpVJc7mKjL8PLTfGh4NZ3le3zg9X",
	}

	// unmarshal the http response to get the actual object and compare it to the expected object
	var actualResultObj model.ObjectStoreS3Info
	bodyBytes, _ := ioutil.ReadAll(w.Body)
	json.Unmarshal(bodyBytes, &actualResultObj)
	assert.Equal(t, expectedRespObj, actualResultObj)
}
