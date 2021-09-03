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
	"github.com/rook/rook/pkg/clusterd"
	testop "github.com/rook/rook/pkg/operator/test"
)

func TestPreClusterStartValidation(t *testing.T) {
	type args struct {
		cluster *cluster
	}
	tests := []struct {
		name    string
		args    args
		wantErr bool
	}{
		{"no settings", args{&cluster{Spec: &cephv1.ClusterSpec{}, context: &clusterd.Context{Clientset: testop.New(t, 3)}}}, false},
		{"even mons", args{&cluster{context: &clusterd.Context{Clientset: testop.New(t, 3)}, Spec: &cephv1.ClusterSpec{Mon: cephv1.MonSpec{Count: 2}}}}, false},
		{"missing stretch zones", args{&cluster{context: &clusterd.Context{Clientset: testop.New(t, 3)}, Spec: &cephv1.ClusterSpec{Mon: cephv1.MonSpec{StretchCluster: &cephv1.StretchClusterSpec{Zones: []cephv1.StretchClusterZoneSpec{
			{Name: "a"},
		}}}}}}, true},
		{"missing arbiter", args{&cluster{context: &clusterd.Context{Clientset: testop.New(t, 3)}, Spec: &cephv1.ClusterSpec{Mon: cephv1.MonSpec{StretchCluster: &cephv1.StretchClusterSpec{Zones: []cephv1.StretchClusterZoneSpec{
			{Name: "a"},
			{Name: "b"},
			{Name: "c"},
		}}}}}}, true},
		{"missing zone name", args{&cluster{context: &clusterd.Context{Clientset: testop.New(t, 3)}, Spec: &cephv1.ClusterSpec{Mon: cephv1.MonSpec{StretchCluster: &cephv1.StretchClusterSpec{Zones: []cephv1.StretchClusterZoneSpec{
			{Arbiter: true},
			{Name: "b"},
			{Name: "c"},
		}}}}}}, true},
		{"valid stretch cluster", args{&cluster{context: &clusterd.Context{Clientset: testop.New(t, 3)}, Spec: &cephv1.ClusterSpec{Mon: cephv1.MonSpec{Count: 3, StretchCluster: &cephv1.StretchClusterSpec{Zones: []cephv1.StretchClusterZoneSpec{
			{Name: "a", Arbiter: true},
			{Name: "b"},
			{Name: "c"},
		}}}}}}, false},
		{"not enough stretch nodes", args{&cluster{context: &clusterd.Context{Clientset: testop.New(t, 3)}, Spec: &cephv1.ClusterSpec{Mon: cephv1.MonSpec{Count: 5, StretchCluster: &cephv1.StretchClusterSpec{Zones: []cephv1.StretchClusterZoneSpec{
			{Name: "a", Arbiter: true},
			{Name: "b"},
			{Name: "c"},
		}}}}}}, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := preClusterStartValidation(tt.args.cluster); (err != nil) != tt.wantErr {
				t.Errorf("ClusterController.preClusterStartValidation() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}
