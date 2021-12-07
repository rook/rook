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

var (
	CephUserID = int64(167)
)

// NewSecurityContextConstraints returns a new SecurityContextConstraints for Rook-Ceph to run on
// OpenShift.
func NewSecurityContextConstraints(namespace string) []*secv1.SecurityContextConstraints {
	rookUserUID := int64(2016)
	return []*secv1.SecurityContextConstraints{
		{
			TypeMeta: metav1.TypeMeta{
				APIVersion: "security.openshift.io/v1",
				Kind:       "SecurityContextConstraints",
			},
			ObjectMeta: metav1.ObjectMeta{
				Name:      "rook-ceph-operator",
				Namespace: namespace,
			},

			// AllowHostDirVolumePlugin allows pod to use the hostPath plugin for volumes to write log files
			AllowHostDirVolumePlugin: true,
			ReadOnlyRootFilesystem:   false,
			AllowHostIPC:             false,
			AllowHostNetwork:         false,
			AllowHostPorts:           false,
			RequiredDropCapabilities: []corev1.Capability{},
			DefaultAddCapabilities:   []corev1.Capability{},
			AllowedCapabilities:      []corev1.Capability{},
			RunAsUser: secv1.RunAsUserStrategyOptions{
				Type: secv1.RunAsUserStrategyMustRunAs,
				UID:  &rookUserUID,
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
			Volumes: []secv1.FSType{},
			Users: []string{
				fmt.Sprintf("system:serviceaccount:%s:rook-ceph-system", namespace),
			},
		},
		{
			TypeMeta: metav1.TypeMeta{
				APIVersion: "security.openshift.io/v1",
				Kind:       "SecurityContextConstraints",
			},
			ObjectMeta: metav1.ObjectMeta{
				Name:      "rook-ceph",
				Namespace: namespace,
			},
			// AllowPrivilegedContainer is set to true so that the Rook-Ceph pods can run privileged
			// It is useful when preparing disks to become OSDs.
			AllowPrivilegedContainer: true,
			// AllowHostDirVolumePlugin allows pod to use the hostPath plugin for volumes to write log files
			AllowHostDirVolumePlugin: true,
			ReadOnlyRootFilesystem:   false,
			// AllowHostIPC is required when cluster-wide encryption is turned on
			AllowHostIPC:             true,
			AllowHostNetwork:         false,
			AllowHostPorts:           false,
			RequiredDropCapabilities: []corev1.Capability{},
			DefaultAddCapabilities:   []corev1.Capability{},
			AllowedCapabilities:      []corev1.Capability{"MKNOD"},
			RunAsUser: secv1.RunAsUserStrategyOptions{
				Type: secv1.RunAsUserStrategyMustRunAs,
				UID:  &CephUserID,
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
				fmt.Sprintf("system:serviceaccount:%s:default", namespace),
				fmt.Sprintf("system:serviceaccount:%s:rook-ceph-mgr", namespace),
				fmt.Sprintf("system:serviceaccount:%s:rook-ceph-osd", namespace),
			},
		},
	}
}
