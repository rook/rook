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
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/rook/rook/pkg/clusterd"
	"github.com/rook/rook/pkg/daemon/ceph/mon"
	"github.com/rook/rook/pkg/operator/test"
	exectest "github.com/rook/rook/pkg/util/exec/test"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	SuccessGetPoolRBDResponse     = `{"pool":"rbd","pool_id":0,"size":1}{"pool":"rbd","pool_id":0,"min_size":1}{"pool":"rbd","pool_id":0,"crash_replay_interval":0}{"pool":"rbd","pool_id":0,"pg_num":2048}{"pool":"rbd","pool_id":0,"pgp_num":2048}{"pool":"rbd","pool_id":0,"crush_ruleset":0}{"pool":"rbd","pool_id":0,"hashpspool":"true"}{"pool":"rbd","pool_id":0,"nodelete":"false"}{"pool":"rbd","pool_id":0,"nopgchange":"false"}{"pool":"rbd","pool_id":0,"nosizechange":"false"}{"pool":"rbd","pool_id":0,"write_fadvise_dontneed":"false"}{"pool":"rbd","pool_id":0,"noscrub":"false"}{"pool":"rbd","pool_id":0,"nodeep-scrub":"false"}{"pool":"rbd","pool_id":0,"use_gmt_hitset":true}{"pool":"rbd","pool_id":0,"auid":0}{"pool":"rbd","pool_id":0,"min_write_recency_for_promote":0}{"pool":"rbd","pool_id":0,"fast_read":0}{"pool":"rbd","pool_id":0}{"pool":"rbd","pool_id":0}{"pool":"rbd","pool_id":0}{"pool":"rbd","pool_id":0}{"pool":"rbd","pool_id":0}{"pool":"rbd","pool_id":0}`
	SuccessGetPoolECPool1Response = `{"pool":"ecPool1","pool_id":1,"size":3}{"pool":"ecPool1","pool_id":1,"min_size":3}{"pool":"ecPool1","pool_id":1,"crash_replay_interval":0}{"pool":"ecPool1","pool_id":1,"pg_num":100}{"pool":"ecPool1","pool_id":1,"pgp_num":100}{"pool":"ecPool1","pool_id":1,"crush_ruleset":1}{"pool":"ecPool1","pool_id":1,"hashpspool":"true"}{"pool":"ecPool1","pool_id":1,"nodelete":"false"}{"pool":"ecPool1","pool_id":1,"nopgchange":"false"}{"pool":"ecPool1","pool_id":1,"nosizechange":"false"}{"pool":"ecPool1","pool_id":1,"write_fadvise_dontneed":"false"}{"pool":"ecPool1","pool_id":1,"noscrub":"false"}{"pool":"ecPool1","pool_id":1,"nodeep-scrub":"false"}{"pool":"ecPool1","pool_id":1,"use_gmt_hitset":true}{"pool":"ecPool1","pool_id":1,"auid":0}{"pool":"ecPool1","pool_id":1,"erasure_code_profile":"ecPool1_ecprofile"}{"pool":"ecPool1","pool_id":1,"min_write_recency_for_promote":0}{"pool":"ecPool1","pool_id":1,"fast_read":0}{"pool":"ecPool1","pool_id":1}{"pool":"ecPool1","pool_id":1}{"pool":"ecPool1","pool_id":1}{"pool":"ecPool1","pool_id":1}{"pool":"ecPool1","pool_id":1}{"pool":"ecPool1","pool_id":1}`
)

func newTestHandler(context *clusterd.Context) *Handler {
	context.Clientset = test.New(3)
	clusterInfo := &mon.ClusterInfo{Name: "default"}
	return newHandler(context, NewConfig(context, 53390, clusterInfo, clusterInfo.Name, "myversion", false))
}

func TestRegisterMetrics(t *testing.T) {
	context, _ := testContext()

	// create and init a new handler.  even though the first attempt fails, it should retry and return no error
	h := newTestHandler(context)
	err := h.RegisterMetrics(0)
	assert.Nil(t, err)
}

func TestGetNodesHandler(t *testing.T) {
	context, _ := testContext()

	req, err := http.NewRequest("GET", "http://10.0.0.100/node", nil)
	if err != nil {
		logger.Fatal(err)
	}

	w := httptest.NewRecorder()
	h := newTestHandler(context)

	// one node discovered
	h.GetNodes(w, req)
	assert.Equal(t, http.StatusOK, w.Code)
	response := `[{"nodeId":"","clusterName":"","publicIp":"","privateIp":"","storage":0,"lastUpdated":0,"state":0,"location":""},{"nodeId":"","clusterName":"","publicIp":"","privateIp":"","storage":0,"lastUpdated":0,"state":0,"location":""},{"nodeId":"","clusterName":"","publicIp":"","privateIp":"","storage":0,"lastUpdated":0,"state":0,"location":""}]`
	assert.Equal(t, response, w.Body.String())
}

func TestGetPoolsHandler(t *testing.T) {
	context, executor := testContext()

	req, err := http.NewRequest("GET", "http://10.0.0.100/pool", nil)
	if err != nil {
		logger.Fatal(err)
	}

	// first return no storage pools
	executor.MockExecuteCommandWithOutputFile = func(debug bool, actionName string, command string, outFileArg string, args ...string) (string, error) {
		switch {
		case args[0] == "osd" && args[1] == "lspools":
			return `[]`, nil
		}
		return "", fmt.Errorf("unexpected ceph command '%v'", args)
	}

	// no storage pools will be returned, should be empty output
	w := httptest.NewRecorder()
	h := newTestHandler(context)
	h.GetPools(w, req)
	assert.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, `[]`, w.Body.String())

	// now return some storage pools from the ceph connection
	w = httptest.NewRecorder()
	executor.MockExecuteCommandWithOutputFile = func(debug bool, actionName string, command string, outFileArg string, args ...string) (string, error) {
		switch {
		case args[0] == "osd" && args[1] == "lspools":
			return `[{"poolnum":0,"poolname":"rbd"},{"poolnum":1,"poolname":"ecPool1"}]`, nil
		case args[0] == "osd" && args[1] == "pool" && args[2] == "get":
			if args[3] == "rbd" {
				return SuccessGetPoolRBDResponse, nil
			} else if args[3] == "ecPool1" {
				return SuccessGetPoolECPool1Response, nil
			}
		case args[1] == "erasure-code-profile" && args[2] == "ls":
			return `["default","ecPool1_ecprofile"]`, nil
		case args[1] == "erasure-code-profile" && args[2] == "get":
			if args[3] == "default" {
				return `{"k":"2","m":"1","plugin":"jerasure","technique":"reed_sol_van"}`, nil
			} else if args[3] == "ecPool1_ecprofile" {
				return `{"jerasure-per-chunk-alignment":"false","k":"2","m":"1","plugin":"jerasure","ruleset-failure-domain":"osd","ruleset-root":"default","technique":"reed_sol_van","w":"8"}`, nil
			}
		}
		return "", fmt.Errorf("unexpected ceph command '%v'", args)
	}

	// storage pools should be returned now, verify the output
	h = newTestHandler(context)
	h.GetPools(w, req)
	assert.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, `[{"poolName":"rbd","poolNum":0,"type":0,"failureDomain":"","crushRoot":"","replicatedConfig":{"size":1},"erasureCodedConfig":{"dataChunkCount":0,"codingChunkCount":0,"algorithm":""}},{"poolName":"ecPool1","poolNum":1,"type":1,"failureDomain":"","crushRoot":"","replicatedConfig":{"size":0},"erasureCodedConfig":{"dataChunkCount":2,"codingChunkCount":1,"algorithm":"jerasure::reed_sol_van"}}]`, w.Body.String())
}

func TestGetPoolsHandlerFailure(t *testing.T) {
	context, executor := testContext()
	executor.MockExecuteCommandWithOutputFile = func(debug bool, actionName string, command string, outFileArg string, args ...string) (string, error) {
		return "", fmt.Errorf("test failure")
	}

	req, err := http.NewRequest("GET", "http://10.0.0.100/pool", nil)
	if err != nil {
		logger.Fatal(err)
	}

	// encounter an error during GetPools
	w := httptest.NewRecorder()
	// TODO: Simulate failure

	h := newTestHandler(context)
	h.GetPools(w, req)
	assert.Equal(t, http.StatusInternalServerError, w.Code)
	assert.Equal(t, "", w.Body.String())
}

func TestCreatePoolHandler(t *testing.T) {
	context, executor := testContext()

	req, err := http.NewRequest("POST", "http://10.0.0.100/pool",
		strings.NewReader(`{"poolName":"ecPool1","poolNum":0,"type":1,"replicatedConfig":{"size":0},"erasureCodedConfig":{"dataChunkCount":2,"codingChunkCount":1,"algorithm":""}}`))
	if err != nil {
		logger.Fatal(err)
	}

	w := httptest.NewRecorder()
	appEnabled := false
	executor.MockExecuteCommandWithOutputFile = func(debug bool, actionName string, command string, outFileArg string, args ...string) (string, error) {
		logger.Infof("EXECUTE: %s %v", command, args)
		switch {
		case args[1] == "erasure-code-profile" && args[2] == "get":
			if args[3] == "default" {
				return `{"k":"2","m":"1","plugin":"jerasure","technique":"reed_sol_van"}`, nil
			}
		case args[1] == "erasure-code-profile" && args[2] == "set":
			return "", nil
		case args[1] == "pool" && args[2] == "create":
			return "", nil
		case args[1] == "pool" && args[2] == "application" && args[3] == "enable":
			assert.Equal(t, "ecPool1", args[4])
			assert.Equal(t, "ecPool1", args[5])
			appEnabled = true
			return "", nil
		}
		return "", fmt.Errorf("unexpected command '%v'", args)
	}

	h := newTestHandler(context)

	h.CreatePool(w, req)
	assert.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, "pool 'ecPool1' created", w.Body.String())
	assert.True(t, appEnabled)
}

func TestCreatePoolHandlerFailure(t *testing.T) {
	context, _ := testContext()

	req, err := http.NewRequest("POST", "http://10.0.0.100/pool", strings.NewReader(`{"poolname":"pool1"}`))
	if err != nil {
		logger.Fatal(err)
	}

	w := httptest.NewRecorder()
	context.Executor = &exectest.MockExecutor{
		MockExecuteCommandWithOutputFile: func(debug bool, actionName string, command string, outFileArg string, args ...string) (string, error) {
			return "", fmt.Errorf("mock failure to create pool1")
		},
	}

	h := newTestHandler(context)

	h.CreatePool(w, req)
	assert.Equal(t, http.StatusInternalServerError, w.Code)
	assert.Equal(t, "", w.Body.String())
}

func TestGetClientAccessInfo(t *testing.T) {
	context, executor := testContext()

	req, err := http.NewRequest("POST", "http://10.0.0.100/image/mapinfo", nil)
	if err != nil {
		require.Fail(t, err.Error())
	}

	w := httptest.NewRecorder()
	executor.MockExecuteCommandWithOutputFile = func(debug bool, actionName string, command string, outFileArg string, args ...string) (string, error) {
		switch {
		case args[0] == "mon_status":
			response := `{"name":"mon0","rank":0,"state":"leader","election_epoch":3,"quorum":[0],"monmap":{"epoch":1,` +
				`"fsid":"22ae0d50-c4bc-4cfb-9cf4-341acbe35302","modified":"2016-09-16 04:21:51.635837","created":"2016-09-16 04:21:51.635837",` +
				`"mons":[{"rank":0,"name":"mon0","addr":"10.37.129.87:6790"}]}}`
			return response, nil
		case args[0] == "auth" && args[1] == "get-key":
			return `{"key":"AQBsCv1X5oD9GhAARHVU9N+kFRWDjyLA1dqzIg=="}`, nil
		}
		return "", nil
	}

	// get image map info and verify the response
	h := newTestHandler(context)
	h.GetClientAccessInfo(w, req)
	assert.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, `{"monAddresses":["10.37.129.87:6790"],"userName":"admin","secretKey":"AQBsCv1X5oD9GhAARHVU9N+kFRWDjyLA1dqzIg=="}`, w.Body.String())
}

func TestGetClientAccessInfoHandlerFailure(t *testing.T) {
	context, _ := testContext()

	req, err := http.NewRequest("POST", "http://10.0.0.100/client", nil)
	if err != nil {
		logger.Fatal(err)
	}

	w := httptest.NewRecorder()

	// get image map info should fail because there's no mock response set up for auth get-key
	h := newTestHandler(context)
	h.GetClientAccessInfo(w, req)
	assert.Equal(t, http.StatusInternalServerError, w.Code)
	assert.Equal(t, "", w.Body.String())
}

func testContext() (*clusterd.Context, *exectest.MockExecutor) {
	return testContextWithMons([]string{"mon0"})
}

func testContextWithMons(mons []string) (*clusterd.Context, *exectest.MockExecutor) {
	executor := &exectest.MockExecutor{}
	return &clusterd.Context{
		Executor:  executor,
		Clientset: test.New(1),
	}, executor
}
