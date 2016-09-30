package client

// interface for creating connections to ceph
type ConnectionFactory interface {
	NewConnWithClusterAndUser(clusterName string, userName string) (Connection, error)
	NewFsid() (string, error)
	NewSecretKey() (string, error)
}

// interface for connecting to the ceph cluster
type Connection interface {
	Connect() error
	Shutdown()
	OpenIOContext(pool string) (IOContext, error)
	ReadConfigFile(path string) error
	MonCommand(args []byte) (buffer []byte, info string, err error)
	MonCommandWithInputBuffer(args, inputBuffer []byte) (buffer []byte, info string, err error)
	PingMonitor(id string) (string, error)
}
