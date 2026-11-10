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
	cephv1 "github.com/rook/rook/pkg/apis/ceph.rook.io/v1"
	v1 "k8s.io/api/core/v1"
)

// ConfigureLivenessProbe returns the desired liveness probe for a given daemon
func ConfigureLivenessProbe(container v1.Container, livenessProbe *cephv1.ProbeSpec) v1.Container {
	container.LivenessProbe = probeFromSpec("liveness", container.Name, livenessProbe, container.LivenessProbe)
	return container
}

// ConfigureStartupProbe returns the desired startup probe for a given daemon
func ConfigureStartupProbe(container v1.Container, startupProbe *cephv1.ProbeSpec) v1.Container {
	container.StartupProbe = probeFromSpec("startup", container.Name, startupProbe, container.StartupProbe)
	return container
}

// ConfigureReadinessProbe returns the desired readiness probe for a given daemon
func ConfigureReadinessProbe(container v1.Container, readinessProbe *cephv1.ProbeSpec) v1.Container {
	container.ReadinessProbe = probeFromSpec("readiness", container.Name, readinessProbe, container.ReadinessProbe)
	return container
}

func probeFromSpec(probeType, containerName string, spec *cephv1.ProbeSpec, rookProbe *v1.Probe) *v1.Probe {
	if spec == nil {
		return rookProbe
	}
	if spec.Disabled {
		return nil
	}
	if spec.Probe == nil {
		return rookProbe
	}
	if rookProbe == nil {
		logger.Warningf("ignoring %s probe settings for container %q because rook has no %s probe for it", probeType, containerName, probeType)
		return nil
	}
	if spec.Probe.ProbeHandler != (v1.ProbeHandler{}) {
		logger.Warningf("ignoring the handler in the %s probe settings for container %q; only timing and threshold settings are applied to rook's %s probe", probeType, containerName, probeType)
	}
	return GetProbeWithDefaults(spec.Probe, rookProbe)
}

// GetProbeWithDefaults returns a copy of currentProbe, the probe Rook generated for a daemon, with
// the timing and threshold fields that are set in desiredProbe applied to it. The handler always
// comes from currentProbe, and any handler in desiredProbe is ignored. If currentProbe is nil, Rook
// has no handler to run and nil is returned.
func GetProbeWithDefaults(desiredProbe, currentProbe *v1.Probe) *v1.Probe {
	if currentProbe == nil {
		return nil
	}
	newProbe := currentProbe.DeepCopy()
	if desiredProbe == nil {
		return newProbe
	}

	if desiredProbe.InitialDelaySeconds != 0 {
		newProbe.InitialDelaySeconds = desiredProbe.InitialDelaySeconds
	}
	if desiredProbe.TimeoutSeconds != 0 {
		newProbe.TimeoutSeconds = desiredProbe.TimeoutSeconds
	}
	if desiredProbe.PeriodSeconds != 0 {
		newProbe.PeriodSeconds = desiredProbe.PeriodSeconds
	}
	if desiredProbe.SuccessThreshold != 0 {
		newProbe.SuccessThreshold = desiredProbe.SuccessThreshold
	}
	if desiredProbe.FailureThreshold != 0 {
		newProbe.FailureThreshold = desiredProbe.FailureThreshold
	}
	if desiredProbe.TerminationGracePeriodSeconds != nil {
		newProbe.TerminationGracePeriodSeconds = new(*desiredProbe.TerminationGracePeriodSeconds)
	}
	return newProbe
}
