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

package v1

import (
	corev1 "k8s.io/api/core/v1"
)

/*
 * Liveness probes
 */

// GetMonLivenessProbe returns the liveness probe for the MON service
func GetMonLivenessProbe(l CephClusterHealthCheckSpec) *corev1.Probe {
	return l.LivenessProbe[ResourcesKeyMon].Probe
}

// GetMgrLivenessProbe returns the liveness probe for the MGR service
func GetMgrLivenessProbe(l CephClusterHealthCheckSpec) *corev1.Probe {
	return l.LivenessProbe[ResourcesKeyMgr].Probe
}

// GetOSDLivenessProbe returns the liveness probe for the OSD service
func GetOSDLivenessProbe(l CephClusterHealthCheckSpec) *corev1.Probe {
	return l.LivenessProbe[ResourcesKeyOSD].Probe
}

// GetMdsLivenessProbe returns the liveness probe for the MDS service
func GetMdsLivenessProbe(l CephClusterHealthCheckSpec) *corev1.Probe {
	return l.LivenessProbe[ResourcesKeyMDS].Probe
}

/*
 * Startup probes
 */

// GetMonStartupProbe returns the startup probe for the MON service
func GetMonStartupProbe(l CephClusterHealthCheckSpec) *corev1.Probe {
	return l.StartupProbe[ResourcesKeyMon].Probe
}

// GetMgrStartupProbe returns the startup probe for the MGR service
func GetMgrStartupProbe(l CephClusterHealthCheckSpec) *corev1.Probe {
	return l.StartupProbe[ResourcesKeyMgr].Probe
}

// GetOSDStartupProbe returns the startup probe for the OSD service
func GetOSDStartupProbe(l CephClusterHealthCheckSpec) *corev1.Probe {
	return l.StartupProbe[ResourcesKeyOSD].Probe
}

// GetMdsStartupProbe returns the startup probe for the MDS service
func GetMdsStartupProbe(l CephClusterHealthCheckSpec) *corev1.Probe {
	return l.StartupProbe[ResourcesKeyMDS].Probe
}
