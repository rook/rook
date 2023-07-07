/*
Copyright 2021 The Rook Authors. All rights reserved.

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
	"fmt"

	secv1 "github.com/openshift/api/security/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// NewSecurityContextConstraints returns a new SecurityContextConstraints for Rook-Ceph to run on
// OpenShift.
func NewSecurityContextConstraints(name, namespace string) *secv1.SecurityContextConstraints {
	return &secv1.SecurityContextConstraints{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "security.openshift.io/v1",
			Kind:       "SecurityContextConstraints",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
		AllowPrivilegedContainer: true,
		AllowHostDirVolumePlugin: true,
		ReadOnlyRootFilesystem:   false,
		AllowHostIPC:             true,
		AllowHostNetwork:         false,
		AllowHostPorts:           false,
		AllowedCapabilities:      []corev1.Capability{"MKNOD"},
		RequiredDropCapabilities: []corev1.Capability{"ALL"},
		DefaultAddCapabilities:   []corev1.Capability{},
		RunAsUser: secv1.RunAsUserStrategyOptions{
			Type: secv1.RunAsUserStrategyRunAsAny,
		},
		SELinuxContext: secv1.SELinuxContextStrategyOptions{
			Type: secv1.SELinuxStrategyMustRunAs,
		},
		FSGroup: secv1.FSGroupStrategyOptions{
			Type: secv1.FSGroupStrategyMustRunAs,
		},
		SupplementalGroups: secv1.SupplementalGroupsStrategyOptions{
			Type: secv1.SupplementalGroupsStrategyRunAsAny,
		},
		Volumes: []secv1.FSType{
			secv1.FSTypeConfigMap,
			secv1.FSTypeDownwardAPI,
			secv1.FSTypeEmptyDir,
			secv1.FSTypeHostPath,
			secv1.FSTypePersistentVolumeClaim,
			secv1.FSProjected,
			secv1.FSTypeSecret,
		},
		Users: []string{
			fmt.Sprintf("system:serviceaccount:%s:rook-ceph-system", namespace),
			fmt.Sprintf("system:serviceaccount:%s:default", namespace),
			fmt.Sprintf("system:serviceaccount:%s:rook-ceph-mgr", namespace),
			fmt.Sprintf("system:serviceaccount:%s:rook-ceph-osd", namespace),
			fmt.Sprintf("system:serviceaccount:%s:rook-ceph-rgw", namespace),
		},
	}
}
