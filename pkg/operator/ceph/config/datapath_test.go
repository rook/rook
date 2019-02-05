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
	d := NewStatefulDaemonDataPathMap("/var/lib/rook", "/mon-a/data", MonType, "a")
	assert.Equal(t, &DataPathMap{
		PersistData:      true,
		HostDataDir:      "/var/lib/rook/mon-a/data",
		ContainerDataDir: "/var/lib/ceph/mon/ceph-a",
	}, d)

	// osd
	d = NewStatefulDaemonDataPathMap("/var/lib/rook/", "osd0/", OsdType, "0")
	assert.Equal(t, &DataPathMap{
		PersistData:      true,
		HostDataDir:      "/var/lib/rook/osd0",
		ContainerDataDir: "/var/lib/ceph/osd/ceph-0",
	}, d)
}

func TestNewStatelessDaemonDataPathMap(t *testing.T) {
	// mgr
	d := NewStatelessDaemonDataPathMap(MgrType, "a")
	assert.Equal(t, &DataPathMap{
		PersistData:      false,
		HostDataDir:      "",
		ContainerDataDir: "/var/lib/ceph/mgr/ceph-a",
	}, d)

	// mds
	d = NewStatelessDaemonDataPathMap(MdsType, "myfs.a")
	assert.Equal(t, &DataPathMap{
		PersistData:      false,
		HostDataDir:      "",
		ContainerDataDir: "/var/lib/ceph/mds/ceph-myfs.a",
	}, d)

	// rgw
	d = NewStatelessDaemonDataPathMap(RgwType, "objstore")
	assert.Equal(t, &DataPathMap{
		PersistData:      false,
		HostDataDir:      "",
		ContainerDataDir: "/var/lib/ceph/rgw/ceph-objstore",
	}, d)
}
