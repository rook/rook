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
		cephv1.KeyRgw,
	}

	fs := cephv1.CephFilesystem{}
	healthCheck := cephv1.CephClusterHealthCheckSpec{}
	livenessProbes := map[cephv1.KeyType]*cephv1.ProbeSpec{
		"mds": fs.Spec.MetadataServer.LivenessProbe,
		"mon": healthCheck.LivenessProbe["mon"],
		"osd": healthCheck.LivenessProbe["osd"],
		"mgr": healthCheck.LivenessProbe["mgr"],
	}

	for _, keyType := range keyTypes {
		configLivenessProbeHelper(t, keyType, livenessProbes)
		integrationLivenessProbeCheck(t, keyType, livenessProbes)

	}
}

func configLivenessProbeHelper(t *testing.T, keyType cephv1.KeyType, livenessProbes map[cephv1.KeyType]*cephv1.ProbeSpec) {
	p := &v1.Probe{
		ProbeHandler: v1.ProbeHandler{
			HTTPGet: &v1.HTTPGetAction{
				Path: "/",
				Port: intstr.FromInt(8080),
			},
		},
	}
	container := v1.Container{LivenessProbe: p}

	got := ConfigureLivenessProbe(container, livenessProbes[keyType])
	assert.Equal(t, got, container)
	// Disabling Liveness Probe
	l := &cephv1.ProbeSpec{Disabled: true}
	livenessProbes[keyType] = l
	got = ConfigureLivenessProbe(container, livenessProbes[keyType])
	assert.Equal(t, got, v1.Container{})
}

func integrationLivenessProbeCheck(t *testing.T, keyType cephv1.KeyType, livenessProbes map[cephv1.KeyType]*cephv1.ProbeSpec) {
	t.Run("integration check: configured probes should override values", func(t *testing.T) {
		defaultProbe := &v1.Probe{
			ProbeHandler: v1.ProbeHandler{
				HTTPGet: &v1.HTTPGetAction{
					Path: "/",
					Port: intstr.FromInt(8443),
				},
			},
		}
		userProbe := &v1.Probe{
			ProbeHandler: v1.ProbeHandler{
				HTTPGet: &v1.HTTPGetAction{
					Path: "/custom/path",
					Port: intstr.FromInt(8080),
				},
			},
			InitialDelaySeconds: 999,
			TimeoutSeconds:      888,
			PeriodSeconds:       777,
			SuccessThreshold:    666,
			FailureThreshold:    555,
		}

		l := &cephv1.ProbeSpec{Disabled: false,
			Probe: userProbe}
		livenessProbes[keyType] = l

		container := v1.Container{StartupProbe: defaultProbe}

		got := ConfigureStartupProbe(container, livenessProbes[keyType])
		// the resultant container's startup probe should have been overridden, but the handler
		// should always be the rook-given default
		expectedProbe := *userProbe
		expectedProbe.ProbeHandler = defaultProbe.ProbeHandler
		assert.Equal(t, &expectedProbe, got.StartupProbe)
	})
}

func TestConfigureStartupProbe(t *testing.T) {
	keyTypes := []cephv1.KeyType{
		cephv1.KeyMds,
		cephv1.KeyMon,
		cephv1.KeyMgr,
		cephv1.KeyOSD,
		cephv1.KeyRgw,
	}

	store := cephv1.ObjectStoreSpec{}
	fs := cephv1.CephFilesystem{}
	healthCheck := cephv1.CephClusterHealthCheckSpec{}
	startupProbes := map[cephv1.KeyType]*cephv1.ProbeSpec{
		"rgw": store.HealthCheck.StartupProbe,
		"mds": fs.Spec.MetadataServer.StartupProbe,
		"mon": healthCheck.StartupProbe["mon"],
		"osd": healthCheck.StartupProbe["osd"],
		"mgr": healthCheck.StartupProbe["mgr"],
	}
	for _, keyType := range keyTypes {
		configStartupProbeHelper(t, keyType, startupProbes)
		integrationStartupProbeCheck(t, keyType, startupProbes)
	}
}

func configStartupProbeHelper(t *testing.T, keyType cephv1.KeyType, startupProbes map[cephv1.KeyType]*cephv1.ProbeSpec) {
	p := &v1.Probe{
		ProbeHandler: v1.ProbeHandler{
			HTTPGet: &v1.HTTPGetAction{
				Path: "/",
				Port: intstr.FromInt(8080),
			},
		},
	}
	container := v1.Container{StartupProbe: p}
	got := ConfigureStartupProbe(container, startupProbes[keyType])
	assert.Equal(t, got, container)
	// Disabling Startup Probe
	l := &cephv1.ProbeSpec{Disabled: true}
	startupProbes[keyType] = l
	got = ConfigureStartupProbe(container, startupProbes[keyType])
	assert.Equal(t, got, v1.Container{})
}

func integrationStartupProbeCheck(t *testing.T, keyType cephv1.KeyType, startupProbes map[cephv1.KeyType]*cephv1.ProbeSpec) {
	t.Run("integration check: configured probes should override values", func(t *testing.T) {
		defaultProbe := &v1.Probe{
			ProbeHandler: v1.ProbeHandler{
				HTTPGet: &v1.HTTPGetAction{
					Path: "/",
					Port: intstr.FromInt(8443),
				},
			},
		}
		userProbe := &v1.Probe{
			ProbeHandler: v1.ProbeHandler{
				HTTPGet: &v1.HTTPGetAction{
					Path: "/custom/path",
					Port: intstr.FromInt(8080),
				},
			},
			InitialDelaySeconds: 999,
			TimeoutSeconds:      888,
			PeriodSeconds:       777,
			SuccessThreshold:    666,
			FailureThreshold:    555,
		}

		l := &cephv1.ProbeSpec{Disabled: false,
			Probe: userProbe}
		startupProbes[keyType] = l

		container := v1.Container{StartupProbe: defaultProbe}

		got := ConfigureStartupProbe(container, startupProbes[keyType])
		// the resultant container's startup probe should have been overridden, but the handler
		// should always be the rook-given default
		expectedProbe := *userProbe
		expectedProbe.ProbeHandler = defaultProbe.ProbeHandler
		assert.Equal(t, &expectedProbe, got.StartupProbe)
	})
}

func TestGetProbeWithDefaults(t *testing.T) {
	t.Run("using default probe", func(t *testing.T) {
		currentProb := &v1.Probe{
			ProbeHandler: v1.ProbeHandler{
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
		desiredProbe = GetProbeWithDefaults(desiredProbe, currentProb)
		assert.Equal(t, desiredProbe, currentProb)
	})

	t.Run("overriding default probes", func(t *testing.T) {
		currentProb := &v1.Probe{
			ProbeHandler: v1.ProbeHandler{
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
			ProbeHandler: v1.ProbeHandler{
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
		desiredProbe = GetProbeWithDefaults(desiredProbe, currentProb)
		assert.Equal(t, desiredProbe.Exec.Command, []string{"env", "-i", "sh", "-c", "ceph --admin-daemon /run/ceph/ceph-mon.c.asok mon_status"})
		assert.Equal(t, desiredProbe.InitialDelaySeconds, int32(1))
		assert.Equal(t, desiredProbe.FailureThreshold, int32(2))
		assert.Equal(t, desiredProbe.PeriodSeconds, int32(3))
		assert.Equal(t, desiredProbe.SuccessThreshold, int32(4))
		assert.Equal(t, desiredProbe.TimeoutSeconds, int32(5))
	})
}
