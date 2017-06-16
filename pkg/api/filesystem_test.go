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
	"fmt"
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/rook/rook/pkg/ceph/test"
	cephtest "github.com/rook/rook/pkg/ceph/test"
	"github.com/rook/rook/pkg/clusterd"
	"github.com/rook/rook/pkg/model"
	"github.com/rook/rook/pkg/util"
	exectest "github.com/rook/rook/pkg/util/exec/test"
	"github.com/stretchr/testify/assert"
)

const (
	// this JSON was generated from the mon_command "fs ls",  ExecuteMonCommand(conn, map[string]interface{}{"prefix": "fs ls"})
	cephFilesystemListResponseRaw = `[{"name":"myfs1","metadata_pool":"myfs1-metadata","metadata_pool_id":2,"data_pool_ids":[1],"data_pools":["myfs1-data"]}]`
)

func TestGetFileSystemsHandler(t *testing.T) {
	etcdClient := util.NewMockEtcdClient()
	req, err := http.NewRequest("GET", "http://10.0.0.100/filesystem", nil)
	if err != nil {
		logger.Fatal(err)
	}
	configDir, _ := ioutil.TempDir("", "")
	defer os.RemoveAll(configDir)
	cephtest.CreateClusterInfo(etcdClient, configDir, []string{"mon0"})

	executor := &exectest.MockExecutor{
		MockExecuteCommandWithOutputFile: func(actionName string, command string, outFileArg string, args ...string) (string, error) {
			logger.Infof("OUTPUT: %s %v", command, args)
			if args[0] == "fs" && args[1] == "ls" {
				return cephFilesystemListResponseRaw, nil
			}
			return "", fmt.Errorf("unexpected mon_command '%s'", args[0])
		},
	}
	context := &clusterd.Context{
		DirectContext: clusterd.DirectContext{EtcdClient: etcdClient},
		Executor:      executor,
	}

	// make a request to GetFileSystems and verify the results
	w := httptest.NewRecorder()
	h := newTestHandler(context)
	h.GetFileSystems(w, req)
	assert.Equal(t, http.StatusOK, w.Code)

	expectedRespObj := []model.Filesystem{
		{Name: "myfs1", MetadataPool: "myfs1-metadata", DataPools: []string{"myfs1-data"}},
	}

	// unmarshal the http response to get the actual object and compare it to the expected object
	var actualResultObj []model.Filesystem
	bodyBytes, _ := ioutil.ReadAll(w.Body)
	json.Unmarshal(bodyBytes, &actualResultObj)
	assert.Equal(t, expectedRespObj, actualResultObj)
}

func TestCreateFileSystemHandler(t *testing.T) {
	etcdClient := util.NewMockEtcdClient()
	configDir, _ := ioutil.TempDir("", "")
	context := &clusterd.Context{
		DirectContext: clusterd.DirectContext{EtcdClient: etcdClient},
		Executor:      &exectest.MockExecutor{},
		ConfigDir:     configDir,
	}
	defer os.RemoveAll(context.ConfigDir)
	test.CreateClusterInfo(etcdClient, configDir, []string{"a"})
	defer os.RemoveAll("mon0")

	req, err := http.NewRequest("POST", "http://10.0.0.100/filesystem", strings.NewReader(`{"name": "myfs1", "poolName": "myfs1-pool"}`))
	if err != nil {
		logger.Fatal(err)
	}

	// call the CreateFileSystem handler, which should return http 202 Accepted and record info
	// about the file system request in etcd
	w := httptest.NewRecorder()
	h := newTestHandler(context)
	h.CreateFileSystem(w, req)
	assert.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, "myfs1-pool", etcdClient.GetValue("/rook/services/ceph/fs/desired/myfs1/pool"))
}

func TestCreateFileSystemMissingName(t *testing.T) {
	etcdClient := util.NewMockEtcdClient()
	context := &clusterd.Context{
		DirectContext: clusterd.DirectContext{EtcdClient: etcdClient},
		Executor:      &exectest.MockExecutor{},
	}

	req, err := http.NewRequest("POST", "http://10.0.0.100/filesystem", strings.NewReader(`{"poolName": "myfs1-pool"}`))
	if err != nil {
		logger.Fatal(err)
	}

	// call the CreateFileSystem handler, which should fail due to the missing name
	w := httptest.NewRecorder()
	h := newTestHandler(context)
	h.CreateFileSystem(w, req)
	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestRemoveFileSystemHandler(t *testing.T) {
	// mock a created filesystem by adding it to desired state in etcd
	etcdClient := util.NewMockEtcdClient()
	etcdClient.SetValue("/rook/services/ceph/fs/desired/myfs1/pool", "myfs1-pool")

	context := &clusterd.Context{
		DirectContext: clusterd.DirectContext{EtcdClient: etcdClient},
		Executor:      &exectest.MockExecutor{},
	}
	req, err := http.NewRequest("DELETE", "http://10.0.0.100/filesystem?name=myfs1", nil)
	if err != nil {
		logger.Fatal(err)
	}

	// call the RemoveFileSystem handler, which should return http 202 Accepted and remove the
	// filesystem from desired state
	w := httptest.NewRecorder()
	h := newTestHandler(context)
	h.RemoveFileSystem(w, req)
	assert.Equal(t, http.StatusAccepted, w.Code)
	assert.Equal(t, "", etcdClient.GetValue("/rook/services/ceph/fs/desired/myfs1/pool"))
}
