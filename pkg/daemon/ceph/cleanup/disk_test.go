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
	exectest "github.com/rook/rook/pkg/util/exec/test"
	"github.com/stretchr/testify/assert"
)

func TestBuildDataSource(t *testing.T) {
	s := NewDiskSanitizer(&clusterd.Context{}, &client.ClusterInfo{}, &cephv1.SanitizeDisksSpec{})
	s.sanitizeDisksSpec.DataSource = cephv1.SanitizeDataSourceZero

	assert.Equal(t, "/dev/zero", s.buildDataSource())
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
