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

package k8sutil

import (
	"os"
	"strings"

	corev1 "k8s.io/api/core/v1"
)

// ParseStringToLabels parse a label selector string into a map[string]string
func ParseStringToLabels(in string) map[string]string {
	labels := map[string]string{}

	if in == "" {
		return labels
	}

	for _, v := range strings.Split(in, ",") {
		labelSplit := strings.Split(v, "=")

		// When a value is set for a label k/v pair
		if len(labelSplit) > 2 {
			logger.Warningf("more than one value found for a label %q, only the first value will be used", labelSplit[0])
		}

		if len(labelSplit) > 1 {
			labels[labelSplit[0]] = labelSplit[1]
		} else {
			labels[labelSplit[0]] = ""
		}
	}

	return labels
}

// AddRecommendedLabels adds the labels to the resources created by rook
// The labels added are name, instance,etc
func AddRecommendedLabels(labels map[string]string, appName, parentName, resourceKind, resourceInstance string) {
	labels["app.kubernetes.io/name"] = appName
	labels["app.kubernetes.io/instance"] = resourceInstance
	labels["app.kubernetes.io/component"] = resourceKind
	labels["app.kubernetes.io/part-of"] = parentName
	labels["app.kubernetes.io/managed-by"] = "rook-ceph-operator"
	labels["app.kubernetes.io/created-by"] = "rook-ceph-operator"
	labels["rook.io/operator-namespace"] = os.Getenv(PodNamespaceEnvVar)
}

// LabelHostname returns label name to identify k8s node hostname
func LabelHostname() string {
	if label := os.Getenv("ROOK_CUSTOM_HOSTNAME_LABEL"); label != "" {
		return label
	}
	return corev1.LabelHostname
}
