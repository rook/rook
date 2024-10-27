/*
Copyright 2023 The Rook Authors. All rights reserved.

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

package cosi

import (
	"github.com/pkg/errors"
	cephv1 "github.com/rook/rook/pkg/apis/ceph.rook.io/v1"
	"github.com/rook/rook/pkg/operator/ceph/controller"
	opcontroller "github.com/rook/rook/pkg/operator/ceph/controller"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	defaultCOSISideCarImage    = "gcr.io/k8s-staging-sig-storage/objectstorage-sidecar:v20240513-v0.1.0-35-gefb3255"
	defaultCephCOSIDriverImage = "quay.io/ceph/cosi:v0.1.2"
)

func createCephCOSIDriverDeployment(cephCOSIDriver *cephv1.CephCOSIDriver) (*appsv1.Deployment, error) {
	cosiPodSpec, err := createCOSIPodSpec(cephCOSIDriver)
	if err != nil {
		return nil, errors.Wrap(err, "failed to create cosi pod spec")
	}
	strategy := appsv1.DeploymentStrategy{
		Type: appsv1.RecreateDeploymentStrategyType,
	}

	replica := int32(1)
	minReadySeconds := int32(30)
	progressDeadlineSeconds := int32(600)

	cephcosidriverDeployment := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      cephCOSIDriver.Name,
			Namespace: cephCOSIDriver.Namespace,
			Labels:    getCOSILabels(cephCOSIDriver.Name, cephCOSIDriver.Namespace),
		},
		Spec: appsv1.DeploymentSpec{
			RevisionHistoryLimit: controller.RevisionHistoryLimit(),
			Replicas:             &replica,
			Selector: &metav1.LabelSelector{
				MatchLabels: getCOSILabels(cephCOSIDriver.Name, cephCOSIDriver.Namespace),
			},
			Template:                cosiPodSpec,
			Strategy:                strategy,
			MinReadySeconds:         minReadySeconds,
			ProgressDeadlineSeconds: &progressDeadlineSeconds,
		},
	}

	return cephcosidriverDeployment, nil
}

func setCOSILabels(label map[string]string) {
	label["app.kubernetes.io/part-of"] = CephCOSIDriverName
	label["app.kubernetes.io/name"] = CephCOSIDriverName
	label["app.kubernetes.io/component"] = CephCOSIDriverName
}

func getCOSILabels(name, namespace string) map[string]string {
	label := controller.AppLabels(name, namespace)
	setCOSILabels(label)
	return label
}

func createCOSIPodSpec(cephCOSIDriver *cephv1.CephCOSIDriver) (corev1.PodTemplateSpec, error) {
	cosiDriverContainer := createCOSIDriverContainer(cephCOSIDriver)
	cosiSideCarContainer := createCOSISideCarContainer(cephCOSIDriver)

	podSpec := corev1.PodSpec{
		HostNetwork: opcontroller.EnforceHostNetwork(),
		Containers: []corev1.Container{
			cosiDriverContainer,
			cosiSideCarContainer,
		},
		SecurityContext:    &corev1.PodSecurityContext{},
		ServiceAccountName: DefaultServiceAccountName,
		Volumes: []corev1.Volume{
			{Name: cosiSocketVolumeName, VolumeSource: corev1.VolumeSource{EmptyDir: &corev1.EmptyDirVolumeSource{}}},
		},
	}

	cephCOSIDriver.Spec.Placement.ApplyToPodSpec(&podSpec)

	podTemplateSpec := corev1.PodTemplateSpec{
		ObjectMeta: metav1.ObjectMeta{
			Name:   CephCOSIDriverName,
			Labels: getCOSILabels(cephCOSIDriver.Name, cephCOSIDriver.Namespace),
		},
		Spec: podSpec,
	}

	return podTemplateSpec, nil
}

func createCOSIDriverContainer(cephCOSIDriver *cephv1.CephCOSIDriver) corev1.Container {
	cephCOSIDriveImage := defaultCephCOSIDriverImage
	if cephCOSIDriver.Spec.Image != "" {
		cephCOSIDriveImage = cephCOSIDriver.Spec.Image
	}
	return corev1.Container{
		Name:  CephCOSIDriverName,
		Image: cephCOSIDriveImage,
		Args: []string{
			"--driver-prefix=" + CephCOSIDriverPrefix,
		},
		Env: []corev1.EnvVar{
			{Name: "POD_NAMESPACE", ValueFrom: &corev1.EnvVarSource{FieldRef: &corev1.ObjectFieldSelector{FieldPath: "metadata.namespace"}}}},
		VolumeMounts: []corev1.VolumeMount{
			{Name: cosiSocketVolumeName, MountPath: cosiSocketMountPath},
		},
		Resources: cephCOSIDriver.Spec.Resources,
	}
}

func createCOSISideCarContainer(cephCOSIDriver *cephv1.CephCOSIDriver) corev1.Container {
	defaultCOSISideCarImage := defaultCOSISideCarImage
	if cephCOSIDriver.Spec.ObjectProvisionerImage != "" {
		defaultCOSISideCarImage = cephCOSIDriver.Spec.ObjectProvisionerImage
	}
	return corev1.Container{
		Name:  COSISideCarName,
		Image: defaultCOSISideCarImage,
		Args: []string{
			"--v=5",
		},
		Env: []corev1.EnvVar{
			{Name: "POD_NAMESPACE", ValueFrom: &corev1.EnvVarSource{FieldRef: &corev1.ObjectFieldSelector{FieldPath: "metadata.namespace"}}}},
		VolumeMounts: []corev1.VolumeMount{
			{Name: cosiSocketVolumeName, MountPath: cosiSocketMountPath},
		},
	}
}
