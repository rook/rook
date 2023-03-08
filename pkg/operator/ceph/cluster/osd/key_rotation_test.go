/*
Copyright 2023 The Rook Authors. All rights reserved.

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
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func Test_keyRotationCronJobName(t *testing.T) {
	type args struct {
		osdID int
	}
	tests := []struct {
		name string
		args args
		want string
	}{
		{
			name: "OSD ID 0",
			args: args{
				osdID: 0,
			},
			want: fmt.Sprintf(keyRotationCronJobAppNameFmt, 0),
		},
		{
			name: "OSD ID 1",
			args: args{
				osdID: 1,
			},
			want: fmt.Sprintf(keyRotationCronJobAppNameFmt, 1),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := keyRotationCronJobName(tt.args.osdID); got != tt.want {
				t.Errorf("keyRotationCronJobName() = %v, want %v", got, tt.want)
			}
		})
	}
}

func Test_applyKeyRotationPlacement(t *testing.T) {
	type args struct {
		spec   *v1.PodSpec
		labels map[string]string
	}
	tests := []struct {
		name string
		args args
	}{
		{
			name: "test 1",
			args: args{
				spec: &v1.PodSpec{
					Affinity: &v1.Affinity{
						PodAffinity:     &v1.PodAffinity{},
						PodAntiAffinity: &v1.PodAntiAffinity{},
					},
					TopologySpreadConstraints: []v1.TopologySpreadConstraint{},
				},
				labels: map[string]string{
					"app":    "rook-ceph-osd",
					"osd-id": "0",
				},
			},
		},
		{
			name: "Affinity is nil",
			args: args{
				spec: &v1.PodSpec{
					Affinity:                  nil,
					TopologySpreadConstraints: []v1.TopologySpreadConstraint{},
				},
				labels: map[string]string{
					"app":    "rook-ceph-osd",
					"osd-id": "0",
				},
			},
		},
	}
	for _, tt := range tests {
		currentTT := tt
		t.Run(currentTT.name, func(t *testing.T) {
			applyKeyRotationPlacement(currentTT.args.spec, currentTT.args.labels)
			assert.Nil(t, currentTT.args.spec.Affinity.PodAntiAffinity)
			assert.Nil(t, currentTT.args.spec.TopologySpreadConstraints)
			assert.Equal(t, currentTT.args.spec.Affinity.PodAffinity, &v1.PodAffinity{
				RequiredDuringSchedulingIgnoredDuringExecution: []v1.PodAffinityTerm{
					{
						LabelSelector: &metav1.LabelSelector{MatchLabels: currentTT.args.labels},
						TopologyKey:   v1.LabelHostname,
					},
				},
			})
		})
	}
}
