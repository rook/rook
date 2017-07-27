package test

import (
	"fmt"
	"io/ioutil"
	"os"
	"path"

	"github.com/rook/rook/pkg/util"
)

func CreateClusterInfo(etcdClient *util.MockEtcdClient, configDir string, mons []string) {
	if etcdClient != nil {
		key := "/rook/services/ceph"
		etcdClient.SetValue(path.Join(key, "fsid"), "12345")
		etcdClient.SetValue(path.Join(key, "name"), "rookcluster")
		etcdClient.SetValue(path.Join(key, "_secrets/monitor"), "foo")
		etcdClient.SetValue(path.Join(key, "_secrets/admin"), "bar")

		base := "/rook/services/ceph/monitor/desired"
		for i, mon := range mons {
			etcdClient.SetValue(path.Join(base, mon, "id"), fmt.Sprintf("mon%d", i))
			etcdClient.SetValue(path.Join(base, mon, "ipaddress"), fmt.Sprintf("1.2.3.%d", i))
			etcdClient.SetValue(path.Join(base, mon, "port"), "4321")
		}
	}

	if configDir != "" {
		os.MkdirAll(configDir, 0744)
		ioutil.WriteFile(path.Join(configDir, "client.admin.keyring"), []byte("key = adminsecret"), 0644)
		ioutil.WriteFile(path.Join(configDir, "mon.keyring"), []byte("key = monsecret"), 0644)
	}
}
