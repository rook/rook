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
	"strings"

	"github.com/coreos/pkg/capnslog"
	"github.com/go-ini/ini"
	rookceph "github.com/rook/rook/pkg/apis/ceph.rook.io/v1"
	"github.com/rook/rook/pkg/clusterd"
	cephconfig "github.com/rook/rook/pkg/daemon/ceph/config"
	"github.com/rook/rook/pkg/operator/k8sutil"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
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

	// chownUserGroup represents ceph:ceph
	chownUserGroup = CephUser + ":" + CephGroup

	// ContainerPostStartCmd is the command we run before starting any Ceph daemon
	// It makes sure Ceph directories are owned by 'ceph'
	ContainerPostStartCmd = []string{"chown", "--recursive", "--verbose", chownUserGroup, VarLogCephDir}
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

// SetDefaultAndUserConfigs sets Rook's desired default configs and the user's override configs from
// the CephCluster CRD in the centralized monitor database. This cannot be called before at least
// one monitor is established.
func SetDefaultAndUserConfigs(
	context *clusterd.Context,
	namespace string,
	clusterInfo *cephconfig.ClusterInfo,
	configOverrides rookceph.ConfigOverridesSpec,
) error {
	// ceph.conf is never used. All configurations are made in the centralized mon config database,
	// or they are specified on the commandline when daemons are called.
	monStore := GetMonStore(context, namespace)

	if err := monStore.SetAll(DefaultCentralizedConfigs(clusterInfo.CephVersion)); err != nil {
		return fmt.Errorf("failed to apply default Ceph configurations. %+v", err)
	}

	if err := monStore.SetAll(DefaultLegacyConfigs()); err != nil {
		return fmt.Errorf("failed to apply legacy config overrides. %+v", err)
	}

	// user-specified config overrides from the CRD will go here
	if err := monStore.SetAll(configOverrides); err != nil {
		return fmt.Errorf("failed to apply one or more user-specified overrides. %+v", err)
	}

	return nil
}

// LegacyConfigMapOverrideDefined checks to see if the legacy ConfigMap override exists with
// overrides defined. If it exists and has config overrides defined, this will return true. If it
// does not exist or exists without config overrides set, it will return false.
func LegacyConfigMapOverrideDefined(context *clusterd.Context, namespace string) (bool, error) {
	cm, err := context.Clientset.CoreV1().ConfigMaps(namespace).
		Get(k8sutil.ConfigOverrideName, metav1.GetOptions{})
	if err != nil {
		if errors.IsNotFound(err) {
			logger.Debugf("override configmap does not exist")
			return false, nil
		}
		// if we can't determine existence or not, assume it exists
		return true, fmt.Errorf("failed to see if legacy override ConfigMap exists. %+v", err)
	}

	o, ok := cm.Data[k8sutil.ConfigOverrideVal]
	if !ok || (ok && o == "") {
		// if no override set or override is empty string
		return false, nil
	}

	return true, nil
}

func configFileTextToOverrides(txt string) (rookceph.ConfigOverridesSpec, error) {
	overrides := []rookceph.ConfigOverride{}

	f, err := configFileTxtToINI(txt)
	if err != nil {
		return nil, err
	}
	sections := f.Sections()
	for _, s := range sections {
		who := s.Name()
		keys := s.Keys()
		for _, k := range keys {
			option := k.Name()
			value := k.String()
			overrides = append(overrides, configOverride(who, option, value))
		}
	}

	return overrides, nil
}

func configFileTxtToINI(txt string) (*ini.File, error) {
	f := ini.Empty()
	ovrTxt := []byte(txt)
	if err := f.Append(ovrTxt); err != nil {
		return nil, fmt.Errorf("failed to interpret text as INI file '''%s'''. %+v", ovrTxt, err)
	}
	return f, nil
}

func getOverrideConfigFromConfigMap(context *clusterd.Context, namespace string) string {
	cm, err := context.Clientset.CoreV1().ConfigMaps(namespace).
		Get(k8sutil.ConfigOverrideName, metav1.GetOptions{})
	if err != nil {
		if errors.IsNotFound(err) {
			logger.Debugf("override configmap does not exist")
			return "" // return empty string if cannot get override text
		}
		logger.Warningf("Error getting override configmap. %+v", err)
		return "" // return empty string if cannot get override text
	}
	// the override config map exists
	o, ok := cm.Data[k8sutil.ConfigOverrideVal]
	if !ok {
		logger.Warningf("The override config map does not have override text defined. %+v", o)
		return "" // return empty string if cannot get override text
	}
	return o
}
