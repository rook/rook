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

import (
	"github.com/rook/rook/pkg/operator/ceph/version"
)

// DefaultFlags returns the default configuration flags Rook will set on the command line for all
// calls to Ceph daemons and tools. Values specified here will not be able to be overridden using
// the mon's central KV store, and that is (and should be) by intent.
func DefaultFlags(fsid, mountedKeyringPath string, cephVersion version.CephVersion) []string {
	flags := []string{
		// fsid unnecessary but is a safety to make sure daemons can only connect to their cluster
		NewFlag("fsid", fsid),
		NewFlag("keyring", mountedKeyringPath),
		// For containers, we're expected to log everything to stderr
		NewFlag("log-to-stderr", "true"),
		NewFlag("err-to-stderr", "true"),
		NewFlag("mon-cluster-log-to-stderr", "true"),
		// differentiate debug text from audit text, and the space after 'debug' is critical
		NewFlag("log-stderr-prefix", "debug "),
	}

	// As of Nautilus 14.2.1 at least
	// These new flags control Ceph's daemon logging behavior to files
	// By default we set them to False so no logs get written on file
	// However they can be activated at any time via the centralized config store
	if cephVersion.IsAtLeast(version.CephVersion{Major: 14, Minor: 2, Extra: 1}) {
		flags = append(flags, []string{
			NewFlag("default-log-to-file", "false"),
			NewFlag("default-mon-cluster-log-to-file", "false"),
		}...)
	}

	flags = append(flags, StoredMonHostEnvVarFlags()...)

	return flags
}

// makes it possible to be slightly less verbose to create a ConfigOverride here
func configOverride(who, option, value string) Option {
	return Option{Who: who, Option: option, Value: value}
}

// DefaultCentralizedConfigs returns the default configuration options Rook will set in Ceph's
// centralized config store.
func DefaultCentralizedConfigs(cephVersion version.CephVersion) []Option {
	overrides := []Option{
		configOverride("global", "mon allow pool delete", "true"),
	}

	// Everything before Nautilus 14.2.1
	// Prior to Nautilus 14.2.1 certain log flags were not present
	// so in order to not log anything on files we must set the following flags to null
	// Since Nautilus 14.2.1 introduced both 'default-log-to-file' and 'default-mon-cluster-log-to-file' (see above defaultFlagConfigs)
	// these are not needed
	if !cephVersion.IsAtLeast(version.CephVersion{Major: 14, Minor: 2, Extra: 1}) {
		// Set the default log files to empty so they don't bloat containers. Can be changed in
		// Mimic+ by users if needed.
		overrides = append(overrides, []Option{
			configOverride("global", "log file", ""),
			configOverride("global", "mon cluster log file", ""),
		}...)
	}

	// We disable "bluestore warn on legacy statfs"
	// This setting appeared on 14.2.2, so if detected we disable the warning
	// As of 14.2.5 (https://github.com/rook/rook/issues/3539#issuecomment-531287051), Ceph will disable this flag by default so there is no need to apply it
	if cephVersion.IsAtLeast(version.CephVersion{Major: 14, Minor: 2, Extra: 2}) && version.IsInferior(cephVersion, version.CephVersion{Major: 14, Minor: 2, Extra: 5}) {
		overrides = append(overrides, []Option{
			configOverride("global", "bluestore warn on legacy statfs", "false"),
		}...)
	}

	return overrides
}

// DefaultLegacyConfigs need to be added to the Ceph config file until the integration tests can be
// made to override these options for the Ceph clusters it creates.
func DefaultLegacyConfigs() []Option {
	overrides := []Option{
		// TODO: drop this when FlexVolume is no longer supported
		configOverride("global", "rbd_default_features", "3"),
	}
	return overrides
}
