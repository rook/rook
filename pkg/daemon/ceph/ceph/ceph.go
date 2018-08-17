/*
Daemon package ceph provides shared information that is applicable to all daemons in a Ceph cluster.
*/

package ceph

import (
	"path"
)

const (
	// DefaultConfigDir is the default dir where Ceph stores its configs
	DefaultConfigDir = "/etc/ceph"
	// DefaultConfigFile is the default name of the file where Ceph stores its configs
	DefaultConfigFile = "ceph.conf"
	// DefaultKeyringFile is the default name of the file where Ceph stores its keyring info
	DefaultKeyringFile = "keyring"
)

// DefaultConfigFilePath returns the full path to Ceph's default config file
func DefaultConfigFilePath() string {
	return path.Join(DefaultConfigDir, DefaultConfigFile)
}

// DefaultKeyringFilePath returns the full path to Ceph's default keyring file
func DefaultKeyringFilePath() string {
	return path.Join(DefaultConfigDir, DefaultKeyringFile)
}
