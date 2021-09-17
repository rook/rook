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

// Package cluster to manage a Ceph cluster.
package cluster

import (
	"testing"

	cephv1 "github.com/rook/rook/pkg/apis/ceph.rook.io/v1"
)

func TestIsMonitoringEnabled(t *testing.T) {
	type args struct {
		daemon      string
		clusterSpec *cephv1.ClusterSpec
	}
	tests := []struct {
		name string
		args args
		want bool
	}{
		{"isEnabled", args{"mon", &cephv1.ClusterSpec{}}, true},
		{"isDisabled", args{"mon", &cephv1.ClusterSpec{HealthCheck: cephv1.CephClusterHealthCheckSpec{DaemonHealth: cephv1.DaemonHealthSpec{Monitor: cephv1.HealthCheckSpec{Disabled: true}}}}}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := isMonitoringEnabled(tt.args.daemon, tt.args.clusterSpec); got != tt.want {
				t.Errorf("isMonitoringEnabled() = %v, want %v", got, tt.want)
			}
		})
	}
}
