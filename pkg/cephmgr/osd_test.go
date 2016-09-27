package cephmgr

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"strings"
	"testing"

	testceph "github.com/quantum/castle/pkg/cephmgr/client/test"
	"github.com/stretchr/testify/assert"
)

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

	factory := &testceph.MockConnectionFactory{Fsid: "fsid", SecretKey: "key"}
	conn, _ := factory.NewConnWithClusterAndUser("cluster", "user")
	conn.(*testceph.MockConnection).MockMonCommand = func(buf []byte) (buffer []byte, info string, err error) {
		var request MonStatusRequest
		err = json.Unmarshal(buf, &request)
		assert.Nil(t, err)
		assert.Equal(t, "json", request.Format)
		assert.Equal(t, "osd crush create-or-move", request.Prefix)
		assert.Equal(t, 23, request.ID)
		assert.NotEqual(t, 0.0, request.Weight)
		assert.Equal(t, 3, len(request.Args), fmt.Sprintf("args=%v", request.Args))
		return []byte{}, "", nil
	}
	location := &CrushLocation{
		Root:       "myroot",
		Datacenter: "dat1",
	}
	err := addOSDToCrushMap(conn, 23, "/", location)
	assert.Nil(t, err)
}

func TestCrushLocation(t *testing.T) {
	loc := &CrushLocation{}

	// check that host is required
	res, err := formatLocation(loc)
	assert.NotNil(t, err)
	loc.Host = "h"

	// check the default root
	res, err = formatLocation(loc)
	assert.Nil(t, err)
	assert.Equal(t, 2, len(res))
	assert.Equal(t, "root=default", res[0])
	assert.Equal(t, "hostName=h", res[1])

	// test all the attributes
	loc.Root = "r"
	loc.Chassis = "c"
	loc.Datacenter = "d"
	loc.PDU = "p1"
	loc.Pod = "p2"
	loc.Rack = "rk"
	loc.Room = "rm"
	loc.Row = "rw"
	res, err = formatLocation(loc)
	assert.Nil(t, err)
	assert.Equal(t, 9, len(res))
	assert.Equal(t, "root=r", res[0])
	assert.Equal(t, "datacenter=d", res[1])
	assert.Equal(t, "room=rm", res[2])
	assert.Equal(t, "row=rw", res[3])
	assert.Equal(t, "pod=p2", res[4])
	assert.Equal(t, "pdu=p1", res[5])
	assert.Equal(t, "rack=rk", res[6])
	assert.Equal(t, "chassis=c", res[7])
	assert.Equal(t, "hostName=h", res[8])
}
