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
	cephv1 "github.com/rook/rook/pkg/apis/ceph.rook.io/v1"
	"github.com/rook/rook/pkg/clusterd"
	cephclient "github.com/rook/rook/pkg/daemon/ceph/client"
	"github.com/rook/rook/pkg/operator/ceph/version"
)

var logger = capnslog.NewPackageLogger("github.com/rook/rook", "op-config")

const (
	// MonType defines the mon DaemonType
	MonType = "mon"

	// MgrType defines the mgr DaemonType
	MgrType = "mgr"

	// OsdType defines the osd DaemonType
	OsdType = "osd"

	// MdsType defines the mds DaemonType
	MdsType = "mds"

	// RgwType defines the rgw DaemonType
	RgwType = "rgw"

	// RbdMirrorType defines the rbd-mirror DaemonType
	RbdMirrorType = "rbd-mirror"

	// FilesystemMirrorType defines the fs-mirror DaemonType
	FilesystemMirrorType = "fs-mirror"

	// CrashType defines the crash collector DaemonType
	CrashType = "crashcollector"

	// CephUser is the Linux Ceph username
	CephUser = "ceph"

	// CephGroup is the Linux Ceph groupname
	CephGroup = "ceph"
)

var (
	// VarLibCephDir is simply "/var/lib/ceph". It is made overwritable only for unit tests where it
	// may be needed to send data intended for /var/lib/ceph to a temporary test dir.
	VarLibCephDir = "/var/lib/ceph"

	// EtcCephDir is simply "/etc/ceph". It is made overwritable only for unit tests where it
	// may be needed to send data intended for /etc/ceph to a temporary test dir.
	EtcCephDir = "/etc/ceph"

	// VarLogCephDir defines Ceph logging directory. It is made overwritable only for unit tests where it
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

// SetOrRemoveDefaultConfigs sets Rook's desired default configs in the centralized monitor database. This
// cannot be called before at least one monitor is established.
// Also, legacy options will be removed
func SetOrRemoveDefaultConfigs(
	context *clusterd.Context,
	clusterInfo *cephclient.ClusterInfo,
	clusterSpec cephv1.ClusterSpec,
) error {
	// ceph.conf is never used. All configurations are made in the centralized mon config database,
	// or they are specified on the commandline when daemons are called.
	monStore := GetMonStore(context, clusterInfo)

	if err := monStore.SetAll("global", DefaultCentralizedConfigs(clusterInfo.CephVersion)); err != nil {
		return errors.Wrapf(err, "failed to apply default Ceph configurations")
	}

	// When enabled the collector will logrotate logs from files
	if clusterSpec.LogCollector.Enabled {
		// Override "log file" for existing clusters since it is empty
		logOptions := map[string]string{
			"log to file": "true",
		}
		if err := monStore.SetAll("global", logOptions); err != nil {
			return errors.Wrapf(err, "failed to apply logging configuration for log collector")
		}
		// If the log collector is disabled we do not log to file since we collect nothing
	} else {
		logOptions := map[string]string{
			"log to file": "false",
		}
		if err := monStore.SetAll("global", logOptions); err != nil {
			return errors.Wrapf(err, "failed to apply logging configuration")
		}
	}

	// Apply Multus if needed
	if clusterSpec.Network.IsMultus() {
		logger.Info("configuring ceph network(s) with multus")
		cephNetworks, err := generateNetworkSettings(clusterInfo.Context, context, clusterInfo.Namespace, clusterSpec.Network.Selectors)
		if err != nil {
			return errors.Wrap(err, "failed to generate network settings")
		}

		// Apply ceph network settings to the mon config store
		if err := monStore.SetAll("global", cephNetworks); err != nil {
			return errors.Wrap(err, "failed to network config overrides")
		}
	}

	// This section will remove any previously configured option(s) from the mon centralized store
	// This is useful for scenarios where options are not needed anymore and we just want to reset to internal's default
	// On upgrade, the flag will be removed
	if err := monStore.DeleteAll(LegacyConfigs()...); err != nil {
		return errors.Wrap(err, "failed to remove legacy options")
	}

	return nil
}

func DisableInsecureGlobalID(context *clusterd.Context, clusterInfo *cephclient.ClusterInfo) {
	if !canDisableInsecureGlobalID(clusterInfo) {
		logger.Infof("cannot disable insecure global id on ceph version %v", clusterInfo.CephVersion.String())
		return
	}

	monStore := GetMonStore(context, clusterInfo)
	if err := monStore.Set("mon", "auth_allow_insecure_global_id_reclaim", "false"); err != nil {
		logger.Warningf("failed to disable the insecure global ID. %v", err)
	} else {
		logger.Info("insecure global ID is now disabled")
	}
}

func canDisableInsecureGlobalID(clusterInfo *cephclient.ClusterInfo) bool {
	cephver := clusterInfo.CephVersion
	if cephver.IsAtLeastQuincy() {
		return true
	}
	if cephver.IsPacific() && cephver.IsAtLeast(version.CephVersion{Major: 16, Minor: 2, Extra: 1}) {
		return true
	}
	return false
}
