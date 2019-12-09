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

// Package config provides methods for generating the Ceph config for a Ceph cluster and for
// producing a "ceph.conf" compatible file from the config as well as Ceph command line-compatible
// flags.
package config

import (
	"fmt"
	"path"
	"strings"

	"github.com/coreos/pkg/capnslog"
	"github.com/pkg/errors"
	"github.com/rook/rook/pkg/clusterd"
	cephconfig "github.com/rook/rook/pkg/daemon/ceph/config"
)

var logger = capnslog.NewPackageLogger("github.com/rook/rook", "op-config")

// DaemonType defines the type of a daemon. e.g., mon, mgr, osd, mds, rgw
type DaemonType string

const (
	// MonType defines the mon DaemonType
	MonType DaemonType = "mon"

	// MgrType defines the mgr DaemonType
	MgrType DaemonType = "mgr"

	// OsdType defines the osd DaemonType
	OsdType DaemonType = "osd"

	// MdsType defines the mds DaemonType
	MdsType DaemonType = "mds"

	// RgwType defines the rgw DaemonType
	RgwType DaemonType = "rgw"

	// RbdMirrorType defines the rbd-mirror DaemonType
	RbdMirrorType DaemonType = "rbd-mirror"

	// CrashType defines the crash collector DaemonType
	CrashType DaemonType = "crashcollector"

	// CephUser is the Linux Ceph username
	CephUser = "ceph"

	// CephGroup is the Linux Ceph groupname
	CephGroup = "ceph"
)

var (
	// VarLibCephDir is simply "/var/lib/ceph". It is made overwriteable only for unit tests where it
	// may be needed to send data intended for /var/lib/ceph to a temporary test dir.
	VarLibCephDir = "/var/lib/ceph"

	// EtcCephDir is simply "/etc/ceph". It is made overwriteable only for unit tests where it
	// may be needed to send data intended for /etc/ceph to a temporary test dir.
	EtcCephDir = "/etc/ceph"

	// VarLogCephDir defines Ceph logging directory. It is made overwriteable only for unit tests where it
	// may be needed to send data intended for /var/log/ceph to a temporary test dir.
	VarLogCephDir = "/var/log/ceph"

	// VarLibCephCrashDir defines Ceph crash reports directory.
	VarLibCephCrashDir = path.Join(VarLibCephDir, "crash")
)

// normalizeKey converts a key in any format to a key with underscores.
//
// The internal representation of Ceph config keys uses underscores only, where Ceph supports both
// spaces, underscores, and hyphens. This is so that Rook can properly match and override keys even
// when they are specified as "some config key" in one section, "some_config_key" in another
// section, and "some-config-key" in yet another section.
func normalizeKey(key string) string {
	return strings.Replace(strings.Replace(key, " ", "_", -1), "-", "_", -1)
}

// NewFlag returns the key-value pair in the format of a Ceph command line-compatible flag.
func NewFlag(key, value string) string {
	// A flag is a normalized key with underscores replaced by dashes.
	// "debug default" ~normalize~> "debug_default" ~to~flag~> "debug-default"
	n := normalizeKey(key)
	f := strings.Replace(n, "_", "-", -1)
	return fmt.Sprintf("--%s=%s", f, value)
}

// SetDefaultConfigs sets Rook's desired default configs in the centralized monitor database. This
// cannot be called before at least one monitor is established.
func SetDefaultConfigs(
	context *clusterd.Context,
	namespace string,
	clusterInfo *cephconfig.ClusterInfo,
) error {
	// ceph.conf is never used. All configurations are made in the centralized mon config database,
	// or they are specified on the commandline when daemons are called.
	monStore := GetMonStore(context, namespace)

	if err := monStore.SetAll(DefaultCentralizedConfigs(clusterInfo.CephVersion)...); err != nil {
		return errors.Wrapf(err, "failed to apply default Ceph configurations")
	}

	if err := monStore.SetAll(DefaultLegacyConfigs()...); err != nil {
		return errors.Wrapf(err, "failed to apply legacy config overrides")
	}

	return nil
}
