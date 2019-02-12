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

// Package config provides default configurations which Rook will set in Ceph clusters.
package config

// DefaultFlags returns the default configuration flags Rook will set on the command line for all
// calls to Ceph daemons and tools. Values specified here will not be able to be overridden using
// the mon's central KV store, and that is (and should be) by intent.
func DefaultFlags(fsid, mountedKeyringPath string) []string {
	c := NewConfig()
	c.Section("global").
		// fsid unnecessary but is a safety to make sure daemons can only connect to their cluster
		Set("fsid", fsid).
		Set("keyring", mountedKeyringPath).
		// For containers, we're expected to log everything to stderr
		Set("log-to-stderr", "true").
		Set("err-to-stderr", "true").
		Set("mon-cluster-log-to-stderr", "true").
		Set("log-stderr-prefix", "debug ")
		// ^ differentiate debug text from audit text, and the space after 'debug' is critical
	return append(
		c.GlobalFlags(),
		StoredMonHostEnvVarFlags()...,
	)
}

// DefaultCentralizedConfigs returns the default configuration options Rook will set in Ceph's
// centralized config store. If the version of Ceph does not support the centralized config store,
// these will be set in a shared config file instead.
func DefaultCentralizedConfigs() *Config {
	c := NewConfig()
	c.Section("global").
		// Set the default log files to empty so they don't bloat containers. Can be changed in
		// Mimic+ by users if needed.
		Set("log file", "").
		Set("mon cluster log file", "").
		//
		Set("mon allow pool delete", "true")
	return c
}

// DefaultLegacyConfigs need to be added to the Ceph config file until the integration tests can be
// made to override these options for the Ceph clusters it creates.
func DefaultLegacyConfigs() *Config {
	c := NewConfig()
	c.Section("global").
		Set("mon max pg per osd", "1000").
		//
		Set("osd pg bits", "11").
		Set("osd pgp bits", "11").
		Set("osd pool default size", "1").
		Set("osd pool default min size", "1").
		Set("osd pool default pg num", "100").
		Set("osd pool default pgp num", "100").
		//
		Set("rbd_default_features", "3"). // TODO: still needed?
		//
		Set("fatal signal handlers", "false") // TODO: still needed?
	return c
}
