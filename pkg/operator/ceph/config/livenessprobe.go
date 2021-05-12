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
	rookv1 "github.com/rook/rook/pkg/apis/rook.io/v1"
	v1 "k8s.io/api/core/v1"
)

type fn func(cephv1.CephClusterHealthCheckSpec) *v1.Probe

// ConfigureLivenessProbe returns the desired liveness probe for a given daemon
func ConfigureLivenessProbe(daemon rookv1.KeyType, container v1.Container, healthCheck cephv1.CephClusterHealthCheckSpec) v1.Container {
	// Map of functions
	probeFnMap := map[rookv1.KeyType]fn{
		cephv1.KeyMon: cephv1.GetMonLivenessProbe,
		cephv1.KeyMgr: cephv1.GetMgrLivenessProbe,
		cephv1.KeyOSD: cephv1.GetOSDLivenessProbe,
		cephv1.KeyMds: cephv1.GetMdsLivenessProbe,
	}

	if _, ok := healthCheck.LivenessProbe[daemon]; ok {
		if healthCheck.LivenessProbe[daemon].Disabled {
			container.LivenessProbe = nil
		} else {
			probe := probeFnMap[daemon](healthCheck)
			// If the spec value is not empty, let's apply it along with default when some fields are not specified
			if probe != nil {
				// Set the liveness probe on the container to overwrite the default probe created by Rook
				container.LivenessProbe = GetLivenessProbeWithDefaults(probe, container.LivenessProbe)
			}
		}
	}

	return container
}

func GetLivenessProbeWithDefaults(desiredProbe, currentProbe *v1.Probe) *v1.Probe {
	// Do not replace the handler with the previous one!
	// On the first iteration, the handler appears empty and is then replaced by whatever first daemon value comes in
	// e.g: [env -i sh -c ceph --admin-daemon /run/ceph/ceph-mon.b.asok mon_status] - meaning mon b was the first picked in the list of mons
	// On the second iteration the value of mon b remains, since the pointer has been allocated
	// This means the handler is not empty anymore and not replaced by the current one which it should
	//
	// Let's always force the default handler, there is no reason to change it anyway since the underlying content is generated based on the daemon's name
	// so we can not make it generic via the spec
	desiredProbe.Handler = currentProbe.Handler

	// If the user has not specified thresholds and timeouts, set them to the same values as
	// in the default liveness probe created by Rook.
	if desiredProbe.FailureThreshold == 0 {
		desiredProbe.FailureThreshold = currentProbe.FailureThreshold
	}
	if desiredProbe.PeriodSeconds == 0 {
		desiredProbe.PeriodSeconds = currentProbe.PeriodSeconds
	}
	if desiredProbe.SuccessThreshold == 0 {
		desiredProbe.SuccessThreshold = currentProbe.SuccessThreshold
	}
	if desiredProbe.TimeoutSeconds == 0 {
		desiredProbe.TimeoutSeconds = currentProbe.TimeoutSeconds
	}
	if desiredProbe.InitialDelaySeconds == 0 {
		desiredProbe.InitialDelaySeconds = currentProbe.InitialDelaySeconds
	}

	return desiredProbe
}
