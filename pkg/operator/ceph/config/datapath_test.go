/*
Copyright 2019 The Rook Authors. All rights reserved.

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

package config

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestNewStatefulDaemonDataPathMap(t *testing.T) {
	// mon
	d := NewStatefulDaemonDataPathMap("/var/lib/rook", "/mon-a/data", MonType, "a", "rook-ceph")
	assert.Equal(t, &DataPathMap{
		PersistData:      true,
		HostDataDir:      "/var/lib/rook/mon-a/data",
		ContainerDataDir: "/var/lib/ceph/mon/ceph-a",
		ContainerLogDir:  "/var/log/ceph",
		HostLogDir:       "/var/lib/rook/rook-ceph/log",
	}, d)

	// osd
	d = NewStatefulDaemonDataPathMap("/var/lib/rook/", "osd0/", OsdType, "0", "rook-ceph")
	assert.Equal(t, &DataPathMap{
		PersistData:      true,
		HostDataDir:      "/var/lib/rook/osd0",
		ContainerDataDir: "/var/lib/ceph/osd/ceph-0",
		ContainerLogDir:  "/var/log/ceph",
		HostLogDir:       "/var/lib/rook/rook-ceph/log",
	}, d)
}

func TestNewStatelessDaemonDataPathMap(t *testing.T) {
	// mgr
	d := NewStatelessDaemonDataPathMap(MgrType, "a", "rook-ceph", "/var/lib/rook")
	assert.Equal(t, &DataPathMap{
		PersistData:      false,
		HostDataDir:      "",
		ContainerDataDir: "/var/lib/ceph/mgr/ceph-a",
		ContainerLogDir:  "/var/log/ceph",
		HostLogDir:       "/var/lib/rook/rook-ceph/log",
	}, d)

	// mds
	d = NewStatelessDaemonDataPathMap(MdsType, "myfs.a", "rook-ceph", "/var/lib/rook")
	assert.Equal(t, &DataPathMap{
		PersistData:      false,
		HostDataDir:      "",
		ContainerDataDir: "/var/lib/ceph/mds/ceph-myfs.a",
		ContainerLogDir:  "/var/log/ceph",
		HostLogDir:       "/var/lib/rook/rook-ceph/log",
	}, d)

	// rgw
	d = NewStatelessDaemonDataPathMap(RgwType, "objstore", "rook-ceph", "/var/lib/rook")
	assert.Equal(t, &DataPathMap{
		PersistData:      false,
		HostDataDir:      "",
		ContainerDataDir: "/var/lib/ceph/rgw/ceph-objstore",
		ContainerLogDir:  "/var/log/ceph",
		HostLogDir:       "/var/lib/rook/rook-ceph/log",
	}, d)
}
