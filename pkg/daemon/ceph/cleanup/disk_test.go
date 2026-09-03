/*
Copyright 2020 The Rook Authors. All rights reserved.

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

package cleanup

import (
	"reflect"
	"strings"
	"testing"

	"github.com/pkg/errors"
	cephv1 "github.com/rook/rook/pkg/apis/ceph.rook.io/v1"
	"github.com/rook/rook/pkg/clusterd"
	"github.com/rook/rook/pkg/daemon/ceph/client"
	oposd "github.com/rook/rook/pkg/operator/ceph/cluster/osd"
	exectest "github.com/rook/rook/pkg/util/exec/test"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBuildDataSource(t *testing.T) {
	s := NewDiskSanitizer(&clusterd.Context{}, &client.ClusterInfo{}, &cephv1.SanitizeDisksSpec{})
	s.sanitizeDisksSpec.DataSource = cephv1.SanitizeDataSourceZero

	assert.Equal(t, "/dev/zero", s.buildDataSource())
}

func TestReturnPVDevice(t *testing.T) {
	newSanitizer := func(output string, err error) *DiskSanitizer {
		executor := &exectest.MockExecutor{
			MockExecuteCommandWithOutput: func(command string, args ...string) (string, error) {
				return output, err
			},
		}
		return NewDiskSanitizer(&clusterd.Context{Executor: executor}, &client.ClusterInfo{}, &cephv1.SanitizeDisksSpec{})
	}

	t.Run("returns the physical volume", func(t *testing.T) {
		pvs, err := newSanitizer("/dev/sda:0-100", nil).returnPVDevice("/dev/vg/lv")
		require.NoError(t, err)
		assert.Equal(t, []string{"/dev/sda"}, pvs)
	})

	// lvs emits one line per segment, so an LV spanning several PVs must yield all of them.
	t.Run("returns every physical volume of a multi-segment lv", func(t *testing.T) {
		pvs, err := newSanitizer("/dev/sdb1:0-2559\n  /dev/sdc1:0-1279", nil).returnPVDevice("/dev/vg/lv")
		require.NoError(t, err)
		assert.Equal(t, []string{"/dev/sdb1", "/dev/sdc1"}, pvs)
	})

	t.Run("returns an error when lvs fails", func(t *testing.T) {
		_, err := newSanitizer("", errors.New("lvs failed")).returnPVDevice("/dev/vg/lv")
		assert.Error(t, err)
	})

	// Previously this returned an empty slice that the caller indexed at [0], panicking
	// before the raw OSD disks were ever sanitized.
	t.Run("returns an error instead of an empty slice", func(t *testing.T) {
		_, err := newSanitizer("", nil).returnPVDevice("/dev/vg/lv")
		assert.Error(t, err)
	})
}

func TestSanitizeLVMDisk(t *testing.T) {
	newSanitizer := func(lvsErr, zapErr error) *DiskSanitizer {
		executor := &exectest.MockExecutor{
			MockExecuteCommandWithOutput: func(command string, args ...string) (string, error) {
				return "/dev/sda:0-100", lvsErr
			},
			MockExecuteCommandWithCombinedOutput: func(command string, args ...string) (string, error) {
				return "", zapErr
			},
		}
		return NewDiskSanitizer(&clusterd.Context{Executor: executor}, &client.ClusterInfo{},
			&cephv1.SanitizeDisksSpec{Method: cephv1.SanitizeMethodQuick, DataSource: cephv1.SanitizeDataSourceZero, Iteration: 1})
	}

	osds := []oposd.OSDInfo{{ID: 0, BlockPath: "/dev/vg/lv"}}

	t.Run("no error when the osd is sanitized", func(t *testing.T) {
		assert.NoError(t, newSanitizer(nil, nil).SanitizeLVMDisk(osds))
	})

	// On master the failed lookup returned an empty slice that was indexed at [0], so this
	// panicked before the raw OSDs were reached. It must now report an error and still zap.
	t.Run("reports the lookup failure without panicking", func(t *testing.T) {
		zapped := false
		executor := &exectest.MockExecutor{
			MockExecuteCommandWithOutput: func(command string, args ...string) (string, error) {
				return "", errors.New("lvs failed")
			},
			MockExecuteCommandWithCombinedOutput: func(command string, args ...string) (string, error) {
				zapped = true
				return "", nil
			},
		}
		s := NewDiskSanitizer(&clusterd.Context{Executor: executor}, &client.ClusterInfo{}, &cephv1.SanitizeDisksSpec{})

		err := s.SanitizeLVMDisk(osds)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "/dev/vg/lv")
		assert.True(t, zapped, "the ceph-volume zap should still be attempted")
	})

	t.Run("reports a zap failure", func(t *testing.T) {
		err := newSanitizer(nil, errors.New("zap failed")).SanitizeLVMDisk(osds)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "osd 0")
	})
}

func TestExecuteSanitizeCommand(t *testing.T) {
	newSanitizer := func(err error) *DiskSanitizer {
		executor := &exectest.MockExecutor{
			MockExecuteCommandWithCombinedOutput: func(command string, args ...string) (string, error) {
				return "", err
			},
		}
		return NewDiskSanitizer(&clusterd.Context{Executor: executor}, &client.ClusterInfo{},
			&cephv1.SanitizeDisksSpec{Method: cephv1.SanitizeMethodQuick, DataSource: cephv1.SanitizeDataSourceZero, Iteration: 1})
	}

	t.Run("no error when the disk is sanitized", func(t *testing.T) {
		assert.NoError(t, newSanitizer(nil).executeSanitizeCommand(oposd.OSDInfo{ID: 0, BlockPath: "/dev/sda"}))
	})

	// The reported bug: the failure was logged and then discarded, so the job still exited 0.
	t.Run("error is reported when sanitizing fails", func(t *testing.T) {
		err := newSanitizer(errors.New("zap failed")).executeSanitizeCommand(oposd.OSDInfo{ID: 0, BlockPath: "/dev/sda"})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "/dev/sda")
	})
}

func TestWipeLVM(t *testing.T) {
	newSanitizer := func(err error) *DiskSanitizer {
		executor := &exectest.MockExecutor{
			MockExecuteCommandWithCombinedOutput: func(command string, args ...string) (string, error) {
				return "", err
			},
		}
		return NewDiskSanitizer(&clusterd.Context{Executor: executor}, &client.ClusterInfo{}, &cephv1.SanitizeDisksSpec{})
	}

	assert.NoError(t, newSanitizer(nil).wipeLVM(0))

	err := newSanitizer(errors.New("zap failed")).wipeLVM(3)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "osd 3")
}

func TestSanitizeRawDisk(t *testing.T) {
	newSanitizer := func(err error) *DiskSanitizer {
		executor := &exectest.MockExecutor{
			MockExecuteCommandWithCombinedOutput: func(command string, args ...string) (string, error) {
				return "", err
			},
		}
		return NewDiskSanitizer(&clusterd.Context{Executor: executor}, &client.ClusterInfo{},
			&cephv1.SanitizeDisksSpec{Method: cephv1.SanitizeMethodQuick, DataSource: cephv1.SanitizeDataSourceZero, Iteration: 1})
	}

	osds := []oposd.OSDInfo{{ID: 0, BlockPath: "/dev/sda"}, {ID: 1, BlockPath: "/dev/sdb"}}

	assert.NoError(t, newSanitizer(nil).SanitizeRawDisk(osds))

	// Every failing disk must be named, not just whichever one happened to fail first.
	err := newSanitizer(errors.New("zap failed")).SanitizeRawDisk(osds)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "/dev/sda")
	assert.Contains(t, err.Error(), "/dev/sdb")
}

func TestBuildShredCommands(t *testing.T) {
	var i int32 = 1

	executor := &exectest.MockExecutor{
		MockExecuteCommandWithOutput: func(command string, args ...string) (string, error) {
			logger.Infof("OUTPUT for %s %v", command, args)

			if command == "lsblk" {
				if strings.Contains(args[0], "sda") { // 2TB
					return `NAME="sdb" SIZE="2000000000000" TYPE="disk" PKNAME=""`, nil
				}
				return "", nil
			}

			if command == "sgdisk" {
				return "Disk identifier (GUID): 18484D7E-5287-4CE9-AC73-D02FB69055CE", nil
			}

			return "", errors.Errorf("unknown command %s %s", command, args)
		},
	}

	c := &clusterd.Context{Executor: executor}

	type fields struct {
		context           *clusterd.Context
		clusterInfo       *client.ClusterInfo
		sanitizeDisksSpec *cephv1.SanitizeDisksSpec
	}
	tests := []struct {
		name   string
		fields fields
		disk   string
		want   []ShredCommand
	}{
		{"quick-zero", fields{c, &client.ClusterInfo{}, &cephv1.SanitizeDisksSpec{Method: cephv1.SanitizeMethodQuick, Iteration: i, DataSource: cephv1.SanitizeDataSourceZero}}, "/dev/sda", []ShredCommand{
			{command: "ceph-volume", args: []string{"lvm", "zap", "/dev/sda"}},
		}},
		{"quick-random", fields{c, &client.ClusterInfo{}, &cephv1.SanitizeDisksSpec{Method: cephv1.SanitizeMethodQuick, Iteration: i, DataSource: cephv1.SanitizeDataSourceRandom}}, "/dev/sda", []ShredCommand{
			{command: "ceph-volume", args: []string{"lvm", "zap", "/dev/sda"}},
		}},
		{"complete-zero-2tb", fields{c, &client.ClusterInfo{}, &cephv1.SanitizeDisksSpec{Method: cephv1.SanitizeMethodComplete, Iteration: i, DataSource: cephv1.SanitizeDataSourceZero}}, "/dev/sda", []ShredCommand{
			{command: "shred", args: []string{"--random-source=/dev/zero", "--force", "--verbose", "--iterations=1", "/dev/sda"}},
		}},
		{"complete-random-2tb", fields{c, &client.ClusterInfo{}, &cephv1.SanitizeDisksSpec{Method: cephv1.SanitizeMethodComplete, Iteration: i, DataSource: cephv1.SanitizeDataSourceRandom}}, "/dev/sda", []ShredCommand{
			{command: "shred", args: []string{"--zero", "--force", "--verbose", "--iterations=1", "/dev/sda"}},
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := &DiskSanitizer{
				context:           tt.fields.context,
				clusterInfo:       tt.fields.clusterInfo,
				sanitizeDisksSpec: tt.fields.sanitizeDisksSpec,
			}
			if got := s.buildShredCommands(tt.disk); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("DiskSanitizer.buildShredArgs() = %v, want %v", got, tt.want)
			}
		})
	}
}
