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

	etcd "github.com/coreos/etcd/client"
	cephclienttest "github.com/rook/rook/pkg/ceph/client/test"
	"github.com/rook/rook/pkg/ceph/mon"
	cephtest "github.com/rook/rook/pkg/ceph/test"
	"github.com/rook/rook/pkg/clusterd"
	"github.com/rook/rook/pkg/clusterd/inventory"
	"github.com/rook/rook/pkg/util"
	exectest "github.com/rook/rook/pkg/util/exec/test"
	"github.com/rook/rook/pkg/util/sys"
	"github.com/stretchr/testify/assert"
	ctx "golang.org/x/net/context"
)

const (
	SuccessGetPoolRBDResponse     = `{"pool":"rbd","pool_id":0,"size":1}{"pool":"rbd","pool_id":0,"min_size":1}{"pool":"rbd","pool_id":0,"crash_replay_interval":0}{"pool":"rbd","pool_id":0,"pg_num":2048}{"pool":"rbd","pool_id":0,"pgp_num":2048}{"pool":"rbd","pool_id":0,"crush_ruleset":0}{"pool":"rbd","pool_id":0,"hashpspool":"true"}{"pool":"rbd","pool_id":0,"nodelete":"false"}{"pool":"rbd","pool_id":0,"nopgchange":"false"}{"pool":"rbd","pool_id":0,"nosizechange":"false"}{"pool":"rbd","pool_id":0,"write_fadvise_dontneed":"false"}{"pool":"rbd","pool_id":0,"noscrub":"false"}{"pool":"rbd","pool_id":0,"nodeep-scrub":"false"}{"pool":"rbd","pool_id":0,"use_gmt_hitset":true}{"pool":"rbd","pool_id":0,"auid":0}{"pool":"rbd","pool_id":0,"min_write_recency_for_promote":0}{"pool":"rbd","pool_id":0,"fast_read":0}{"pool":"rbd","pool_id":0}{"pool":"rbd","pool_id":0}{"pool":"rbd","pool_id":0}{"pool":"rbd","pool_id":0}{"pool":"rbd","pool_id":0}{"pool":"rbd","pool_id":0}`
	SuccessGetPoolECPool1Response = `{"pool":"ecPool1","pool_id":1,"size":3}{"pool":"ecPool1","pool_id":1,"min_size":3}{"pool":"ecPool1","pool_id":1,"crash_replay_interval":0}{"pool":"ecPool1","pool_id":1,"pg_num":100}{"pool":"ecPool1","pool_id":1,"pgp_num":100}{"pool":"ecPool1","pool_id":1,"crush_ruleset":1}{"pool":"ecPool1","pool_id":1,"hashpspool":"true"}{"pool":"ecPool1","pool_id":1,"nodelete":"false"}{"pool":"ecPool1","pool_id":1,"nopgchange":"false"}{"pool":"ecPool1","pool_id":1,"nosizechange":"false"}{"pool":"ecPool1","pool_id":1,"write_fadvise_dontneed":"false"}{"pool":"ecPool1","pool_id":1,"noscrub":"false"}{"pool":"ecPool1","pool_id":1,"nodeep-scrub":"false"}{"pool":"ecPool1","pool_id":1,"use_gmt_hitset":true}{"pool":"ecPool1","pool_id":1,"auid":0}{"pool":"ecPool1","pool_id":1,"erasure_code_profile":"ecPool1_ecprofile"}{"pool":"ecPool1","pool_id":1,"min_write_recency_for_promote":0}{"pool":"ecPool1","pool_id":1,"fast_read":0}{"pool":"ecPool1","pool_id":1}{"pool":"ecPool1","pool_id":1}{"pool":"ecPool1","pool_id":1}{"pool":"ecPool1","pool_id":1}{"pool":"ecPool1","pool_id":1}{"pool":"ecPool1","pool_id":1}`
)

func newTestHandler(context *clusterd.Context) *Handler {
	clusterInfo, _ := mon.LoadClusterInfo(context.EtcdClient)
	return newHandler(context, &Config{ClusterHandler: NewEtcdHandler(context), ClusterInfo: clusterInfo})
}

func TestRegisterMetrics(t *testing.T) {
	context, _, _ := testContext()
	defer os.RemoveAll(context.ConfigDir)

	// create and init a new handler.  even though the first attempt fails, it should retry and return no error
	h := newTestHandler(context)
	err := h.RegisterMetrics(0)
	assert.Nil(t, err)
}

func TestGetNodesHandler(t *testing.T) {
	nodeID := "node1"
	context, etcdClient, _ := testContext()
	defer os.RemoveAll(context.ConfigDir)

	req, err := http.NewRequest("GET", "http://10.0.0.100/node", nil)
	if err != nil {
		logger.Fatal(err)
	}

	w := httptest.NewRecorder()
	etcdClient.SetValue("/rook/services/ceph/name", "cluster5")
	h := newTestHandler(context)

	// no nodes discovered, should return empty set
	h.GetNodes(w, req)
	assert.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, `[]`, w.Body.String())

	// add the disks to etcd
	disks := []inventory.Disk{
		inventory.Disk{Type: sys.DiskType, Size: 50, Rotational: true, Empty: true},
		inventory.Disk{Type: sys.DiskType, Size: 100, Rotational: false, Empty: true},
	}
	output, _ := json.Marshal(disks)
	etcdClient.SetValue(path.Join(inventory.NodesConfigKey, nodeID, "disks"), string(output))

	// set up a discovered node in etcd
	inventory.SetIPAddress(etcdClient, nodeID, "1.2.3.4", "10.0.0.11")
	inventory.SetLocation(etcdClient, nodeID, "root=default,dc=datacenter1")
	appliedOSDKey := path.Join("/rook/services/ceph/osd/applied", nodeID)
	etcdClient.SetValue(path.Join(appliedOSDKey, "12", "disk-uuid"), "123d4869-29ee-4bfd-bf21-dfd597bd222e")
	etcdClient.SetValue(path.Join(appliedOSDKey, "13", "disk-uuid"), "321d4869-29ee-4bfd-bf21-dfd597bdffff")

	// since a node exists (with storage), it should be returned now
	w = httptest.NewRecorder()
	h.GetNodes(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, "[{\"nodeId\":\"node1\",\"clusterName\":\"cluster5\",\"publicIp\":\"1.2.3.4\",\"privateIp\":\"10.0.0.11\",\"storage\":150,\"lastUpdated\":31536000000000000,\"state\":1,\"location\":\"root=default,dc=datacenter1\"}]",
		w.Body.String())
}

func TestGetNodesHandlerFailure(t *testing.T) {
	context, etcdClient, _ := testContext()
	defer os.RemoveAll(context.ConfigDir)

	req, err := http.NewRequest("GET", "http://10.0.0.100/node", nil)
	if err != nil {
		logger.Fatal(err)
	}

	w := httptest.NewRecorder()

	// failure during node lookup, should return an error status code
	etcdClient.MockGet = func(context ctx.Context, key string, opts *etcd.GetOptions) (*etcd.Response, error) {
		return nil, fmt.Errorf("mock etcd GET error")
	}
	h := newTestHandler(context)
	h.GetNodes(w, req)
	assert.Equal(t, http.StatusInternalServerError, w.Code)
	assert.Equal(t, ``, w.Body.String())
}

func TestGetMonsHandler(t *testing.T) {
	context, etcdClient, executor := testContextWithMons([]string{})
	defer os.RemoveAll(context.ConfigDir)

	req, err := http.NewRequest("GET", "http://10.0.0.100/mon", nil)
	assert.Nil(t, err)

	// first return no mons
	w := httptest.NewRecorder()

	// no mons will be returned, should be empty output
	h := newTestHandler(context)
	h.GetMonitors(w, req)
	assert.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, `[]`, w.Body.String())

	// now return some monitors from etcd
	key := "/rook/services/ceph/monitor/desired/a"
	etcdClient.SetValue(path.Join(key, "id"), "mon0")
	etcdClient.SetValue(path.Join(key, "ipaddress"), "1.2.3.4")
	etcdClient.SetValue(path.Join(key, "port"), "8765")

	executor.MockExecuteCommandWithOutput = func(actionName string, command string, args ...string) (string, error) {
		if args[0] == "mon_status" {
			return cephclienttest.MonInQuorumResponse(), nil
		}
		return "", fmt.Errorf("unrecognized command: %+v", args)
	}

	// monitors should be returned now, verify the output
	w = httptest.NewRecorder()
	h = newTestHandler(context)
	h.GetMonitors(w, req)
	assert.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, "{\"status\":{\"quorum\":[0],\"monmap\":{\"mons\":[{\"name\":\"mon0\",\"rank\":0,\"addr\":\"1.2.3.4\"}]}},\"desired\":[{\"name\":\"mon0\",\"endpoint\":\"1.2.3.4:8765\"}]}", w.Body.String())
}

func TestGetPoolsHandler(t *testing.T) {
	context, _, executor := testContext()
	defer os.RemoveAll(context.ConfigDir)

	req, err := http.NewRequest("GET", "http://10.0.0.100/pool", nil)
	if err != nil {
		logger.Fatal(err)
	}

	// first return no storage pools
	executor.MockExecuteCommandWithOutput = func(actionName string, command string, args ...string) (string, error) {
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
	executor.MockExecuteCommandWithOutput = func(actionName string, command string, args ...string) (string, error) {
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
	assert.Equal(t, "[{\"poolName\":\"rbd\",\"poolNum\":0,\"type\":0,\"replicationConfig\":{\"size\":1},\"erasureCodedConfig\":{\"dataChunkCount\":0,\"codingChunkCount\":0,\"algorithm\":\"\"}},{\"poolName\":\"ecPool1\",\"poolNum\":1,\"type\":1,\"replicationConfig\":{\"size\":0},\"erasureCodedConfig\":{\"dataChunkCount\":2,\"codingChunkCount\":1,\"algorithm\":\"jerasure::reed_sol_van\"}}]", w.Body.String())
}

func TestGetPoolsHandlerFailure(t *testing.T) {
	context, _, _ := testContext()
	defer os.RemoveAll(context.ConfigDir)

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
	context, _, executor := testContext()
	defer os.RemoveAll(context.ConfigDir)

	req, err := http.NewRequest("POST", "http://10.0.0.100/pool",
		strings.NewReader(`{"poolName":"ecPool1","poolNum":0,"type":1,"replicationConfig":{"size":0},"erasureCodedConfig":{"dataChunkCount":2,"codingChunkCount":1,"algorithm":""}}`))
	if err != nil {
		logger.Fatal(err)
	}

	w := httptest.NewRecorder()
	executor.MockExecuteCommandWithOutput = func(actionName string, command string, args ...string) (string, error) {
		switch {
		case args[1] == "erasure-code-profile" && args[2] == "get":
			if args[3] == "default" {
				return `{"k":"2","m":"1","plugin":"jerasure","technique":"reed_sol_van"}`, nil
			}
		case args[1] == "erasure-code-profile" && args[2] == "set":
			return "", nil
		case args[1] == "pool" && args[2] == "create":
			return "pool 'ecPool1' created", nil

		}
		return "", fmt.Errorf("unexpected mon_command '%v'", args)
	}

	h := newTestHandler(context)

	h.CreatePool(w, req)
	assert.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, "pool 'ecPool1' created", w.Body.String())
}

func TestCreatePoolHandlerFailure(t *testing.T) {
	context, _, _ := testContext()
	defer os.RemoveAll(context.ConfigDir)

	req, err := http.NewRequest("POST", "http://10.0.0.100/pool", strings.NewReader(`{"poolname":"pool1"}`))
	if err != nil {
		logger.Fatal(err)
	}

	w := httptest.NewRecorder()
	context.Executor = &exectest.MockExecutor{
		MockExecuteCommandWithOutput: func(actionName string, command string, args ...string) (string, error) {
			return "", fmt.Errorf("mock failure to create pool1")
		},
	}

	h := newTestHandler(context)

	h.CreatePool(w, req)
	assert.Equal(t, http.StatusInternalServerError, w.Code)
	assert.Equal(t, "", w.Body.String())
}

func TestGetClientAccessInfo(t *testing.T) {
	context, _, executor := testContext()
	defer os.RemoveAll(context.ConfigDir)

	req, err := http.NewRequest("POST", "http://10.0.0.100/image/mapinfo", nil)
	if err != nil {
		logger.Fatal(err)
	}

	w := httptest.NewRecorder()
	executor.MockExecuteCommandWithOutput = func(actionName string, command string, args ...string) (string, error) {
		switch {
		case args[0] == "mon_status":
			response := "{\"name\":\"mon0\",\"rank\":0,\"state\":\"leader\",\"election_epoch\":3,\"quorum\":[0],\"monmap\":{\"epoch\":1," +
				"\"fsid\":\"22ae0d50-c4bc-4cfb-9cf4-341acbe35302\",\"modified\":\"2016-09-16 04:21:51.635837\",\"created\":\"2016-09-16 04:21:51.635837\"," +
				"\"mons\":[{\"rank\":0,\"name\":\"mon0\",\"addr\":\"10.37.129.87:6790\"}]}}"
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
	assert.Equal(t, "{\"monAddresses\":[\"10.37.129.87:6790\"],\"userName\":\"admin\",\"secretKey\":\"AQBsCv1X5oD9GhAARHVU9N+kFRWDjyLA1dqzIg==\"}", w.Body.String())
}

func TestGetClientAccessInfoHandlerFailure(t *testing.T) {
	context, _, _ := testContext()
	defer os.RemoveAll(context.ConfigDir)

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

func testContext() (*clusterd.Context, *util.MockEtcdClient, *exectest.MockExecutor) {
	return testContextWithMons([]string{"mon0"})
}

func testContextWithMons(mons []string) (*clusterd.Context, *util.MockEtcdClient, *exectest.MockExecutor) {
	etcdClient := util.NewMockEtcdClient()
	configDir, _ := ioutil.TempDir("", "")
	cephtest.CreateClusterInfo(etcdClient, configDir, mons)
	executor := &exectest.MockExecutor{}
	return &clusterd.Context{
		DirectContext: clusterd.DirectContext{EtcdClient: etcdClient},
		Executor:      executor,
		ConfigDir:     configDir,
	}, etcdClient, executor
}
