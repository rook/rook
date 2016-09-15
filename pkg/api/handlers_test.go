package api

import (
	"fmt"
	"log"
	"net/http"
	"net/http/httptest"
	"path"
	"testing"

	etcd "github.com/coreos/etcd/client"
	ctx "golang.org/x/net/context"

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
	h := NewHandler(etcdClient)

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
	assert.Equal(t, "[{\"nodeID\":\"node1\",\"ipAddr\":\"10.0.0.11\",\"storage\":150}]", w.Body.String())
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
	h := NewHandler(etcdClient)
	h.GetNodes(w, req)
	assert.Equal(t, http.StatusInternalServerError, w.Code)
	assert.Equal(t, ``, w.Body.String())
}
