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

// Package config provides a shared way of referring to data storage locations for Ceph Daemons,
// including both the in-container location and on-host location as well as whether the data is
// persisted to the host.
package config

import "path"

// A DataPathMap is a struct which contains information about where Ceph daemon data is stored in
// containers and whether the data should be persisted to the host. If it is persisted to the host,
// directory on the host where the specific daemon's data is stored is given.
type DataPathMap struct {
	// HostDataDir should be set to the path on the host where the specific daemon's data is stored.
	// If this is empty, the daemon does not persist data to the host, but data may still be shared
	// between containers in a pod via an empty dir.
	HostDataDir string

	// ContainerDataDir should be set to the path in the container where the specific daemon's data
	// is stored. If this is empty, the daemon does not store data at all, even in the container,
	// and data is not shared between container in a pod via empty dir.
	ContainerDataDir string

	// HostLogDir represents Ceph's logging directory on the host. If this is empty, logs are not
	// persisted to the host. The log dir is always /var/log/ceph. If logs are not persisted to the
	// host, logs are not shared between containers via empty dir or any other mechanism.
	HostLogDir string
}

// NewStatefulDaemonDataPathMap returns a new DataPathMap for a daemon which requires a persistent
// config (mons, osds). daemonDataDirHostRelativePath is the path relative to the dataDirHostPath
// where the daemon's data is stored on the host's filesystem. Daemons which use a DataPathMap
// created by this method will only have access to their own data and not the entire dataDirHostPath
// which may include data from other daemons.
func NewStatefulDaemonDataPathMap(
	dataDirHostPath, daemonDataDirHostRelativePath string,
	daemonType DaemonType, daemonID, namespace string,
) *DataPathMap {
	return &DataPathMap{
		HostDataDir:      path.Join(dataDirHostPath, daemonDataDirHostRelativePath),
		ContainerDataDir: cephDataDir(daemonType, daemonID),
		HostLogDir:       path.Join(dataDirHostPath, namespace, "log"),
	}
}

// NewStatelessDaemonDataPathMap returns a new DataPathMap for a daemon which does not persist data
// to the host (mgrs, mdses, rgws)
func NewStatelessDaemonDataPathMap(
	daemonType DaemonType, daemonID, namespace, dataDirHostPath string,
) *DataPathMap {
	return &DataPathMap{
		HostDataDir:      "",
		ContainerDataDir: cephDataDir(daemonType, daemonID),
		HostLogDir:       path.Join(dataDirHostPath, namespace, "log"),
	}
}

// NewDatalessDaemonDataPathMap returns a new DataPathMap for a daemon which does not utilize a data
// dir in the container as the mon, mgr, osd, mds, and rgw daemons do.
func NewDatalessDaemonDataPathMap(namespace, dataDirHostPath string) *DataPathMap {
	return &DataPathMap{
		HostDataDir:      "",
		ContainerDataDir: "",
		HostLogDir:       path.Join(dataDirHostPath, namespace, "log"),
	}
}

func cephDataDir(daemonType DaemonType, daemonID string) string {
	// daemons' default data dirs are: /var/lib/ceph/<daemon-type>/ceph-<daemon-id>
	return path.Join(VarLibCephDir, string(daemonType), "ceph-"+daemonID)
}
