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
	"github.com/rook/rook/pkg/clusterd"
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
)

// Config is a Ceph config struct containing zero or more Ceph config sections.
type Config struct {
	sections        map[string]*Section
	sectionOrder    []string
	context         *clusterd.Context
	namespace       string
	ownerRef        *metav1.OwnerReference
	dataDirHostPath string
}

// A Section is a collection of zero or more Ceph config key-value pairs.
type Section struct {
	configs     map[string]string
	configOrder []string // allows us to make an ordered dict
}

// normalizeKey converts a key in any format to a key with underscores.
//
// The internal representation of Ceph config keys uses underscores only, where Ceph supports both
// spaces, underscores, and hyphens. This is so that Rook can properly match and override keys even
// when they are specified as "some config key" in one section, "some_config_key" in another
// section, and "some-config-key" in yet another section.
func normalizeKey(key string) string {
	return strings.Replace(strings.Replace(key, " ", "_", -1), "-", "_", -1)
}

// NewConfig returns a new, blank Ceph Config.
func NewConfig() *Config {
	return &Config{
		sections:     make(map[string]*Section),
		sectionOrder: make([]string, 0, 1), // capacity=1 b/c usually we'll only create [global]
	}
}

// Section returns a Ceph config Section with the given name from the Config. If the section does
// not exist, a new empty section is created and returned for the caller's use.
func (c *Config) Section(name string) *Section {
	s, ok := c.sections[name]
	if !ok {
		s = &Section{
			configs:     make(map[string]string),
			configOrder: []string{},
		}
		c.sections[name] = s
		c.sectionOrder = append(c.sectionOrder, name)
	}
	return s
}

// Set sets the Ceph config key to the given value. Set accepts Ceph keys in both
// space-separated-words and underscore-separated-words formats. Set modifies the section in-place
// and also returns the same modified-in-place Section to give flexibility of usage.
//
// NOTE that if a config item in a map should have quotes around it, they will have to be added to
// the string explicitly. e.g., a Value of " test " will be interpreted as as 'Key = test', whereas
// a Value of "\" test \"" will be interpreted as 'Key = " test "'
func (s *Section) Set(key, value string) *Section {
	key = normalizeKey(key)
	_, ok := s.configs[key]
	if !ok {
		s.configOrder = append(s.configOrder, key)
	}
	s.configs[key] = value
	return s
}

// Merge modifies the Section in-place adding values from the overrides Section and overriding
// existing values with the new value in overrides.
func (s *Section) Merge(overrides *Section) {
	for _, k := range overrides.configOrder {
		if v, ok := overrides.configs[k]; ok {
			s.Set(k, v)
			continue
		}
		// this condition should never exist except in case of some kind of mem corruption
		logger.Errorf("override config key %s could not be applied. possible struct corruption of overrides: %+v", k, overrides)
	}

}

// Merge modifies the Config in-place, adding Sections from the overrides config, and overriding
// existing Sections with Section values from overrides.
func (c *Config) Merge(overrides *Config) {
	for _, hdr := range overrides.sectionOrder {
		if s, ok := overrides.sections[hdr]; ok {
			c.Section(hdr).Merge(s)
			continue
		}
		// this condition should never exist except in case of some kind of mem corruption
		logger.Errorf("override section %s could not be applied. possible struct corruption of overrides: %+v", hdr, overrides)
	}
}

// IniFile converts the Ceph Config to an *ini.File.
func (c *Config) IniFile() (*ini.File, error) {
	f := ini.Empty()
	for _, hdr := range c.sectionOrder {
		sec, ok := c.sections[hdr]
		if !ok {
			return nil, fmt.Errorf("Error: section header %s not found: %+v", hdr, c)
		}
		s, err := f.NewSection(hdr)
		if err != nil {
			return nil, fmt.Errorf("Error converting config to ini file text: %+v. %+v", c, err)
		}
		for _, k := range sec.configOrder {
			v, ok := sec.configs[k]
			if !ok {
				return nil, fmt.Errorf("Error: config %s not found in section %s: %+v", k, hdr, sec)
			}
			if _, err := s.NewKey(k, v); err != nil {
				return nil, fmt.Errorf("Error converting config to ini file text: %+v. %+v", c, err)
			}
		}
	}
	return f, nil
}

// NewFlag returns the key-value pair in the format of a Ceph command line-compatible flag.
func NewFlag(key, value string) string {
	// A flag is a normalized key with underscores replaced by dashes.
	// "debug default" ~normalize~> "debug_default" ~to~flag~> "debug-default"
	n := normalizeKey(key)
	f := strings.Replace(n, "_", "-", -1)
	return fmt.Sprintf("--%s=%s", f, value)
}

// GlobalFlags converts the Config's 'global' Section to a list of command line flag strings which
// can be used with Kubernetes container specs for Ceph daemons.
//
// It should never be necessary to get flags for anything but the 'global' config section,
// otherwise, Rook will have to determine which sections apply for a given Ceph daemon. Does
// 'mon.a' apply to 'mon.b'? 'mon.aa'? 'osd.0'? The logic would be prone to errors, so only allow
// the 'global' section to be converted to flags.
func (c *Config) GlobalFlags() []string {
	f := []string{}
	g := c.Section("global")
	for _, k := range g.configOrder {
		if v, ok := g.configs[k]; ok {
			f = append(f, NewFlag(k, v))
			continue
		}
		// this condition should never exist except in case of some kind of mem corruption
		logger.Errorf("override config key %s could not be applied. possible struct corruption of section: %+v", k, g)
	}
	return f
}
