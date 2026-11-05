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
	"github.com/stretchr/testify/require"
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

		l := &cephv1.ProbeSpec{
			Disabled: false,
			Probe:    userProbe,
		}
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

		l := &cephv1.ProbeSpec{
			Disabled: false,
			Probe:    userProbe,
		}
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
	t.Run("nil current probe", func(t *testing.T) {
		desiredProbe := &v1.Probe{
			FailureThreshold:    1,
			PeriodSeconds:       2,
			SuccessThreshold:    3,
			TimeoutSeconds:      4,
			InitialDelaySeconds: 5,
		}
		assert.Nil(t, GetProbeWithDefaults(desiredProbe, nil))
	})
	t.Run("nil current probe with a desired exec handler", func(t *testing.T) {
		assert.Nil(t, GetProbeWithDefaults(crExecProbe(), nil))
	})
	t.Run("desired handlers of every type are replaced by the current handler", func(t *testing.T) {
		currentProbe := &v1.Probe{
			ProbeHandler: v1.ProbeHandler{
				TCPSocket: &v1.TCPSocketAction{Port: intstr.FromInt(4420)},
			},
			InitialDelaySeconds: 10,
			TimeoutSeconds:      5,
			FailureThreshold:    10,
		}
		desiredProbe := crExecProbe()
		desiredProbe.HTTPGet = &v1.HTTPGetAction{Path: "/", Port: intstr.FromInt(8080)}
		desiredProbe.GRPC = &v1.GRPCAction{Port: 9090}

		got := GetProbeWithDefaults(desiredProbe, currentProbe)

		require.NotNil(t, got)
		assert.Equal(t, currentProbe.ProbeHandler, got.ProbeHandler)
		assert.Equal(t, int32(11), got.InitialDelaySeconds)
		assert.Equal(t, int32(12), got.TimeoutSeconds)
		assert.Equal(t, int32(13), got.PeriodSeconds)
		assert.Equal(t, int32(1), got.SuccessThreshold)
		assert.Equal(t, int32(15), got.FailureThreshold)
		assert.Equal(t, new(int64(16)), got.TerminationGracePeriodSeconds)
	})
}

// crExecProbe returns a probe as a CR could carry it: an exec handler plus timing values.
func crExecProbe() *v1.Probe {
	return &v1.Probe{
		ProbeHandler: v1.ProbeHandler{
			Exec: &v1.ExecAction{Command: []string{"sh", "-c", "touch /tmp/cr-handler-ran"}},
		},
		InitialDelaySeconds:           11,
		TimeoutSeconds:                12,
		PeriodSeconds:                 13,
		SuccessThreshold:              1,
		FailureThreshold:              15,
		TerminationGracePeriodSeconds: new(int64(16)),
	}
}

func TestConfigureProbeWithoutRookDefault(t *testing.T) {
	t.Run("liveness", func(t *testing.T) {
		got := ConfigureLivenessProbe(v1.Container{Name: "daemon"}, &cephv1.ProbeSpec{Probe: crExecProbe()})
		assert.Nil(t, got.LivenessProbe)
	})
	t.Run("startup", func(t *testing.T) {
		got := ConfigureStartupProbe(v1.Container{Name: "daemon"}, &cephv1.ProbeSpec{Probe: crExecProbe()})
		assert.Nil(t, got.StartupProbe)
	})
	t.Run("readiness", func(t *testing.T) {
		got := ConfigureReadinessProbe(v1.Container{Name: "daemon"}, &cephv1.ProbeSpec{Probe: crExecProbe()})
		assert.Nil(t, got.ReadinessProbe)
	})
}

func TestConfigureReadinessProbe(t *testing.T) {
	rookProbe := func() *v1.Probe {
		return &v1.Probe{
			ProbeHandler:  v1.ProbeHandler{Exec: &v1.ExecAction{Command: []string{"rook-check"}}},
			PeriodSeconds: 10,
		}
	}

	t.Run("no CR settings keeps the rook probe", func(t *testing.T) {
		got := ConfigureReadinessProbe(v1.Container{ReadinessProbe: rookProbe()}, nil)
		assert.Equal(t, rookProbe(), got.ReadinessProbe)
	})
	t.Run("disabled", func(t *testing.T) {
		got := ConfigureReadinessProbe(v1.Container{ReadinessProbe: rookProbe()}, &cephv1.ProbeSpec{Disabled: true, Probe: crExecProbe()})
		assert.Nil(t, got.ReadinessProbe)
	})
	t.Run("CR handler is replaced by the rook handler", func(t *testing.T) {
		got := ConfigureReadinessProbe(v1.Container{ReadinessProbe: rookProbe()}, &cephv1.ProbeSpec{Probe: crExecProbe()})
		require.NotNil(t, got.ReadinessProbe)
		assert.Equal(t, rookProbe().ProbeHandler, got.ReadinessProbe.ProbeHandler)
		assert.Equal(t, int32(13), got.ReadinessProbe.PeriodSeconds)
	})
}
