package api

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"net/http/httptest"
	"path"
	"strings"
	"testing"

	etcd "github.com/coreos/etcd/client"
	ctx "golang.org/x/net/context"

	ceph "github.com/quantum/castle/pkg/cephmgr/client"
	testceph "github.com/quantum/castle/pkg/cephmgr/client/test"
	"github.com/quantum/castle/pkg/cephmgr/test"
	"github.com/quantum/castle/pkg/clusterd/inventory"
	"github.com/quantum/castle/pkg/util"
	"github.com/stretchr/testify/assert"
)

func TestAddRemoveDeviceHandler(t *testing.T) {
	req, err := http.NewRequest("POST", "http://10.0.0.100/device", strings.NewReader(`{"name":"foo"}`))
	assert.Nil(t, err)

	w := httptest.NewRecorder()
	etcdClient := util.NewMockEtcdClient()
	h := NewHandler(etcdClient, nil, nil)

	// missing the node id
	h.AddDevice(w, req)
	assert.Equal(t, http.StatusBadRequest, w.Code)

	// successful request
	req, err = http.NewRequest("POST", "http://10.0.0.100/device", strings.NewReader(`{"name":"foo","nodeId":"123"}`))
	assert.Nil(t, err)
	h = NewHandler(etcdClient, nil, nil)
	w = httptest.NewRecorder()
	h.AddDevice(w, req)
	assert.Equal(t, http.StatusOK, w.Code)

	devices := etcdClient.GetChildDirs("/castle/services/ceph/osd/desired/123/device")
	assert.Equal(t, 1, devices.Count())
	assert.True(t, devices.Contains("foo"))

	// remove the device
	req, err = http.NewRequest("POST", "http://10.0.0.100/device/remove", strings.NewReader(`{"name":"foo","nodeId":"123"}`))
	assert.Nil(t, err)
	h = NewHandler(etcdClient, nil, nil)
	w = httptest.NewRecorder()
	h.RemoveDevice(w, req)
	assert.Equal(t, http.StatusOK, w.Code)

	// check the desired state
	devices = etcdClient.GetChildDirs("/castle/services/ceph/osd/desired/123/device")
	assert.Equal(t, 0, devices.Count())
}

func TestRemoveDeviceHandler(t *testing.T) {
}

func TestGetNodesHandler(t *testing.T) {
	req, err := http.NewRequest("GET", "http://10.0.0.100/node", nil)
	if err != nil {
		log.Fatal(err)
	}

	w := httptest.NewRecorder()
	etcdClient := util.NewMockEtcdClient()
	etcdClient.SetValue("/castle/services/ceph/name", "cluster5")
	cephFactory := &testceph.MockConnectionFactory{Fsid: "myfsid", SecretKey: "mykey"}
	h := NewHandler(etcdClient, &test.MockConnectionFactory{}, cephFactory)

	// no nodes discovered, should return empty set
	h.GetNodes(w, req)
	assert.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, `[]`, w.Body.String())

	// set up a discovered node in etcd
	inventory.SetIPAddress(etcdClient, "node1", "10.0.0.11")
	inventory.SetLocation(etcdClient, "node1", "root=default,dc=datacenter1")
	nodeConfigKey := path.Join(inventory.NodesConfigKey, "node1")
	etcdClient.CreateDir(nodeConfigKey)
	inventory.TestSetDiskInfo(etcdClient, nodeConfigKey, "MB2CK3F6S5041EPCPJ4T", "sda", "506d4869-29ee-4bfd-bf21-dfd597bd222e",
		100, true, false, "btrfs", "/mnt/abc", inventory.Disk, "", false)
	inventory.TestSetDiskInfo(etcdClient, nodeConfigKey, "2B9C7KZN3VBM77PSA63P", "sdb", "506d4869-29ee-4bfd-bf21-dfd597bd222e",
		50, false, false, "ext4", "/mnt/def", inventory.Disk, "", false)
	appliedOSDKey := "/castle/services/ceph/osd/applied/node1/devices"
	etcdClient.SetValue(path.Join(appliedOSDKey, "sda", "serial"), "MB2CK3F6S5041EPCPJ4T")
	etcdClient.SetValue(path.Join(appliedOSDKey, "sdb", "serial"), "2B9C7KZN3VBM77PSA63P")

	// since a node exists (with storage), it should be returned now
	w = httptest.NewRecorder()
	h.GetNodes(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, "[{\"nodeId\":\"node1\",\"clusterName\":\"cluster5\",\"ipAddr\":\"10.0.0.11\",\"storage\":150,\"lastUpdated\":31536000000000000,\"state\":1,\"location\":\"root=default,dc=datacenter1\"}]",
		w.Body.String())
}

func TestGetNodesHandlerFailure(t *testing.T) {
	req, err := http.NewRequest("GET", "http://10.0.0.100/node", nil)
	if err != nil {
		log.Fatal(err)
	}

	w := httptest.NewRecorder()
	etcdClient := util.NewMockEtcdClient()

	// failure during node lookup, should return an error status code
	etcdClient.MockGet = func(context ctx.Context, key string, opts *etcd.GetOptions) (*etcd.Response, error) {
		return nil, fmt.Errorf("mock etcd GET error")
	}
	cephFactory := &testceph.MockConnectionFactory{Fsid: "myfsid", SecretKey: "mykey"}
	h := NewHandler(etcdClient, &test.MockConnectionFactory{}, cephFactory)
	h.GetNodes(w, req)
	assert.Equal(t, http.StatusInternalServerError, w.Code)
	assert.Equal(t, ``, w.Body.String())
}

func TestGetMonsHandler(t *testing.T) {
	etcdClient := util.NewMockEtcdClient()
	cephFactory := &testceph.MockConnectionFactory{Fsid: "myfsid", SecretKey: "mykey"}

	req, err := http.NewRequest("GET", "http://10.0.0.100/mon", nil)
	assert.Nil(t, err)

	// first return no mons
	w := httptest.NewRecorder()

	// no mons will be returned, should be empty output
	h := NewHandler(etcdClient, &test.MockConnectionFactory{}, cephFactory)
	h.GetMonitors(w, req)
	assert.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, `[]`, w.Body.String())

	// now return some monitors from etcd
	key := "/castle/services/ceph/monitor/desired/a"
	etcdClient.SetValue(path.Join(key, "id"), "mon0")
	etcdClient.SetValue(path.Join(key, "ipaddress"), "1.2.3.4")
	etcdClient.SetValue(path.Join(key, "port"), "8765")

	// monitors should be returned now, verify the output
	w = httptest.NewRecorder()
	h = NewHandler(etcdClient, &test.MockConnectionFactory{}, cephFactory)
	h.GetMonitors(w, req)
	assert.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, "{\"status\":{\"quorum\":[0],\"monmap\":{\"mons\":[{\"name\":\"mon0\",\"rank\":0,\"addr\":\"10.37.129.87:6790\"}]}},\"desired\":[{\"name\":\"mon0\",\"endpoint\":\"1.2.3.4:8765\"}]}", w.Body.String())
}

func TestGetPoolsHandler(t *testing.T) {
	req, err := http.NewRequest("GET", "http://10.0.0.100/pool", nil)
	if err != nil {
		log.Fatal(err)
	}

	// first return no storage pools
	w := httptest.NewRecorder()
	etcdClient := util.NewMockEtcdClient()
	cephFactory := &testceph.MockConnectionFactory{Fsid: "myfsid", SecretKey: "mykey"}
	connFactory := &test.MockConnectionFactory{}
	cephFactory.Conn = &testceph.MockConnection{
		MockMonCommand: func(args []byte) (buffer []byte, info string, err error) {
			return []byte(`[]`), "", nil
		},
	}
	connFactory.MockConnectAsAdmin = func(cephFactory ceph.ConnectionFactory, etcdClient etcd.KeysAPI) (ceph.Connection, error) {
		return cephFactory.NewConnWithClusterAndUser("mycluster", "admin")
	}

	// no storage pools will be returned, should be empty output
	h := NewHandler(etcdClient, connFactory, cephFactory)
	h.GetPools(w, req)
	assert.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, `[]`, w.Body.String())

	// now return some storage pools from the ceph connection
	w = httptest.NewRecorder()
	cephFactory.Conn = &testceph.MockConnection{
		MockMonCommand: func(args []byte) (buffer []byte, info string, err error) {
			cephPools := []ceph.CephStoragePool{
				{Number: 0, Name: "pool0"},
				{Number: 1, Name: "pool1"},
			}
			resp, err := json.Marshal(cephPools)
			assert.Nil(t, err)
			return resp, "", nil

		},
	}

	// storage pools should be returned now, verify the output
	h = NewHandler(etcdClient, connFactory, cephFactory)
	h.GetPools(w, req)
	assert.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, "[{\"poolName\":\"pool0\",\"poolNum\":0},{\"poolName\":\"pool1\",\"poolNum\":1}]", w.Body.String())
}

func TestGetPoolsHandlerFailure(t *testing.T) {
	req, err := http.NewRequest("GET", "http://10.0.0.100/pool", nil)
	if err != nil {
		log.Fatal(err)
	}

	// encounter an error during GetPools
	w := httptest.NewRecorder()
	etcdClient := util.NewMockEtcdClient()
	cephFactory := &testceph.MockConnectionFactory{Fsid: "myfsid", SecretKey: "mykey"}
	connFactory := &test.MockConnectionFactory{}
	connFactory.MockConnectAsAdmin = func(cephFactory ceph.ConnectionFactory, etcdClient etcd.KeysAPI) (ceph.Connection, error) {
		return nil, fmt.Errorf("mock error for connect as admin")
	}

	h := NewHandler(etcdClient, connFactory, cephFactory)
	h.GetPools(w, req)
	assert.Equal(t, http.StatusInternalServerError, w.Code)
	assert.Equal(t, "", w.Body.String())
}

func TestCreatePoolHandler(t *testing.T) {
	req, err := http.NewRequest("POST", "http://10.0.0.100/pool", strings.NewReader(`{"poolname":"pool1"}`))
	if err != nil {
		log.Fatal(err)
	}

	w := httptest.NewRecorder()
	etcdClient := util.NewMockEtcdClient()
	cephFactory := &testceph.MockConnectionFactory{Fsid: "myfsid", SecretKey: "mykey"}
	connFactory := &test.MockConnectionFactory{}
	cephFactory.Conn = &testceph.MockConnection{
		MockMonCommand: func(args []byte) (buffer []byte, info string, err error) {
			return []byte(""), "successfully created pool1", nil
		},
	}
	connFactory.MockConnectAsAdmin = func(cephFactory ceph.ConnectionFactory, etcdClient etcd.KeysAPI) (ceph.Connection, error) {
		return cephFactory.NewConnWithClusterAndUser("mycluster", "admin")
	}

	h := NewHandler(etcdClient, connFactory, cephFactory)

	h.CreatePool(w, req)
	assert.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, "successfully created pool1", w.Body.String())
}

func TestCreatePoolHandlerFailure(t *testing.T) {
	req, err := http.NewRequest("POST", "http://10.0.0.100/pool", strings.NewReader(`{"poolname":"pool1"}`))
	if err != nil {
		log.Fatal(err)
	}

	w := httptest.NewRecorder()
	etcdClient := util.NewMockEtcdClient()
	cephFactory := &testceph.MockConnectionFactory{Fsid: "myfsid", SecretKey: "mykey"}
	connFactory := &test.MockConnectionFactory{}
	cephFactory.Conn = &testceph.MockConnection{
		MockMonCommand: func(args []byte) (buffer []byte, info string, err error) {
			errMsg := "mock failure to create pool1"
			return []byte(""), errMsg, fmt.Errorf(errMsg)
		},
	}
	connFactory.MockConnectAsAdmin = func(cephFactory ceph.ConnectionFactory, etcdClient etcd.KeysAPI) (ceph.Connection, error) {
		return cephFactory.NewConnWithClusterAndUser("mycluster", "admin")
	}

	h := NewHandler(etcdClient, connFactory, cephFactory)

	h.CreatePool(w, req)
	assert.Equal(t, http.StatusInternalServerError, w.Code)
	assert.Equal(t, "", w.Body.String())
}

func TestGetImagesHandler(t *testing.T) {
	req, err := http.NewRequest("GET", "http://10.0.0.100/image", nil)
	if err != nil {
		log.Fatal(err)
	}

	// first return no storage pools, which means no images will be returned either
	w := httptest.NewRecorder()
	etcdClient := util.NewMockEtcdClient()
	cephFactory := &testceph.MockConnectionFactory{Fsid: "myfsid", SecretKey: "mykey"}
	cephFactory.Conn = &testceph.MockConnection{
		MockMonCommand: func(args []byte) (buffer []byte, info string, err error) {
			return []byte(`[]`), "", nil
		},
	}
	connFactory := &test.MockConnectionFactory{}
	connFactory.MockConnectAsAdmin = func(cephFactory ceph.ConnectionFactory, etcdClient etcd.KeysAPI) (ceph.Connection, error) {
		return cephFactory.NewConnWithClusterAndUser("mycluster", "admin")
	}

	// no images will be returned, should be empty output
	h := NewHandler(etcdClient, connFactory, cephFactory)
	h.GetImages(w, req)
	assert.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, `[]`, w.Body.String())

	// now return some storage pools and images from the ceph connection
	w = httptest.NewRecorder()
	cephFactory.Conn = &testceph.MockConnection{
		MockMonCommand: func(args []byte) (buffer []byte, info string, err error) {
			cephPools := []ceph.CephStoragePool{
				{Number: 0, Name: "pool0"},
				{Number: 1, Name: "pool1"},
			}
			resp, err := json.Marshal(cephPools)
			assert.Nil(t, err)
			return resp, "", nil
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
	h = NewHandler(etcdClient, connFactory, cephFactory)
	h.GetImages(w, req)
	assert.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, "[{\"imageName\":\"image1 - pool0\",\"poolName\":\"pool0\",\"size\":100,\"device\":\"\",\"mountPoint\":\"\"},{\"imageName\":\"image1 - pool1\",\"poolName\":\"pool1\",\"size\":100,\"device\":\"\",\"mountPoint\":\"\"}]", w.Body.String())
}

func TestGetImagesHandlerFailure(t *testing.T) {
	req, err := http.NewRequest("GET", "http://10.0.0.100/image", nil)
	if err != nil {
		log.Fatal(err)
	}

	etcdClient := util.NewMockEtcdClient()
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
	connFactory.MockConnectAsAdmin = func(cephFactory ceph.ConnectionFactory, etcdClient etcd.KeysAPI) (ceph.Connection, error) {
		return cephFactory.NewConnWithClusterAndUser("mycluster", "admin")
	}

	// GetImages should fail due to the mocked error for listing pools
	w := httptest.NewRecorder()
	h := NewHandler(etcdClient, connFactory, cephFactory)
	h.GetImages(w, req)
	assert.Equal(t, http.StatusInternalServerError, w.Code)
	assert.Equal(t, ``, w.Body.String())
}

func TestCreateImageHandler(t *testing.T) {
	req, err := http.NewRequest("POST", "http://10.0.0.100/image", nil)
	if err != nil {
		log.Fatal(err)
	}

	w := httptest.NewRecorder()
	etcdClient := util.NewMockEtcdClient()
	cephFactory := &testceph.MockConnectionFactory{Fsid: "myfsid", SecretKey: "mykey"}
	connFactory := &test.MockConnectionFactory{}
	connFactory.MockConnectAsAdmin = func(cephFactory ceph.ConnectionFactory, etcdClient etcd.KeysAPI) (ceph.Connection, error) {
		return cephFactory.NewConnWithClusterAndUser("mycluster", "admin")
	}

	// image is missing from request body, should be bad request
	h := NewHandler(etcdClient, connFactory, cephFactory)
	h.CreateImage(w, req)
	assert.Equal(t, http.StatusBadRequest, w.Code)
	assert.Equal(t, ``, w.Body.String())

	// request body exists but it's bad json, should be bad request
	req, err = http.NewRequest("POST", "http://10.0.0.100/image", strings.NewReader(`bad json`))
	if err != nil {
		log.Fatal(err)
	}
	w = httptest.NewRecorder()
	h = NewHandler(etcdClient, connFactory, cephFactory)
	h.CreateImage(w, req)
	assert.Equal(t, http.StatusBadRequest, w.Code)
	assert.Equal(t, ``, w.Body.String())

	// missing fields for the image passed via request body, should be bad request
	req, err = http.NewRequest("POST", "http://10.0.0.100/image", strings.NewReader(`{"imageName":"myImage1"}`))
	if err != nil {
		log.Fatal(err)
	}
	w = httptest.NewRecorder()
	h = NewHandler(etcdClient, connFactory, cephFactory)
	h.CreateImage(w, req)
	assert.Equal(t, http.StatusBadRequest, w.Code)
	assert.Equal(t, ``, w.Body.String())

	// well formed successful request to create an image
	req, err = http.NewRequest("POST", "http://10.0.0.100/image", strings.NewReader(`{"imageName":"myImage1","poolName":"myPool1","size":1024}`))
	if err != nil {
		log.Fatal(err)
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
	h = NewHandler(etcdClient, connFactory, cephFactory)
	h.CreateImage(w, req)
	assert.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, `succeeded created image myImage1`, w.Body.String())
}

func TestCreateImageHandlerFailure(t *testing.T) {
	req, err := http.NewRequest("POST", "http://10.0.0.100/image", strings.NewReader(`{"imageName":"myImage1","poolName":"myPool1","size":1024}`))
	if err != nil {
		log.Fatal(err)
	}

	w := httptest.NewRecorder()
	etcdClient := util.NewMockEtcdClient()
	cephFactory := &testceph.MockConnectionFactory{Fsid: "myfsid", SecretKey: "mykey"}
	connFactory := &test.MockConnectionFactory{}
	connFactory.MockConnectAsAdmin = func(cephFactory ceph.ConnectionFactory, etcdClient etcd.KeysAPI) (ceph.Connection, error) {
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
	h := NewHandler(etcdClient, connFactory, cephFactory)
	h.CreateImage(w, req)
	assert.Equal(t, http.StatusInternalServerError, w.Code)
	assert.Equal(t, ``, w.Body.String())
}

func TestGetImageMapInfoHandler(t *testing.T) {
	req, err := http.NewRequest("POST", "http://10.0.0.100/image/mapinfo", nil)
	if err != nil {
		log.Fatal(err)
	}

	w := httptest.NewRecorder()
	etcdClient := util.NewMockEtcdClient()
	cephFactory := &testceph.MockConnectionFactory{Fsid: "myfsid", SecretKey: "mykey"}
	connFactory := &test.MockConnectionFactory{}
	connFactory.MockConnectAsAdmin = func(cephFactory ceph.ConnectionFactory, etcdClient etcd.KeysAPI) (ceph.Connection, error) {
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
	h := NewHandler(etcdClient, connFactory, cephFactory)
	h.GetImageMapInfo(w, req)
	assert.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, "{\"monAddresses\":[\"10.37.129.87:6790\"],\"userName\":\"admin\",\"secretKey\":\"AQBsCv1X5oD9GhAARHVU9N+kFRWDjyLA1dqzIg==\"}", w.Body.String())
}

func TestGetImageMapInfoHandlerFailure(t *testing.T) {
	req, err := http.NewRequest("POST", "http://10.0.0.100/image/mapinfo", nil)
	if err != nil {
		log.Fatal(err)
	}

	w := httptest.NewRecorder()
	etcdClient := util.NewMockEtcdClient()
	cephFactory := &testceph.MockConnectionFactory{Fsid: "myfsid", SecretKey: "mykey"}
	connFactory := &test.MockConnectionFactory{}
	connFactory.MockConnectAsAdmin = func(cephFactory ceph.ConnectionFactory, etcdClient etcd.KeysAPI) (ceph.Connection, error) {
		return cephFactory.NewConnWithClusterAndUser("mycluster", "admin")
	}
	cephFactory.Conn = &testceph.MockConnection{}

	// get image map info should fail becuase there's no mock response set up for auth get-key
	h := NewHandler(etcdClient, connFactory, cephFactory)
	h.GetImageMapInfo(w, req)
	assert.Equal(t, http.StatusInternalServerError, w.Code)
	assert.Equal(t, "", w.Body.String())
}
