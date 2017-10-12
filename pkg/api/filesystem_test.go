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
	"path"
	"strings"
	"testing"

	"k8s.io/api/core/v1"

	"github.com/rook/rook/pkg/ceph/client"
	cephtest "github.com/rook/rook/pkg/ceph/test"
	"github.com/rook/rook/pkg/clusterd"
	"github.com/rook/rook/pkg/model"
	"github.com/rook/rook/pkg/operator/test"
	exectest "github.com/rook/rook/pkg/util/exec/test"
	"github.com/stretchr/testify/assert"
)

const (
	// this JSON was generated from the mon_command "fs ls",  ExecuteMonCommand(conn, map[string]interface{}{"prefix": "fs ls"})
	cephFilesystemListResponseRaw = `[{"name":"myfs1","metadata_pool":"myfs1-metadata","metadata_pool_id":2,"data_pool_ids":[1],"data_pools":["myfs1-data"]}]`
	basicFS                       = `{"name":"myfs1","metadataPool":{"replicatedConfig":{"size":1}},"dataPools":[{"replicatedConfig":{"size":1}}],"metadataServer":{"activeCount":1}}`
)

func TestGetFileSystemsHandler(t *testing.T) {
	req, err := http.NewRequest("GET", "http://10.0.0.100/filesystem", nil)
	if err != nil {
		logger.Fatal(err)
	}

	executor := &exectest.MockExecutor{
		MockExecuteCommandWithOutputFile: func(debug bool, actionName string, command string, outFileArg string, args ...string) (string, error) {
			logger.Infof("Command: %s %v", command, args)
			if args[0] == "fs" && args[1] == "ls" {
				return cephFilesystemListResponseRaw, nil
			}
			return "", fmt.Errorf("unexpected mon_command '%s'", args[0])
		},
	}
	context := &clusterd.Context{
		Executor: executor,
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
	created := false
	configDir, _ := ioutil.TempDir("", "")
	defer os.RemoveAll(configDir)
	requestedFS := false
	clientset := test.New(3)
	clusterName := "default"
	cm := &v1.ConfigMap{
		Data: map[string]string{
			"data": "rook-ceph-mon0=10.0.0.1:6790,rook-ceph-mon1=10.0.0.2:6790,rook-ceph-mon2=10.0.0.3:6790",
		},
	}
	cm.Name = "rook-ceph-mon-endpoints"
	clientset.CoreV1().ConfigMaps(clusterName).Create(cm)

	executor := &exectest.MockExecutor{
		MockExecuteCommandWithOutput: func(debug bool, actionName string, command string, args ...string) (string, error) {
			if strings.Contains(command, "ceph-authtool") {
				cephtest.CreateClusterInfo(nil, path.Join(configDir, clusterName), nil)
			}

			return "", nil
		},
		MockExecuteCommandWithOutputFile: func(debug bool, actionName string, command string, outFileArg string, args ...string) (string, error) {
			logger.Infof("Command: %s %v", command, args)
			if args[1] == "pool" {
				if args[2] == "create" || args[2] == "set" || args[2] == "application" {
					return "", nil
				}
			}
			if args[0] == "fs" {
				if args[1] == "new" {
					created = true
					return "", nil
				}
				if args[1] == "get" {
					if requestedFS {
						return `{"name":"myfs1","metadataPool":"myfs1-metadata","dataPools":["myfs1-data0"]}`, nil
					}
					requestedFS = true
					return "", fmt.Errorf("still need to create FS")
				}
			}

			return "", fmt.Errorf("unexpected command '%s'", args[0])
		},
	}
	context := &clusterd.Context{
		Executor:  executor,
		ConfigDir: configDir,
		Clientset: clientset,
	}

	req, err := http.NewRequest("POST", "http://10.0.0.100/filesystem", strings.NewReader(basicFS))
	if err != nil {
		logger.Fatal(err)
	}

	// call the CreateFileSystem handler, which should return http 202 Accepted
	w := httptest.NewRecorder()
	h := newTestHandler(context)
	h.CreateFileSystem(w, req)
	assert.Equal(t, http.StatusOK, w.Code)
	assert.True(t, created)
}

func TestCreateFileSystemMissingName(t *testing.T) {
	context := &clusterd.Context{
		Executor: &exectest.MockExecutor{},
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
	markedDown := false
	deleted := false

	executor := &exectest.MockExecutor{
		MockExecuteCommandWithOutputFile: func(debug bool, actionName string, command string, outFileArg string, args ...string) (string, error) {
			logger.Infof("Command: %s %v", command, args)
			if args[0] == "fs" {
				if args[1] == "set" && args[3] == "cluster_down" {
					markedDown = true
					return "", nil
				}
				if args[1] == "rm" {
					deleted = true
					return "", nil
				}
				if args[1] == "get" {
					return basicFS, nil
				}
			}
			if args[0] == "osd" {
				if args[1] == "lspools" {
					pools := []*client.CephStoragePoolSummary{
						{Name: "mypool", Number: 0},
					}
					output, err := json.Marshal(pools)
					assert.Nil(t, err)
					return string(output), nil
				}
			}
			return "", fmt.Errorf("unexpected command '%s'", args[0])
		},
	}
	context := &clusterd.Context{Executor: executor}

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
	assert.True(t, markedDown)
	assert.True(t, deleted)
}
