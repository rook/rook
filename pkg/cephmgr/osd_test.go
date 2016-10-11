package cephmgr

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/quantum/castle/pkg/cephmgr/client"
	testceph "github.com/quantum/castle/pkg/cephmgr/client/test"
	"github.com/quantum/castle/pkg/clusterd"
	"github.com/quantum/castle/pkg/util"
	"github.com/stretchr/testify/assert"
)

func TestGetOSDInfo(t *testing.T) {
	// error when no info is found on disk
	id, guid, err := getOSDInfo("/tmp")
	assert.Equal(t, -1, id)
	assert.NotNil(t, err)
	assert.Equal(t, uuid.UUID{}, guid)

	// write the info to disk
	whoFile := "/tmp/whoami"
	ioutil.WriteFile(whoFile, []byte("23"), 0644)
	defer os.Remove(whoFile)
	fsidFile := "/tmp/fsid"
	testUUID, _ := uuid.NewUUID()
	ioutil.WriteFile(fsidFile, []byte(testUUID.String()), 0644)
	defer os.Remove(fsidFile)

	// check the successful osd info
	id, guid, err = getOSDInfo("/tmp")
	assert.Nil(t, err)
	assert.Equal(t, 23, id)
	assert.Equal(t, testUUID, guid)
}

func TestDesiredDeviceState(t *testing.T) {
	etcdClient := util.NewMockEtcdClient()

	// add a device
	device := &Device{Name: "foo", NodeID: "a"}
	err := AddDesiredDevice(etcdClient, device)
	assert.Nil(t, err)
	devices := etcdClient.GetChildDirs("/castle/services/ceph/osd/desired/a/device")
	assert.Equal(t, 1, devices.Count())
	assert.True(t, devices.Contains("foo"))

	// remove the device
	err = RemoveDesiredDevice(etcdClient, device)
	assert.Nil(t, err)
	devices = etcdClient.GetChildDirs("/castle/services/ceph/osd/desired/a/device")
	assert.Equal(t, 0, devices.Count())

	// removing a non-existent device is a no-op
	err = RemoveDesiredDevice(etcdClient, device)
	assert.Nil(t, err)
}

func TestOSDBootstrap(t *testing.T) {
	clusterName := "mycluster"
	targetPath := getBootstrapOSDKeyringPath(clusterName)
	defer os.Remove(targetPath)

	factory := &testceph.MockConnectionFactory{}
	conn, _ := factory.NewConnWithClusterAndUser(clusterName, "user")
	conn.(*testceph.MockConnection).MockMonCommand = func(buf []byte) (buffer []byte, info string, err error) {
		response := "{\"key\":\"mysecurekey\"}"
		log.Printf("Returning: %s", response)
		return []byte(response), "", nil
	}

	err := createOSDBootstrapKeyring(conn, clusterName)
	assert.Nil(t, err)

	contents, err := ioutil.ReadFile(targetPath)
	assert.Nil(t, err)
	assert.NotEqual(t, -1, strings.Index(string(contents), "[client.bootstrap-osd]"))
	assert.NotEqual(t, -1, strings.Index(string(contents), "key = mysecurekey"))
	assert.NotEqual(t, -1, strings.Index(string(contents), "caps mon = \"allow profile bootstrap-osd\""))
}

func TestCrushMap(t *testing.T) {

	etcdClient := util.NewMockEtcdClient()
	context := &clusterd.Context{EtcdClient: etcdClient, NodeID: "node1"}
	factory := &testceph.MockConnectionFactory{Fsid: "fsid", SecretKey: "key"}
	conn, _ := factory.NewConnWithClusterAndUser("cluster", "user")
	conn.(*testceph.MockConnection).MockMonCommand = func(buf []byte) (buffer []byte, info string, err error) {
		var request client.MonStatusRequest
		err = json.Unmarshal(buf, &request)
		assert.Nil(t, err)
		assert.Equal(t, "json", request.Format)
		assert.Equal(t, "osd crush create-or-move", request.Prefix)
		assert.Equal(t, 23, request.ID)
		assert.NotEqual(t, 0.0, request.Weight)
		assert.Equal(t, 3, len(request.Args), fmt.Sprintf("args=%v", request.Args))

		// verify the contents of the CRUSH location args
		argsSet := util.CreateSet(request.Args)
		assert.True(t, argsSet.Contains("root=default"))
		assert.True(t, argsSet.Contains("dc=datacenter1"))
		assert.True(t, argsSet.Contains("hostName=node1"))

		return []byte{}, "", nil
	}

	location := "root=default,dc=datacenter1,hostName=node1"

	err := addOSDToCrushMap(conn, context, 23, "/", location)
	assert.Nil(t, err)

	// location should have been stored in etcd as well
	assert.Equal(t, location, etcdClient.GetValue("/castle/nodes/config/node1/location"))
}

func TestGetCrushMap(t *testing.T) {
	factory := &testceph.MockConnectionFactory{Fsid: "fsid", SecretKey: "key"}
	conn, _ := factory.NewConnWithClusterAndUser("cluster", "user")
	response, err := GetCrushMap(conn)

	assert.Nil(t, err)
	assert.Equal(t, "", response)
}

func TestCrushLocation(t *testing.T) {
	loc := "dc=datacenter1"
	hostName, err := os.Hostname()
	assert.Nil(t, err)

	// test that host name and root will get filled in with default/runtime values
	res, err := formatLocation(loc)
	assert.Nil(t, err)
	assert.Equal(t, 3, len(res))
	locSet := util.CreateSet(res)
	assert.True(t, locSet.Contains("root=default"))
	assert.True(t, locSet.Contains("dc=datacenter1"))
	assert.True(t, locSet.Contains(fmt.Sprintf("hostName=%s", hostName)))

	// test that if host name and root are already set they will be honored
	loc = "root=otherRoot,dc=datacenter2,hostName=node123"
	res, err = formatLocation(loc)
	assert.Nil(t, err)
	assert.Equal(t, 3, len(res))
	locSet = util.CreateSet(res)
	assert.True(t, locSet.Contains("root=otherRoot"))
	assert.True(t, locSet.Contains("dc=datacenter2"))
	assert.True(t, locSet.Contains("hostName=node123"))

	// test an invalid CRUSH location format
	loc = "root=default,prop:value"
	_, err = formatLocation(loc)
	assert.NotNil(t, err)
	assert.Contains(t, err.Error(), "is not in a valid format")
}
