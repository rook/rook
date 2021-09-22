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

// Package config allows a ceph config file to be stored in Kubernetes and mounted as volumes into
// Ceph daemon containers.
package config

import (
	"reflect"
	"testing"

	cephv1 "github.com/rook/rook/pkg/apis/ceph.rook.io/v1"
	"github.com/stretchr/testify/assert"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
)

func TestConfigureLivenessProbe(t *testing.T) {
	keyTypes := []cephv1.KeyType{
		cephv1.KeyMds,
		cephv1.KeyMon,
		cephv1.KeyMgr,
		cephv1.KeyOSD,
	}

	for _, keyType := range keyTypes {
		configLivenessProbeHelper(t, keyType)
	}
}

func configLivenessProbeHelper(t *testing.T, keyType cephv1.KeyType) {
	p := &v1.Probe{
		Handler: v1.Handler{
			HTTPGet: &v1.HTTPGetAction{
				Path: "/",
				Port: intstr.FromInt(8080),
			},
		},
	}
	container := v1.Container{LivenessProbe: p}
	l := map[cephv1.KeyType]*cephv1.ProbeSpec{keyType: {Disabled: true}}
	type args struct {
		daemon      cephv1.KeyType
		container   v1.Container
		healthCheck cephv1.CephClusterHealthCheckSpec
	}
	tests := []struct {
		name string
		args args
		want v1.Container
	}{
		{"probe-enabled", args{keyType, container, cephv1.CephClusterHealthCheckSpec{}}, container},
		{"probe-disabled", args{keyType, container, cephv1.CephClusterHealthCheckSpec{LivenessProbe: l}}, v1.Container{}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := ConfigureLivenessProbe(tt.args.daemon, tt.args.container, tt.args.healthCheck); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("ConfigureLivenessProbe() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestGetLivenessProbeWithDefaults(t *testing.T) {
	t.Run("using default probe", func(t *testing.T) {
		currentProb := &v1.Probe{
			Handler: v1.Handler{
				Exec: &v1.ExecAction{
					// Example:
					Command: []string{
						"env",
						"-i",
						"sh",
						"-c",
						"ceph --admin-daemon /run/ceph/ceph-mon.c.asok mon_status",
					},
				},
			},
			InitialDelaySeconds: 10,
		}
		// in case of default probe
		desiredProbe := &v1.Probe{}
		desiredProbe = GetLivenessProbeWithDefaults(desiredProbe, currentProb)
		assert.Equal(t, desiredProbe, currentProb)
	})

	t.Run("overriding default probes", func(t *testing.T) {
		currentProb := &v1.Probe{
			Handler: v1.Handler{
				Exec: &v1.ExecAction{
					// Example:
					Command: []string{
						"env",
						"-i",
						"sh",
						"-c",
						"ceph --admin-daemon /run/ceph/ceph-mon.c.asok mon_status",
					},
				},
			},
			InitialDelaySeconds: 10,
		}

		desiredProbe := &v1.Probe{
			Handler: v1.Handler{
				Exec: &v1.ExecAction{
					// Example:
					Command: []string{
						"env",
						"-i",
						"sh",
						"-c",
						"ceph --admin-daemon /run/ceph/ceph-mon.foo.asok mon_status",
					},
				},
			},
			InitialDelaySeconds: 1,
			FailureThreshold:    2,
			PeriodSeconds:       3,
			SuccessThreshold:    4,
			TimeoutSeconds:      5,
		}
		desiredProbe = GetLivenessProbeWithDefaults(desiredProbe, currentProb)
		assert.Equal(t, desiredProbe.Exec.Command, []string{"env", "-i", "sh", "-c", "ceph --admin-daemon /run/ceph/ceph-mon.c.asok mon_status"})
		assert.Equal(t, desiredProbe.InitialDelaySeconds, int32(1))
		assert.Equal(t, desiredProbe.FailureThreshold, int32(2))
		assert.Equal(t, desiredProbe.PeriodSeconds, int32(3))
		assert.Equal(t, desiredProbe.SuccessThreshold, int32(4))
		assert.Equal(t, desiredProbe.TimeoutSeconds, int32(5))
	})
}
