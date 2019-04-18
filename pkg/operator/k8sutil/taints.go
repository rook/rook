/*
Copyright 2019 The Rook Authors. All rights reserved.

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

package k8sutil

import (
	v1 "k8s.io/api/core/v1"
	scheduler "k8s.io/kubernetes/pkg/scheduler/api"
)

// WellKnownTaints is a list of well-known taint keys in the Kubernetes code base. Kubernetes may
// automatically apply these taints to nodes during runtime. Most will be added with the
// `NoSchedule` affect, but some are created with `NoExecute`. Rook may wish to ignore these taints
// when decided whether to modify resources it creates based on whether taints are likely to have
// been added by Kubernetes or by the user.
// see: https://github.com/kubernetes/kubernetes/blob/master/pkg/scheduler/api/well_known_labels.go
// and: https://kubernetes.io/docs/concepts/configuration/taint-and-toleration/#taint-based-evictions
var WellKnownTaints = []string{
	scheduler.TaintNodeNotReady,    // NoExecute
	scheduler.TaintNodeUnreachable, // NoExecute
	scheduler.TaintNodeUnschedulable,
	scheduler.TaintNodeMemoryPressure,
	scheduler.TaintNodeDiskPressure,
	scheduler.TaintNodeNetworkUnavailable,
	scheduler.TaintNodePIDPressure,
	scheduler.TaintExternalCloudProvider,
	scheduler.TaintNodeShutdown,
}

// TaintIsWellKnown returns true if the taint's key is in the WellKnownTaints list. False otherwise.
// See WellKnownTaints for more information.
func TaintIsWellKnown(t v1.Taint) bool {
	for _, w := range WellKnownTaints {
		if t.Key == w {
			return true
		}
	}
	return false
}
