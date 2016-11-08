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
