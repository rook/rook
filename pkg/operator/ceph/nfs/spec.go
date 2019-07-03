/*
Copyright 2018 The Rook Authors. All rights reserved.

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

// Package nfs for NFS ganesha
package nfs

import (
	"fmt"
	"path"

	cephv1 "github.com/rook/rook/pkg/apis/ceph.rook.io/v1"
	opmon "github.com/rook/rook/pkg/operator/ceph/cluster/mon"
	opspec "github.com/rook/rook/pkg/operator/ceph/spec"
	"github.com/rook/rook/pkg/operator/k8sutil"
	apps "k8s.io/api/apps/v1"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
)

const (
	appName             = "rook-ceph-nfs"
	ganeshaConfigVolume = "ganesha-config"
	nfsPort             = 2049
)

func (c *CephNFSController) createCephNFSService(n cephv1.CephNFS, name string) error {
	labels := getLabels(n, name)
	svc := &v1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      instanceName(n, name),
			Namespace: n.Namespace,
			Labels:    labels,
		},
		Spec: v1.ServiceSpec{
			Selector: labels,
			Ports: []v1.ServicePort{
				{
					Name:       "nfs",
					Port:       nfsPort,
					TargetPort: intstr.FromInt(int(nfsPort)),
					Protocol:   v1.ProtocolTCP,
				},
			},
		},
	}
	k8sutil.SetOwnerRef(&svc.ObjectMeta, &c.ownerRef)
	if c.hostNetwork {
		svc.Spec.ClusterIP = v1.ClusterIPNone
	}

	svc, err := c.context.Clientset.CoreV1().Services(n.Namespace).Create(svc)
	if err != nil {
		if !errors.IsAlreadyExists(err) {
			return fmt.Errorf("failed to create ganesha service. %+v", err)
		}
		logger.Infof("ceph nfs service already created")
		return nil
	}

	logger.Infof("ceph nfs service running at %s:%d", svc.Spec.ClusterIP, nfsPort)
	return nil
}

func (c *CephNFSController) makeDeployment(n cephv1.CephNFS, name, configName string) *apps.Deployment {
	binariesEnvVar, binariesVolume, binariesMount := k8sutil.BinariesMountInfo()

	deployment := &apps.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      instanceName(n, name),
			Namespace: n.Namespace,
		},
	}
	n.Spec.Server.Annotations.ApplyToObjectMeta(&deployment.ObjectMeta)
	k8sutil.SetOwnerRef(&deployment.ObjectMeta, &c.ownerRef)
	configMapSource := &v1.ConfigMapVolumeSource{
		LocalObjectReference: v1.LocalObjectReference{Name: configName},
		Items:                []v1.KeyToPath{{Key: "config", Path: "ganesha.conf"}},
	}
	configVolume := v1.Volume{Name: ganeshaConfigVolume, VolumeSource: v1.VolumeSource{ConfigMap: configMapSource}}

	podSpec := v1.PodSpec{
		InitContainers: []v1.Container{c.initContainer(n, binariesEnvVar, binariesMount)},
		Containers:     []v1.Container{c.daemonContainer(n, name, binariesMount)},
		RestartPolicy:  v1.RestartPolicyAlways,
		Volumes: append(
			opspec.PodVolumes("", ""),
			configVolume,
			binariesVolume,
		),
		HostNetwork: c.hostNetwork,
	}
	if c.hostNetwork {
		podSpec.DNSPolicy = v1.DNSClusterFirstWithHostNet
	}
	n.Spec.Server.Placement.ApplyToPodSpec(&podSpec)

	podTemplateSpec := v1.PodTemplateSpec{
		ObjectMeta: metav1.ObjectMeta{
			Name:   instanceName(n, name),
			Labels: getLabels(n, name),
		},
		Spec: podSpec,
	}
	n.Spec.Server.Annotations.ApplyToObjectMeta(&podTemplateSpec.ObjectMeta)

	// Multiple replicas of the nfs service would be handled by creating a service and a new deployment for each one, rather than increasing the pod count here
	replicas := int32(1)
	deployment.Spec = apps.DeploymentSpec{
		Selector: &metav1.LabelSelector{
			MatchLabels: getLabels(n, name),
		},
		Template: podTemplateSpec,
		Replicas: &replicas,
	}
	return deployment
}

func (c *CephNFSController) initContainer(n cephv1.CephNFS, binariesEnvVar v1.EnvVar, binariesMount v1.VolumeMount) v1.Container {

	return v1.Container{
		Args: []string{
			"ceph",
			"nfs",
			"init",
		},
		Name:  opspec.ConfigInitContainerName,
		Image: c.rookImage,
		VolumeMounts: append(
			opspec.RookVolumeMounts(),
			binariesMount),
		Env: []v1.EnvVar{
			binariesEnvVar,
			opmon.ClusterNameEnvVar(n.Namespace),
			opmon.EndpointEnvVar(),
			opmon.AdminSecretEnvVar(),
			k8sutil.PodIPEnvVar(k8sutil.PrivateIPEnvVar),
			k8sutil.PodIPEnvVar(k8sutil.PublicIPEnvVar),
			k8sutil.ConfigOverrideEnvVar(),
		},
		Resources: n.Spec.Server.Resources,
	}
}

func (c *CephNFSController) daemonContainer(n cephv1.CephNFS, name string, binariesMount v1.VolumeMount) v1.Container {
	configMount := v1.VolumeMount{Name: ganeshaConfigVolume, MountPath: "/etc/ganesha"}

	return v1.Container{
		Command: []string{
			path.Join(k8sutil.BinariesMountPath, "tini"),
		},
		Args: []string{
			"--", path.Join(k8sutil.BinariesMountPath, "rook"),
			"ceph",
			"nfs",
			"run",
		},
		Name:  "ceph-nfs",
		Image: c.cephVersion.Image,
		VolumeMounts: append(
			opspec.CephVolumeMounts(),
			configMount,
			binariesMount,
		),
		Env: append(
			k8sutil.ClusterDaemonEnvVars(c.cephVersion.Image),
			v1.EnvVar{Name: "ROOK_CEPH_NFS_NAME", Value: name},
		),
		Resources: n.Spec.Server.Resources,
	}
}

func getLabels(n cephv1.CephNFS, name string) map[string]string {
	return map[string]string{
		k8sutil.AppAttr:     appName,
		k8sutil.ClusterAttr: n.Namespace,
		"ceph_nfs":          n.Name,
		"instance":          name,
	}
}
