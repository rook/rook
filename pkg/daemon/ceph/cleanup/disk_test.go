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
				if strings.Contains(args[0], "sdb") { // 500GB
					return `NAME="sdc" SIZE="500000000000" TYPE="disk" PKNAME=""`, nil
				}
				if strings.Contains(args[0], "sdc") { // 80GB
					return `NAME="sdd" SIZE="80000000000" TYPE="disk" PKNAME=""`, nil
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
		{"quick-zero-2tb", fields{c, &client.ClusterInfo{}, &cephv1.SanitizeDisksSpec{Method: cephv1.SanitizeMethodQuick, Iteration: i, DataSource: cephv1.SanitizeDataSourceZero}}, "/dev/sda", []ShredCommand{
			{command: "dd", args: []string{"if=/dev/zero", "of=/dev/sda", "bs=10485760", "count=1", "oflag=direct,dsync", "seek=0"}},
			{command: "dd", args: []string{"if=/dev/zero", "of=/dev/sda", "bs=1024", "count=200", "oflag=direct,dsync,seek_bytes", "seek=1073741824"}},
			{command: "dd", args: []string{"if=/dev/zero", "of=/dev/sda", "bs=1024", "count=200", "oflag=direct,dsync,seek_bytes", "seek=10737418240"}},
			{command: "dd", args: []string{"if=/dev/zero", "of=/dev/sda", "bs=1024", "count=200", "oflag=direct,dsync,seek_bytes", "seek=107374182400"}},
			{command: "dd", args: []string{"if=/dev/zero", "of=/dev/sda", "bs=1024", "count=200", "oflag=direct,dsync,seek_bytes", "seek=1073741824000"}},
		}},
		{"quick-random-2tb", fields{c, &client.ClusterInfo{}, &cephv1.SanitizeDisksSpec{Method: cephv1.SanitizeMethodQuick, Iteration: i, DataSource: cephv1.SanitizeDataSourceRandom}}, "/dev/sda", []ShredCommand{
			{command: "dd", args: []string{"if=/dev/urandom", "of=/dev/sda", "bs=10485760", "count=1", "oflag=direct,dsync", "seek=0"}},
			{command: "dd", args: []string{"if=/dev/urandom", "of=/dev/sda", "bs=1024", "count=200", "oflag=direct,dsync,seek_bytes", "seek=1073741824"}},
			{command: "dd", args: []string{"if=/dev/urandom", "of=/dev/sda", "bs=1024", "count=200", "oflag=direct,dsync,seek_bytes", "seek=10737418240"}},
			{command: "dd", args: []string{"if=/dev/urandom", "of=/dev/sda", "bs=1024", "count=200", "oflag=direct,dsync,seek_bytes", "seek=107374182400"}},
			{command: "dd", args: []string{"if=/dev/urandom", "of=/dev/sda", "bs=1024", "count=200", "oflag=direct,dsync,seek_bytes", "seek=1073741824000"}},
			{command: "dd", args: []string{"if=/dev/zero", "of=/dev/sda", "bs=10485760", "count=1", "oflag=direct,dsync", "seek=0"}},
			{command: "dd", args: []string{"if=/dev/zero", "of=/dev/sda", "bs=1024", "count=200", "oflag=direct,dsync,seek_bytes", "seek=1073741824"}},
			{command: "dd", args: []string{"if=/dev/zero", "of=/dev/sda", "bs=1024", "count=200", "oflag=direct,dsync,seek_bytes", "seek=10737418240"}},
			{command: "dd", args: []string{"if=/dev/zero", "of=/dev/sda", "bs=1024", "count=200", "oflag=direct,dsync,seek_bytes", "seek=107374182400"}},
			{command: "dd", args: []string{"if=/dev/zero", "of=/dev/sda", "bs=1024", "count=200", "oflag=direct,dsync,seek_bytes", "seek=1073741824000"}},
		}},
		{"quick-zero-500gb", fields{c, &client.ClusterInfo{}, &cephv1.SanitizeDisksSpec{Method: cephv1.SanitizeMethodQuick, Iteration: i, DataSource: cephv1.SanitizeDataSourceZero}}, "/dev/sdb", []ShredCommand{
			{command: "dd", args: []string{"if=/dev/zero", "of=/dev/sdb", "bs=10485760", "count=1", "oflag=direct,dsync", "seek=0"}},
			{command: "dd", args: []string{"if=/dev/zero", "of=/dev/sdb", "bs=1024", "count=200", "oflag=direct,dsync,seek_bytes", "seek=1073741824"}},
			{command: "dd", args: []string{"if=/dev/zero", "of=/dev/sdb", "bs=1024", "count=200", "oflag=direct,dsync,seek_bytes", "seek=10737418240"}},
			{command: "dd", args: []string{"if=/dev/zero", "of=/dev/sdb", "bs=1024", "count=200", "oflag=direct,dsync,seek_bytes", "seek=107374182400"}},
		}},
		{"quick-random-500gb", fields{c, &client.ClusterInfo{}, &cephv1.SanitizeDisksSpec{Method: cephv1.SanitizeMethodQuick, Iteration: i, DataSource: cephv1.SanitizeDataSourceRandom}}, "/dev/sdb", []ShredCommand{
			{command: "dd", args: []string{"if=/dev/urandom", "of=/dev/sdb", "bs=10485760", "count=1", "oflag=direct,dsync", "seek=0"}},
			{command: "dd", args: []string{"if=/dev/urandom", "of=/dev/sdb", "bs=1024", "count=200", "oflag=direct,dsync,seek_bytes", "seek=1073741824"}},
			{command: "dd", args: []string{"if=/dev/urandom", "of=/dev/sdb", "bs=1024", "count=200", "oflag=direct,dsync,seek_bytes", "seek=10737418240"}},
			{command: "dd", args: []string{"if=/dev/urandom", "of=/dev/sdb", "bs=1024", "count=200", "oflag=direct,dsync,seek_bytes", "seek=107374182400"}},
			{command: "dd", args: []string{"if=/dev/zero", "of=/dev/sdb", "bs=10485760", "count=1", "oflag=direct,dsync", "seek=0"}},
			{command: "dd", args: []string{"if=/dev/zero", "of=/dev/sdb", "bs=1024", "count=200", "oflag=direct,dsync,seek_bytes", "seek=1073741824"}},
			{command: "dd", args: []string{"if=/dev/zero", "of=/dev/sdb", "bs=1024", "count=200", "oflag=direct,dsync,seek_bytes", "seek=10737418240"}},
			{command: "dd", args: []string{"if=/dev/zero", "of=/dev/sdb", "bs=1024", "count=200", "oflag=direct,dsync,seek_bytes", "seek=107374182400"}},
		}},
		{"quick-zero-80gb", fields{c, &client.ClusterInfo{}, &cephv1.SanitizeDisksSpec{Method: cephv1.SanitizeMethodQuick, Iteration: i, DataSource: cephv1.SanitizeDataSourceZero}}, "/dev/sdc", []ShredCommand{
			{command: "dd", args: []string{"if=/dev/zero", "of=/dev/sdc", "bs=10485760", "count=1", "oflag=direct,dsync", "seek=0"}},
			{command: "dd", args: []string{"if=/dev/zero", "of=/dev/sdc", "bs=1024", "count=200", "oflag=direct,dsync,seek_bytes", "seek=1073741824"}},
			{command: "dd", args: []string{"if=/dev/zero", "of=/dev/sdc", "bs=1024", "count=200", "oflag=direct,dsync,seek_bytes", "seek=10737418240"}},
		}},
		{"quick-random-80gb", fields{c, &client.ClusterInfo{}, &cephv1.SanitizeDisksSpec{Method: cephv1.SanitizeMethodQuick, Iteration: i, DataSource: cephv1.SanitizeDataSourceRandom}}, "/dev/sdc", []ShredCommand{
			{command: "dd", args: []string{"if=/dev/urandom", "of=/dev/sdc", "bs=10485760", "count=1", "oflag=direct,dsync", "seek=0"}},
			{command: "dd", args: []string{"if=/dev/urandom", "of=/dev/sdc", "bs=1024", "count=200", "oflag=direct,dsync,seek_bytes", "seek=1073741824"}},
			{command: "dd", args: []string{"if=/dev/urandom", "of=/dev/sdc", "bs=1024", "count=200", "oflag=direct,dsync,seek_bytes", "seek=10737418240"}},
			{command: "dd", args: []string{"if=/dev/zero", "of=/dev/sdc", "bs=10485760", "count=1", "oflag=direct,dsync", "seek=0"}},
			{command: "dd", args: []string{"if=/dev/zero", "of=/dev/sdc", "bs=1024", "count=200", "oflag=direct,dsync,seek_bytes", "seek=1073741824"}},
			{command: "dd", args: []string{"if=/dev/zero", "of=/dev/sdc", "bs=1024", "count=200", "oflag=direct,dsync,seek_bytes", "seek=10737418240"}},
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
