package testceph

import (
	"strings"

	"github.com/quantum/castle/pkg/cephclient"
)

/////////////////////////////////////////////////////////////
// implement the interface for generating ceph connections
/////////////////////////////////////////////////////////////
type MockConnectionFactory struct {
	Conn      *MockConnection
	Fsid      string
	SecretKey string
}

func (m *MockConnectionFactory) NewConnWithClusterAndUser(clusterName string, userName string) (cephclient.Connection, error) {
	if m.Conn == nil {
		m.Conn = &MockConnection{}
	}

	return m.Conn, nil
}
func (m *MockConnectionFactory) NewFsid() (string, error) {
	return m.Fsid, nil
}
func (m *MockConnectionFactory) NewSecretKey() (string, error) {
	return m.SecretKey, nil
}

/////////////////////////////////////////////////////////////
// implement the interface for connecting to the ceph cluster
/////////////////////////////////////////////////////////////
type MockConnection struct {
	IOContext      *MockIOContext
	MockMonCommand func(args []byte) (buffer []byte, info string, err error)
}

func (m *MockConnection) Connect() error {
	return nil
}
func (m *MockConnection) Shutdown() {
}
func (m *MockConnection) OpenIOContext(pool string) (cephclient.IOContext, error) {
	if m.IOContext == nil {
		m.IOContext = &MockIOContext{}
	}

	return m.IOContext, nil
}
func (m *MockConnection) ReadConfigFile(path string) error {
	return nil
}
func (m *MockConnection) MonCommand(args []byte) (buffer []byte, info string, err error) {
	if m.MockMonCommand != nil {
		return m.MockMonCommand(args)
	}

	// return a response for monitor status
	switch {
	case strings.Index(string(args), "mon_status") != -1:
		response := "{\"name\":\"mon0\",\"rank\":0,\"state\":\"leader\",\"election_epoch\":3,\"quorum\":[0],\"monmap\":{\"epoch\":1," +
			"\"fsid\":\"22ae0d50-c4bc-4cfb-9cf4-341acbe35302\",\"modified\":\"2016-09-16 04:21:51.635837\",\"created\":\"2016-09-16 04:21:51.635837\"," +
			"\"mons\":[{\"rank\":0,\"name\":\"mon0\",\"addr\":\"10.37.129.87:6790\"}]}}"
		return []byte(response), "info", nil
	}

	// unhandled response
	return []byte{}, "info", nil
}
func (m *MockConnection) MonCommandWithInputBuffer(args, inputBuffer []byte) (buffer []byte, info string, err error) {
	return []byte{}, "info", nil
}
func (m *MockConnection) PingMonitor(id string) (string, error) {
	return "pinginfo", nil
}

/////////////////////////////////////////////////////////////
// implement the interface for the ceph io context
/////////////////////////////////////////////////////////////
type MockIOContext struct {
}

func (m *MockIOContext) Read(oid string, data []byte, offset uint64) (int, error) {
	return 0, nil
}
func (m *MockIOContext) Write(oid string, data []byte, offset uint64) error {
	return nil
}
func (m *MockIOContext) WriteFull(oid string, data []byte) error {
	return nil
}
