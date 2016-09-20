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

	"github.com/quantum/castle/pkg/castled/test"
	"github.com/quantum/castle/pkg/cephclient"
	testceph "github.com/quantum/castle/pkg/cephclient/test"
	"github.com/quantum/castle/pkg/clusterd/inventory"
	"github.com/quantum/castle/pkg/util"
	"github.com/stretchr/testify/assert"
)

func TestGetNodesHandler(t *testing.T) {
	req, err := http.NewRequest("GET", "http://10.0.0.100/node", nil)
	if err != nil {
		log.Fatal(err)
	}

	w := httptest.NewRecorder()
	etcdClient := util.NewMockEtcdClient()
	cephFactory := &testceph.MockConnectionFactory{Fsid: "myfsid", SecretKey: "mykey"}
	h := NewHandler(etcdClient, &test.MockConnectionFactory{}, cephFactory)

	// no nodes discovered, should return empty set
	h.GetNodes(w, req)
	assert.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, `[]`, w.Body.String())

	// set up a discovered node in etcd
	inventory.SetIpAddress(etcdClient, "node1", "10.0.0.11")
	nodeConfigKey := path.Join(inventory.DiscoveredNodesKey, "node1")
	etcdClient.CreateDir(nodeConfigKey)
	inventory.TestSetDiskInfo(etcdClient, nodeConfigKey, "MB2CK3F6S5041EPCPJ4T", "sda", "506d4869-29ee-4bfd-bf21-dfd597bd222e",
		100, true, false, "btrfs", "/mnt/abc", inventory.Disk, "", false)
	inventory.TestSetDiskInfo(etcdClient, nodeConfigKey, "2B9C7KZN3VBM77PSA63P", "sdb", "506d4869-29ee-4bfd-bf21-dfd597bd222e",
		50, false, false, "ext4", "/mnt/def", inventory.Disk, "", false)
	appliedOSDKey := "/castle/services/ceph/osd/applied/node1"
	etcdClient.CreateDir(path.Join(appliedOSDKey, "MB2CK3F6S5041EPCPJ4T"))
	etcdClient.CreateDir(path.Join(appliedOSDKey, "2B9C7KZN3VBM77PSA63P"))

	// since a node exists (with storage), it should be returned now
	w = httptest.NewRecorder()
	h.GetNodes(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, "[{\"nodeId\":\"node1\",\"ipAddr\":\"10.0.0.11\",\"storage\":150}]", w.Body.String())
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
	connFactory.MockConnectAsAdmin = func(cephFactory cephclient.ConnectionFactory, etcdClient etcd.KeysAPI) (cephclient.Connection, error) {
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
			cephPools := []cephclient.CephStoragePool{
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
	connFactory.MockConnectAsAdmin = func(cephFactory cephclient.ConnectionFactory, etcdClient etcd.KeysAPI) (cephclient.Connection, error) {
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
	connFactory.MockConnectAsAdmin = func(cephFactory cephclient.ConnectionFactory, etcdClient etcd.KeysAPI) (cephclient.Connection, error) {
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
	connFactory.MockConnectAsAdmin = func(cephFactory cephclient.ConnectionFactory, etcdClient etcd.KeysAPI) (cephclient.Connection, error) {
		return cephFactory.NewConnWithClusterAndUser("mycluster", "admin")
	}

	h := NewHandler(etcdClient, connFactory, cephFactory)

	h.CreatePool(w, req)
	assert.Equal(t, http.StatusInternalServerError, w.Code)
	assert.Equal(t, "", w.Body.String())
}
