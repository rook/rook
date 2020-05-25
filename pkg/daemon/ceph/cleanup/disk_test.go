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
	"testing"

	cephv1 "github.com/rook/rook/pkg/apis/ceph.rook.io/v1"
	"github.com/rook/rook/pkg/clusterd"
	"github.com/rook/rook/pkg/daemon/ceph/client"
	"github.com/stretchr/testify/assert"
)

func TestBuildDataSource(t *testing.T) {
	s := NewDiskSanitizer(&clusterd.Context{}, &client.ClusterInfo{}, &cephv1.SanitizeDisksSpec{})
	s.sanitizeDisksSpec.DataSource = cephv1.SanitizeDataSourceZero

	assert.Equal(t, "/dev/zero", s.buildDataSource())
}

func TestBuildShredArgs(t *testing.T) {
	var i int32 = 1
	c := &clusterd.Context{}
	disk := "/dev/sda"
	type fields struct {
		context           *clusterd.Context
		clusterInfo       *client.ClusterInfo
		sanitizeDisksSpec *cephv1.SanitizeDisksSpec
	}
	tests := []struct {
		name   string
		fields fields
		disk   string
		want   []string
	}{
		{"quick-zero", fields{c, &client.ClusterInfo{}, &cephv1.SanitizeDisksSpec{Method: cephv1.SanitizeMethodQuick, Iteration: i, DataSource: cephv1.SanitizeDataSourceZero}}, disk, []string{"--size=10M", "--random-source=/dev/zero", "--force", "--verbose", "--iterations=1", "/dev/sda"}},
		{"quick-random", fields{c, &client.ClusterInfo{}, &cephv1.SanitizeDisksSpec{Method: cephv1.SanitizeMethodQuick, Iteration: i, DataSource: cephv1.SanitizeDataSourceRandom}}, disk, []string{"--zero", "--size=10M", "--force", "--verbose", "--iterations=1", "/dev/sda"}},
		{"complete-zero", fields{c, &client.ClusterInfo{}, &cephv1.SanitizeDisksSpec{Method: cephv1.SanitizeMethodComplete, Iteration: i, DataSource: cephv1.SanitizeDataSourceZero}}, disk, []string{"--random-source=/dev/zero", "--force", "--verbose", "--iterations=1", "/dev/sda"}},
		{"complete-random", fields{c, &client.ClusterInfo{}, &cephv1.SanitizeDisksSpec{Method: cephv1.SanitizeMethodComplete, Iteration: i, DataSource: cephv1.SanitizeDataSourceRandom}}, disk, []string{"--zero", "--force", "--verbose", "--iterations=1", "/dev/sda"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := &DiskSanitizer{
				context:           tt.fields.context,
				clusterInfo:       tt.fields.clusterInfo,
				sanitizeDisksSpec: tt.fields.sanitizeDisksSpec,
			}
			if got := s.buildShredArgs(tt.disk); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("DiskSanitizer.buildShredArgs() = %v, want %v", got, tt.want)
			}
		})
	}
}
