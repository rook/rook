/*
Copyright 2016 The Rook Authors. All rights reserved.

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
package osd

import (
	"os"
	"path"
	"strings"
	"testing"

	"github.com/rook/rook/pkg/clusterd"
	"github.com/rook/rook/pkg/daemon/ceph/client"
	oposd "github.com/rook/rook/pkg/operator/ceph/cluster/osd"
	"github.com/rook/rook/pkg/operator/ceph/cluster/osd/config"
	exectest "github.com/rook/rook/pkg/util/exec/test"
	"github.com/stretchr/testify/assert"
)

func TestOSDBootstrap(t *testing.T) {
	configDir := t.TempDir()

	executor := &exectest.MockExecutor{
		MockExecuteCommandWithOutput: func(command string, args ...string) (string, error) {
			return "{\"key\":\"mysecurekey\"}", nil
		},
	}

	context := &clusterd.Context{Executor: executor, ConfigDir: configDir}
	err := createOSDBootstrapKeyring(context, client.AdminTestClusterInfo("mycluster"), configDir)
	assert.Nil(t, err)

	targetPath := path.Join(configDir, bootstrapOsdKeyring)
	contents, err := os.ReadFile(targetPath)
	assert.Nil(t, err)
	assert.NotEqual(t, -1, strings.Index(string(contents), "[client.bootstrap-osd]"))
	assert.NotEqual(t, -1, strings.Index(string(contents), "key = mysecurekey"))
	assert.NotEqual(t, -1, strings.Index(string(contents), "caps mon = \"allow profile bootstrap-osd\""))
}

func TestDeviceClassFor(t *testing.T) {
	tests := []struct {
		name         string
		devices      []DesiredDevice
		pvcBacked    bool
		storeDefault string
		envDefault   string
		paths        []string
		expected     string
	}{
		{
			name:         "per-device wins over default",
			devices:      []DesiredDevice{{Name: "sda", DeviceClass: "fast"}},
			storeDefault: "ssd",
			paths:        []string{"/dev/sda"},
			expected:     "fast",
		},
		{
			name:         "non-PVC falls back to storeConfig default when no per-device match",
			devices:      []DesiredDevice{{Name: "sdb", DeviceClass: "fast"}},
			storeDefault: "ssd",
			paths:        []string{"/dev/sdc"},
			expected:     "ssd",
		},
		{
			name:         "PVC reads default from env var, ignoring storeConfig",
			pvcBacked:    true,
			envDefault:   "env-class",
			storeDefault: "ignored-when-pvc",
			paths:        []string{"/mnt/pvc-0"},
			expected:     "env-class",
		},
		{
			name:     "returns empty when nothing is configured",
			paths:    []string{"/dev/sda"},
			expected: "",
		},
		{
			name:         "IsFilter entries do not match concrete blockPaths",
			devices:      []DesiredDevice{{Name: "^sd.$", DeviceClass: "huge", IsFilter: true}},
			storeDefault: "ssd",
			paths:        []string{"/dev/sda"},
			expected:     "ssd",
		},
		{
			name:         "IsDevicePathFilter entries do not match concrete blockPaths",
			devices:      []DesiredDevice{{Name: "^/dev/sd", DeviceClass: "tiny", IsDevicePathFilter: true}},
			storeDefault: "ssd",
			paths:        []string{"/dev/sda"},
			expected:     "ssd",
		},
		{
			// Empty DeviceClass on a matching spec means "unset"; resolution
			// falls through to the default rather than returning "".
			name:         "matching entry with empty DeviceClass falls through to default",
			devices:      []DesiredDevice{{Name: "sda"}},
			storeDefault: "ssd",
			paths:        []string{"/dev/sda"},
			expected:     "ssd",
		},
		{
			name:     "matching entry with empty DeviceClass and no default returns empty",
			devices:  []DesiredDevice{{Name: "sda"}},
			paths:    []string{"/dev/sda"},
			expected: "",
		},
		{
			// filepath.Base("/dev/mapper/sda") is "sda"; a plain base-name match
			// would incorrectly pick up a {name: "sda"} spec.
			name:         "dm-mapper path does not match a short-name spec",
			devices:      []DesiredDevice{{Name: "sda", DeviceClass: "fast"}},
			storeDefault: "ssd",
			paths:        []string{"/dev/mapper/sda"},
			expected:     "ssd",
		},
		{
			name:         "fullpath spec matches via DevLinks candidate",
			devices:      []DesiredDevice{{Name: "/dev/disk/by-id/wwn-0xabc", DeviceClass: "fast"}},
			storeDefault: "ssd",
			paths:        []string{"/dev/disk/by-id/wwn-0xabc", "/dev/sda"},
			expected:     "fast",
		},
		{
			name:         "fullpath spec with no DevLinks candidate falls through to default",
			devices:      []DesiredDevice{{Name: "/dev/disk/by-id/wwn-0xabc", DeviceClass: "fast"}},
			storeDefault: "ssd",
			paths:        []string{"/dev/sda"},
			expected:     "ssd",
		},
		{
			name:         "first configured device wins when multiple could match",
			devices:      []DesiredDevice{{Name: "sda", DeviceClass: "first"}, {Name: "/dev/disk/by-id/wwn-0xabc", DeviceClass: "second"}},
			storeDefault: "ssd",
			paths:        []string{"/dev/sda", "/dev/disk/by-id/wwn-0xabc"},
			expected:     "first",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv(oposd.CrushDeviceClassVarName, tt.envDefault)
			a := &OsdAgent{
				devices:     tt.devices,
				pvcBacked:   tt.pvcBacked,
				storeConfig: config.StoreConfig{DeviceClass: tt.storeDefault},
			}
			assert.Equal(t, tt.expected, a.deviceClassByPath(tt.paths))
		})
	}
}

func TestDeviceClass(t *testing.T) {
	tests := []struct {
		name         string
		device       DesiredDevice
		pvcBacked    bool
		storeDefault string
		envDefault   string
		expected     string
	}{
		{
			name:         "per-device wins over default",
			device:       DesiredDevice{Name: "sda", DeviceClass: "fast"},
			storeDefault: "ssd",
			expected:     "fast",
		},
		{
			name:         "non-PVC falls back to storeConfig default",
			device:       DesiredDevice{Name: "sda"},
			storeDefault: "ssd",
			expected:     "ssd",
		},
		{
			name:         "PVC reads default from env var, ignoring storeConfig",
			device:       DesiredDevice{Name: "/mnt/pvc"},
			pvcBacked:    true,
			envDefault:   "env-class",
			storeDefault: "ignored-when-pvc",
			expected:     "env-class",
		},
		{
			name:     "empty everywhere returns empty",
			device:   DesiredDevice{Name: "sda"},
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv(oposd.CrushDeviceClassVarName, tt.envDefault)
			a := &OsdAgent{
				pvcBacked:   tt.pvcBacked,
				storeConfig: config.StoreConfig{DeviceClass: tt.storeDefault},
			}
			assert.Equal(t, tt.expected, a.deviceClass(tt.device))
		})
	}
}

func TestDeviceClassFor_NameNormalization(t *testing.T) {
	// A spec "sda" matches blockPath "/dev/sda" and vice versa.
	tests := []struct {
		specName  string
		blockPath string
	}{
		{"sda", "/dev/sda"},
		{"sda", "sda"},
		{"/dev/sdb", "sdb"},
		{"/dev/sdb", "/dev/sdb"},
	}
	for _, tt := range tests {
		t.Run(tt.specName+"_vs_"+tt.blockPath, func(t *testing.T) {
			a := &OsdAgent{
				devices: []DesiredDevice{{Name: tt.specName, DeviceClass: "fast"}},
			}
			assert.Equal(t, "fast", a.deviceClassByPath([]string{tt.blockPath}))
		})
	}
}
