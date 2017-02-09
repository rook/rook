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
	"net/http"
	"net/http/httptest"
	"path"
	"strings"
	"testing"

	etcd "github.com/coreos/etcd/client"
	ctx "golang.org/x/net/context"

	ceph "github.com/rook/rook/pkg/cephmgr/client"
	testceph "github.com/rook/rook/pkg/cephmgr/client/test"
	"github.com/rook/rook/pkg/cephmgr/mon"
	"github.com/rook/rook/pkg/cephmgr/test"
	"github.com/rook/rook/pkg/clusterd"
	"github.com/rook/rook/pkg/clusterd/inventory"
	"github.com/rook/rook/pkg/util"
	"github.com/rook/rook/pkg/util/sys"
	"github.com/stretchr/testify/assert"
)

const (
	SuccessGetPoolRBDResponse     = `{"pool":"rbd","pool_id":0,"size":1}{"pool":"rbd","pool_id":0,"min_size":1}{"pool":"rbd","pool_id":0,"crash_replay_interval":0}{"pool":"rbd","pool_id":0,"pg_num":2048}{"pool":"rbd","pool_id":0,"pgp_num":2048}{"pool":"rbd","pool_id":0,"crush_ruleset":0}{"pool":"rbd","pool_id":0,"hashpspool":"true"}{"pool":"rbd","pool_id":0,"nodelete":"false"}{"pool":"rbd","pool_id":0,"nopgchange":"false"}{"pool":"rbd","pool_id":0,"nosizechange":"false"}{"pool":"rbd","pool_id":0,"write_fadvise_dontneed":"false"}{"pool":"rbd","pool_id":0,"noscrub":"false"}{"pool":"rbd","pool_id":0,"nodeep-scrub":"false"}{"pool":"rbd","pool_id":0,"use_gmt_hitset":true}{"pool":"rbd","pool_id":0,"auid":0}{"pool":"rbd","pool_id":0,"min_write_recency_for_promote":0}{"pool":"rbd","pool_id":0,"fast_read":0}{"pool":"rbd","pool_id":0}{"pool":"rbd","pool_id":0}{"pool":"rbd","pool_id":0}{"pool":"rbd","pool_id":0}{"pool":"rbd","pool_id":0}{"pool":"rbd","pool_id":0}`
	SuccessGetPoolECPool1Response = `{"pool":"ecPool1","pool_id":1,"size":3}{"pool":"ecPool1","pool_id":1,"min_size":3}{"pool":"ecPool1","pool_id":1,"crash_replay_interval":0}{"pool":"ecPool1","pool_id":1,"pg_num":100}{"pool":"ecPool1","pool_id":1,"pgp_num":100}{"pool":"ecPool1","pool_id":1,"crush_ruleset":1}{"pool":"ecPool1","pool_id":1,"hashpspool":"true"}{"pool":"ecPool1","pool_id":1,"nodelete":"false"}{"pool":"ecPool1","pool_id":1,"nopgchange":"false"}{"pool":"ecPool1","pool_id":1,"nosizechange":"false"}{"pool":"ecPool1","pool_id":1,"write_fadvise_dontneed":"false"}{"pool":"ecPool1","pool_id":1,"noscrub":"false"}{"pool":"ecPool1","pool_id":1,"nodeep-scrub":"false"}{"pool":"ecPool1","pool_id":1,"use_gmt_hitset":true}{"pool":"ecPool1","pool_id":1,"auid":0}{"pool":"ecPool1","pool_id":1,"erasure_code_profile":"ecPool1_ecprofile"}{"pool":"ecPool1","pool_id":1,"min_write_recency_for_promote":0}{"pool":"ecPool1","pool_id":1,"fast_read":0}{"pool":"ecPool1","pool_id":1}{"pool":"ecPool1","pool_id":1}{"pool":"ecPool1","pool_id":1}{"pool":"ecPool1","pool_id":1}{"pool":"ecPool1","pool_id":1}{"pool":"ecPool1","pool_id":1}`
)

func newTestHandler(context *clusterd.Context, connFactory mon.ConnectionFactory, cephFactory ceph.ConnectionFactory) *Handler {
	return newHandler(context, &Config{ConnFactory: connFactory, CephFactory: cephFactory, ClusterHandler: NewEtcdHandler(context)})
}

func TestGetNodesHandler(t *testing.T) {
	nodeID := "node1"
	etcdClient := util.NewMockEtcdClient()
	context := &clusterd.Context{EtcdClient: etcdClient}

	req, err := http.NewRequest("GET", "http://10.0.0.100/node", nil)
	if err != nil {
		logger.Fatal(err)
	}

	w := httptest.NewRecorder()
	etcdClient.SetValue("/rook/services/ceph/name", "cluster5")
	cephFactory := &testceph.MockConnectionFactory{Fsid: "myfsid", SecretKey: "mykey"}
	h := newTestHandler(context, &test.MockConnectionFactory{}, cephFactory)

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
	etcdClient := util.NewMockEtcdClient()
	context := &clusterd.Context{EtcdClient: etcdClient}
	req, err := http.NewRequest("GET", "http://10.0.0.100/node", nil)
	if err != nil {
		logger.Fatal(err)
	}

	w := httptest.NewRecorder()

	// failure during node lookup, should return an error status code
	etcdClient.MockGet = func(context ctx.Context, key string, opts *etcd.GetOptions) (*etcd.Response, error) {
		return nil, fmt.Errorf("mock etcd GET error")
	}
	cephFactory := &testceph.MockConnectionFactory{Fsid: "myfsid", SecretKey: "mykey"}
	h := newTestHandler(context, &test.MockConnectionFactory{}, cephFactory)
	h.GetNodes(w, req)
	assert.Equal(t, http.StatusInternalServerError, w.Code)
	assert.Equal(t, ``, w.Body.String())
}

func TestGetMonsHandler(t *testing.T) {
	etcdClient := util.NewMockEtcdClient()
	context := &clusterd.Context{EtcdClient: etcdClient}
	cephFactory := &testceph.MockConnectionFactory{Fsid: "myfsid", SecretKey: "mykey"}

	req, err := http.NewRequest("GET", "http://10.0.0.100/mon", nil)
	assert.Nil(t, err)

	// first return no mons
	w := httptest.NewRecorder()

	// no mons will be returned, should be empty output
	h := newTestHandler(context, &test.MockConnectionFactory{}, cephFactory)
	h.GetMonitors(w, req)
	assert.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, `[]`, w.Body.String())

	// now return some monitors from etcd
	key := "/rook/services/ceph/monitor/desired/a"
	etcdClient.SetValue(path.Join(key, "id"), "mon0")
	etcdClient.SetValue(path.Join(key, "ipaddress"), "1.2.3.4")
	etcdClient.SetValue(path.Join(key, "port"), "8765")

	// monitors should be returned now, verify the output
	w = httptest.NewRecorder()
	h = newTestHandler(context, &test.MockConnectionFactory{}, cephFactory)
	h.GetMonitors(w, req)
	assert.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, "{\"status\":{\"quorum\":[0],\"monmap\":{\"mons\":[{\"name\":\"mon0\",\"rank\":0,\"addr\":\"10.37.129.87:6790\"}]}},\"desired\":[{\"name\":\"mon0\",\"endpoint\":\"1.2.3.4:8765\"}]}", w.Body.String())
}

func TestGetPoolsHandler(t *testing.T) {
	etcdClient := util.NewMockEtcdClient()
	context := &clusterd.Context{EtcdClient: etcdClient}

	req, err := http.NewRequest("GET", "http://10.0.0.100/pool", nil)
	if err != nil {
		logger.Fatal(err)
	}

	// first return no storage pools
	w := httptest.NewRecorder()
	cephFactory := &testceph.MockConnectionFactory{Fsid: "myfsid", SecretKey: "mykey"}
	connFactory := &test.MockConnectionFactory{}
	cephFactory.Conn = &testceph.MockConnection{
		MockMonCommand: func(args []byte) (buffer []byte, info string, err error) {
			return []byte(`[]`), "", nil
		},
	}
	connFactory.MockConnectAsAdmin = func(context *clusterd.Context, cephFactory ceph.ConnectionFactory) (ceph.Connection, error) {
		return cephFactory.NewConnWithClusterAndUser("mycluster", "admin")
	}

	// no storage pools will be returned, should be empty output
	h := newTestHandler(context, connFactory, cephFactory)
	h.GetPools(w, req)
	assert.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, `[]`, w.Body.String())

	// now return some storage pools from the ceph connection
	w = httptest.NewRecorder()
	cephFactory.Conn = &testceph.MockConnection{
		MockMonCommand: func(args []byte) (buffer []byte, info string, err error) {
			switch {
			case strings.Index(string(args), "osd lspools") != -1:
				return []byte(`[{"poolnum":0,"poolname":"rbd"},{"poolnum":1,"poolname":"ecPool1"}]`), "info", nil
			case strings.Index(string(args), "osd pool get") != -1:
				if strings.Index(string(args), "rbd") != -1 {
					return []byte(SuccessGetPoolRBDResponse), "info", nil
				} else if strings.Index(string(args), "ecPool1") != -1 {
					return []byte(SuccessGetPoolECPool1Response), "info", nil
				}
			case strings.Index(string(args), "osd erasure-code-profile ls") != -1:
				return []byte(`["default","ecPool1_ecprofile"]`), "info", nil
			case strings.Index(string(args), "osd erasure-code-profile get") != -1:
				if strings.Index(string(args), "default") != -1 {
					return []byte(`{"k":"2","m":"1","plugin":"jerasure","technique":"reed_sol_van"}`), "info", nil
				} else if strings.Index(string(args), "ecPool1") != -1 {
					return []byte(`{"jerasure-per-chunk-alignment":"false","k":"2","m":"1","plugin":"jerasure","ruleset-failure-domain":"osd","ruleset-root":"default","technique":"reed_sol_van","w":"8"}`), "info", nil
				}
			}
			return nil, "", fmt.Errorf("unexpected mon_command '%s'", string(args))
		},
	}

	// storage pools should be returned now, verify the output
	h = newTestHandler(context, connFactory, cephFactory)
	h.GetPools(w, req)
	assert.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, "[{\"poolName\":\"rbd\",\"poolNum\":0,\"type\":0,\"replicationConfig\":{\"size\":1},\"erasureCodedConfig\":{\"dataChunkCount\":0,\"codingChunkCount\":0,\"algorithm\":\"\"}},{\"poolName\":\"ecPool1\",\"poolNum\":1,\"type\":1,\"replicationConfig\":{\"size\":0},\"erasureCodedConfig\":{\"dataChunkCount\":2,\"codingChunkCount\":1,\"algorithm\":\"jerasure::reed_sol_van\"}}]", w.Body.String())
}

func TestGetPoolsHandlerFailure(t *testing.T) {
	etcdClient := util.NewMockEtcdClient()
	context := &clusterd.Context{EtcdClient: etcdClient}

	req, err := http.NewRequest("GET", "http://10.0.0.100/pool", nil)
	if err != nil {
		logger.Fatal(err)
	}

	// encounter an error during GetPools
	w := httptest.NewRecorder()
	cephFactory := &testceph.MockConnectionFactory{Fsid: "myfsid", SecretKey: "mykey"}
	connFactory := &test.MockConnectionFactory{}
	connFactory.MockConnectAsAdmin = func(context *clusterd.Context, cephFactory ceph.ConnectionFactory) (ceph.Connection, error) {
		return nil, fmt.Errorf("mock error for connect as admin")
	}

	h := newTestHandler(context, connFactory, cephFactory)
	h.GetPools(w, req)
	assert.Equal(t, http.StatusInternalServerError, w.Code)
	assert.Equal(t, "", w.Body.String())
}

func TestCreatePoolHandler(t *testing.T) {
	etcdClient := util.NewMockEtcdClient()
	context := &clusterd.Context{EtcdClient: etcdClient}

	req, err := http.NewRequest("POST", "http://10.0.0.100/pool",
		strings.NewReader(`{"poolName":"ecPool1","poolNum":0,"type":1,"replicationConfig":{"size":0},"erasureCodedConfig":{"dataChunkCount":2,"codingChunkCount":1,"algorithm":""}}`))
	if err != nil {
		logger.Fatal(err)
	}

	w := httptest.NewRecorder()
	cephFactory := &testceph.MockConnectionFactory{Fsid: "myfsid", SecretKey: "mykey"}
	connFactory := &test.MockConnectionFactory{}
	cephFactory.Conn = &testceph.MockConnection{
		MockMonCommand: func(args []byte) (buffer []byte, info string, err error) {
			switch {
			case strings.Index(string(args), "osd erasure-code-profile get") != -1:
				if strings.Index(string(args), "default") != -1 {
					return []byte(`{"k":"2","m":"1","plugin":"jerasure","technique":"reed_sol_van"}`), "info", nil
				}
			case strings.Index(string(args), "osd erasure-code-profile set") != -1:
				return []byte(""), "", nil
			case strings.Index(string(args), "osd pool create") != -1:
				return []byte(""), "pool 'ecPool1' created", nil

			}
			return nil, "", fmt.Errorf("unexpected mon_command '%s'", string(args))
		},
	}
	connFactory.MockConnectAsAdmin = func(context *clusterd.Context, cephFactory ceph.ConnectionFactory) (ceph.Connection, error) {
		return cephFactory.NewConnWithClusterAndUser("mycluster", "admin")
	}

	h := newTestHandler(context, connFactory, cephFactory)

	h.CreatePool(w, req)
	assert.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, "pool 'ecPool1' created", w.Body.String())
}

func TestCreatePoolHandlerFailure(t *testing.T) {
	etcdClient := util.NewMockEtcdClient()
	context := &clusterd.Context{EtcdClient: etcdClient}

	req, err := http.NewRequest("POST", "http://10.0.0.100/pool", strings.NewReader(`{"poolname":"pool1"}`))
	if err != nil {
		logger.Fatal(err)
	}

	w := httptest.NewRecorder()
	cephFactory := &testceph.MockConnectionFactory{Fsid: "myfsid", SecretKey: "mykey"}
	connFactory := &test.MockConnectionFactory{}
	cephFactory.Conn = &testceph.MockConnection{
		MockMonCommand: func(args []byte) (buffer []byte, info string, err error) {
			errMsg := "mock failure to create pool1"
			return []byte(""), errMsg, fmt.Errorf(errMsg)
		},
	}
	connFactory.MockConnectAsAdmin = func(context *clusterd.Context, cephFactory ceph.ConnectionFactory) (ceph.Connection, error) {
		return cephFactory.NewConnWithClusterAndUser("mycluster", "admin")
	}

	h := newTestHandler(context, connFactory, cephFactory)

	h.CreatePool(w, req)
	assert.Equal(t, http.StatusInternalServerError, w.Code)
	assert.Equal(t, "", w.Body.String())
}

func TestGetImagesHandler(t *testing.T) {
	etcdClient := util.NewMockEtcdClient()
	context := &clusterd.Context{EtcdClient: etcdClient}

	req, err := http.NewRequest("GET", "http://10.0.0.100/image", nil)
	if err != nil {
		logger.Fatal(err)
	}

	// first return no storage pools, which means no images will be returned either
	w := httptest.NewRecorder()
	cephFactory := &testceph.MockConnectionFactory{Fsid: "myfsid", SecretKey: "mykey"}
	cephFactory.Conn = &testceph.MockConnection{
		MockMonCommand: func(args []byte) (buffer []byte, info string, err error) {
			return []byte(`[]`), "", nil
		},
	}
	connFactory := &test.MockConnectionFactory{}
	connFactory.MockConnectAsAdmin = func(context *clusterd.Context, cephFactory ceph.ConnectionFactory) (ceph.Connection, error) {
		return cephFactory.NewConnWithClusterAndUser("mycluster", "admin")
	}

	// no images will be returned, should be empty output
	h := newTestHandler(context, connFactory, cephFactory)
	h.GetImages(w, req)
	assert.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, `[]`, w.Body.String())

	// now return some storage pools and images from the ceph connection
	w = httptest.NewRecorder()
	cephFactory.Conn = &testceph.MockConnection{
		MockMonCommand: func(args []byte) (buffer []byte, info string, err error) {
			switch {
			case strings.Index(string(args), "osd lspools") != -1:
				return []byte(`[{"poolnum":0,"poolname":"pool0"},{"poolnum":1,"poolname":"pool1"}]`), "info", nil
			}
			return nil, "", fmt.Errorf("unexpected mon_command '%s'", string(args))
		},
		MockOpenIOContext: func(pool string) (ceph.IOContext, error) {
			return &testceph.MockIOContext{
				MockGetImageNames: func() (names []string, err error) {
					return []string{fmt.Sprintf("image1 - %s", pool)}, nil
				},
				MockGetImage: func(name string) ceph.Image {
					return &testceph.MockImage{
						MockName: name,
						MockStat: func() (info *ceph.ImageInfo, err error) {
							return &ceph.ImageInfo{
								Size: 100,
							}, nil
						},
					}
				},
			}, nil
		},
	}

	// verify that the expected images are returned
	h = newTestHandler(context, connFactory, cephFactory)
	h.GetImages(w, req)
	assert.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, "[{\"imageName\":\"image1 - pool0\",\"poolName\":\"pool0\",\"size\":100,\"device\":\"\",\"mountPoint\":\"\"},{\"imageName\":\"image1 - pool1\",\"poolName\":\"pool1\",\"size\":100,\"device\":\"\",\"mountPoint\":\"\"}]", w.Body.String())
}

func TestGetImagesHandlerFailure(t *testing.T) {
	etcdClient := util.NewMockEtcdClient()
	context := &clusterd.Context{EtcdClient: etcdClient}

	req, err := http.NewRequest("GET", "http://10.0.0.100/image", nil)
	if err != nil {
		logger.Fatal(err)
	}

	cephFactory := &testceph.MockConnectionFactory{
		Fsid:      "myfsid",
		SecretKey: "mykey",
		Conn: &testceph.MockConnection{
			MockMonCommand: func(args []byte) (buffer []byte, info string, err error) {
				return nil, "mock error", fmt.Errorf("mock error for list pools")
			},
		},
	}
	connFactory := &test.MockConnectionFactory{}
	connFactory.MockConnectAsAdmin = func(context *clusterd.Context, cephFactory ceph.ConnectionFactory) (ceph.Connection, error) {
		return cephFactory.NewConnWithClusterAndUser("mycluster", "admin")
	}

	// GetImages should fail due to the mocked error for listing pools
	w := httptest.NewRecorder()
	h := newTestHandler(context, connFactory, cephFactory)
	h.GetImages(w, req)
	assert.Equal(t, http.StatusInternalServerError, w.Code)
	assert.Equal(t, ``, w.Body.String())
}

func TestCreateImageHandler(t *testing.T) {
	etcdClient := util.NewMockEtcdClient()
	context := &clusterd.Context{EtcdClient: etcdClient}

	req, err := http.NewRequest("POST", "http://10.0.0.100/image", nil)
	if err != nil {
		logger.Fatal(err)
	}

	w := httptest.NewRecorder()
	cephFactory := &testceph.MockConnectionFactory{Fsid: "myfsid", SecretKey: "mykey"}
	connFactory := &test.MockConnectionFactory{}
	connFactory.MockConnectAsAdmin = func(context *clusterd.Context, cephFactory ceph.ConnectionFactory) (ceph.Connection, error) {
		return cephFactory.NewConnWithClusterAndUser("mycluster", "admin")
	}

	// image is missing from request body, should be bad request
	h := newTestHandler(context, connFactory, cephFactory)
	h.CreateImage(w, req)
	assert.Equal(t, http.StatusBadRequest, w.Code)
	assert.Equal(t, ``, w.Body.String())

	// request body exists but it's bad json, should be bad request
	req, err = http.NewRequest("POST", "http://10.0.0.100/image", strings.NewReader(`bad json`))
	if err != nil {
		logger.Fatal(err)
	}
	w = httptest.NewRecorder()
	h = newTestHandler(context, connFactory, cephFactory)
	h.CreateImage(w, req)
	assert.Equal(t, http.StatusBadRequest, w.Code)
	assert.Equal(t, ``, w.Body.String())

	// missing fields for the image passed via request body, should be bad request
	req, err = http.NewRequest("POST", "http://10.0.0.100/image", strings.NewReader(`{"imageName":"myImage1"}`))
	if err != nil {
		logger.Fatal(err)
	}
	w = httptest.NewRecorder()
	h = newTestHandler(context, connFactory, cephFactory)
	h.CreateImage(w, req)
	assert.Equal(t, http.StatusBadRequest, w.Code)
	assert.Equal(t, ``, w.Body.String())

	// well formed successful request to create an image
	req, err = http.NewRequest("POST", "http://10.0.0.100/image", strings.NewReader(`{"imageName":"myImage1","poolName":"myPool1","size":1024}`))
	if err != nil {
		logger.Fatal(err)
	}
	cephFactory.Conn = &testceph.MockConnection{
		MockOpenIOContext: func(pool string) (ceph.IOContext, error) {
			return &testceph.MockIOContext{
				MockCreateImage: func(name string, size uint64, order int, args ...uint64) (image ceph.Image, err error) {
					return &testceph.MockImage{MockName: name}, nil
				},
			}, nil
		},
	}
	w = httptest.NewRecorder()
	h = newTestHandler(context, connFactory, cephFactory)
	h.CreateImage(w, req)
	assert.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, `succeeded created image myImage1`, w.Body.String())
}

func TestCreateImageHandlerFailure(t *testing.T) {
	etcdClient := util.NewMockEtcdClient()
	context := &clusterd.Context{EtcdClient: etcdClient}

	req, err := http.NewRequest("POST", "http://10.0.0.100/image", strings.NewReader(`{"imageName":"myImage1","poolName":"myPool1","size":1024}`))
	if err != nil {
		logger.Fatal(err)
	}

	w := httptest.NewRecorder()
	cephFactory := &testceph.MockConnectionFactory{Fsid: "myfsid", SecretKey: "mykey"}
	connFactory := &test.MockConnectionFactory{}
	connFactory.MockConnectAsAdmin = func(context *clusterd.Context, cephFactory ceph.ConnectionFactory) (ceph.Connection, error) {
		return cephFactory.NewConnWithClusterAndUser("mycluster", "admin")
	}
	cephFactory.Conn = &testceph.MockConnection{
		MockOpenIOContext: func(pool string) (ceph.IOContext, error) {
			return &testceph.MockIOContext{
				// mock a failure in the create image call
				MockCreateImage: func(name string, size uint64, order int, args ...uint64) (image ceph.Image, err error) {
					return &testceph.MockImage{}, fmt.Errorf("mock failure to create image")
				},
			}, nil
		},
	}

	// create image request should fail while creating the image
	h := newTestHandler(context, connFactory, cephFactory)
	h.CreateImage(w, req)
	assert.Equal(t, http.StatusInternalServerError, w.Code)
	assert.Equal(t, ``, w.Body.String())
}

func TestGetClientAccessInfo(t *testing.T) {
	etcdClient := util.NewMockEtcdClient()
	context := &clusterd.Context{EtcdClient: etcdClient}

	req, err := http.NewRequest("POST", "http://10.0.0.100/image/mapinfo", nil)
	if err != nil {
		logger.Fatal(err)
	}

	w := httptest.NewRecorder()
	cephFactory := &testceph.MockConnectionFactory{Fsid: "myfsid", SecretKey: "mykey"}
	connFactory := &test.MockConnectionFactory{}
	connFactory.MockConnectAsAdmin = func(context *clusterd.Context, cephFactory ceph.ConnectionFactory) (ceph.Connection, error) {
		return cephFactory.NewConnWithClusterAndUser("mycluster", "admin")
	}
	cephFactory.Conn = &testceph.MockConnection{
		MockMonCommand: func(args []byte) (buffer []byte, info string, err error) {
			switch {
			case strings.Index(string(args), "mon_status") != -1:
				return []byte(testceph.SuccessfulMonStatusResponse), "info", nil
			case strings.Index(string(args), "auth get-key") != -1:
				return []byte(`{"key":"AQBsCv1X5oD9GhAARHVU9N+kFRWDjyLA1dqzIg=="}`), "info", nil
			}
			return nil, "", nil
		},
	}

	// get image map info and verify the response
	h := newTestHandler(context, connFactory, cephFactory)
	h.GetClientAccessInfo(w, req)
	assert.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, "{\"monAddresses\":[\"10.37.129.87:6790\"],\"userName\":\"admin\",\"secretKey\":\"AQBsCv1X5oD9GhAARHVU9N+kFRWDjyLA1dqzIg==\"}", w.Body.String())
}

func TestGetClientAccessInfoHandlerFailure(t *testing.T) {
	etcdClient := util.NewMockEtcdClient()
	context := &clusterd.Context{EtcdClient: etcdClient}

	req, err := http.NewRequest("POST", "http://10.0.0.100/client", nil)
	if err != nil {
		logger.Fatal(err)
	}

	w := httptest.NewRecorder()
	cephFactory := &testceph.MockConnectionFactory{Fsid: "myfsid", SecretKey: "mykey"}
	connFactory := &test.MockConnectionFactory{}
	connFactory.MockConnectAsAdmin = func(context *clusterd.Context, cephFactory ceph.ConnectionFactory) (ceph.Connection, error) {
		return cephFactory.NewConnWithClusterAndUser("mycluster", "admin")
	}
	cephFactory.Conn = &testceph.MockConnection{}

	// get image map info should fail becuase there's no mock response set up for auth get-key
	h := newTestHandler(context, connFactory, cephFactory)
	h.GetClientAccessInfo(w, req)
	assert.Equal(t, http.StatusInternalServerError, w.Code)
	assert.Equal(t, "", w.Body.String())
}
