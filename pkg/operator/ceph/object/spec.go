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

package object

import (
	"fmt"
	"strings"

	"github.com/pkg/errors"
	rook "github.com/rook/rook/pkg/apis/rook.io/v1alpha2"
	"github.com/rook/rook/pkg/operator/ceph/cluster/mon"
	cephconfig "github.com/rook/rook/pkg/operator/ceph/config"
	opspec "github.com/rook/rook/pkg/operator/ceph/spec"
	"github.com/rook/rook/pkg/operator/k8sutil"
	apps "k8s.io/api/apps/v1"
	v1 "k8s.io/api/core/v1"
	kerrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
)

func (c *clusterConfig) createDeployment(rgwConfig *rgwConfig) *apps.Deployment {
	replicas := int32(1)
	d := &apps.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      rgwConfig.ResourceName,
			Namespace: c.store.Namespace,
			Labels:    c.getLabels(),
		},
		Spec: apps.DeploymentSpec{
			Selector: &metav1.LabelSelector{
				MatchLabels: c.getLabels(),
			},
			Template: c.makeRGWPodSpec(rgwConfig),
			Replicas: &replicas,
			Strategy: apps.DeploymentStrategy{
				Type: apps.RecreateDeploymentStrategyType,
			},
		},
	}
	k8sutil.AddRookVersionLabelToDeployment(d)
	c.store.Spec.Gateway.Annotations.ApplyToObjectMeta(&d.ObjectMeta)
	opspec.AddCephVersionLabelToDeployment(c.clusterInfo.CephVersion, d)
	k8sutil.SetOwnerRef(&d.ObjectMeta, &c.ownerRef)

	return d
}

func (c *clusterConfig) makeRGWPodSpec(rgwConfig *rgwConfig) v1.PodTemplateSpec {
	podSpec := v1.PodSpec{
		InitContainers: []v1.Container{
			c.makeChownInitContainer(rgwConfig),
		},
		Containers: []v1.Container{
			c.makeDaemonContainer(rgwConfig),
		},
		RestartPolicy: v1.RestartPolicyAlways,
		Volumes: append(
			opspec.DaemonVolumes(c.DataPathMap, rgwConfig.ResourceName),
			c.mimeTypesVolume(),
		),
		HostNetwork:       c.clusterSpec.Network.IsHost(),
		PriorityClassName: c.store.Spec.Gateway.PriorityClassName,
	}
	// Replace default unreachable node toleration
	k8sutil.AddUnreachableNodeToleration(&podSpec)

	if c.clusterSpec.Network.IsHost() {
		podSpec.DNSPolicy = v1.DNSClusterFirstWithHostNet
	}

	// Set the ssl cert if specified
	if c.store.Spec.Gateway.SSLCertificateRef != "" {
		// Keep the SSL secret as secure as possible in the container. Give only user read perms.
		userReadOnly := int32(0400)
		certVol := v1.Volume{
			Name: certVolumeName,
			VolumeSource: v1.VolumeSource{
				Secret: &v1.SecretVolumeSource{
					SecretName: c.store.Spec.Gateway.SSLCertificateRef,
					Items: []v1.KeyToPath{
						{Key: certKeyName, Path: certFilename, Mode: &userReadOnly},
					}}}}
		podSpec.Volumes = append(podSpec.Volumes, certVol)
	}
	c.setPodPlacement(&podSpec, c.store.Spec.Gateway.Placement)

	podTemplateSpec := v1.PodTemplateSpec{
		ObjectMeta: metav1.ObjectMeta{
			Name:   rgwConfig.ResourceName,
			Labels: c.getLabels(),
		},
		Spec: podSpec,
	}
	c.store.Spec.Gateway.Annotations.ApplyToObjectMeta(&podTemplateSpec.ObjectMeta)

	return podTemplateSpec
}

func (c *clusterConfig) setPodPlacement(pod *v1.PodSpec, p rook.Placement) {
	p.ApplyToPodSpec(pod)

	// label selector for gateways used in anti-affinity rules
	podAntiAffinity := v1.PodAffinityTerm{
		LabelSelector: &metav1.LabelSelector{
			MatchLabels: c.getLabels(),
		},
		TopologyKey: v1.LabelHostname,
	}

	// ApplyToPodSpec ensures that pod.Affinity is non-nil
	if pod.Affinity.PodAntiAffinity == nil {
		pod.Affinity.PodAntiAffinity = &v1.PodAntiAffinity{}
	}
	paa := pod.Affinity.PodAntiAffinity

	// Set gateways pod anti-affinity rules when gateways should never be
	// co-located (e.g. HostNetworking)
	if c.clusterSpec.Network.IsHost() {
		paa.RequiredDuringSchedulingIgnoredDuringExecution =
			append(paa.RequiredDuringSchedulingIgnoredDuringExecution, podAntiAffinity)
	}
}

func (c *clusterConfig) makeChownInitContainer(rgwConfig *rgwConfig) v1.Container {
	return opspec.ChownCephDataDirsInitContainer(
		*c.DataPathMap,
		c.clusterSpec.CephVersion.Image,
		opspec.DaemonVolumeMounts(c.DataPathMap, rgwConfig.ResourceName),
		c.store.Spec.Gateway.Resources,
		mon.PodSecurityContext(),
	)
}

func (c *clusterConfig) makeDaemonContainer(rgwConfig *rgwConfig) v1.Container {
	// start the rgw daemon in the foreground
	container := v1.Container{
		Name:  "rgw",
		Image: c.clusterSpec.CephVersion.Image,
		Command: []string{
			"radosgw",
		},
		Args: append(
			append(
				opspec.DaemonFlags(c.clusterInfo, strings.TrimPrefix(generateCephXUser(rgwConfig.ResourceName), "client.")),
				"--foreground",
				cephconfig.NewFlag("rgw frontends", fmt.Sprintf("%s %s", rgwFrontend(c.clusterInfo.CephVersion), c.portString(c.clusterInfo.CephVersion))),
				cephconfig.NewFlag("host", opspec.ContainerEnvVarReference("POD_NAME")),
				cephconfig.NewFlag("rgw-mime-types-file", mimeTypesMountPath()),
			),
		),
		VolumeMounts: append(
			opspec.DaemonVolumeMounts(c.DataPathMap, rgwConfig.ResourceName),
			c.mimeTypesVolumeMount(),
		),
		Env:       opspec.DaemonEnvVars(c.clusterSpec.CephVersion.Image),
		Resources: c.store.Spec.Gateway.Resources,
		LivenessProbe: &v1.Probe{
			Handler: v1.Handler{
				HTTPGet: &v1.HTTPGetAction{
					Path: "/swift/healthcheck",
					Port: intstr.FromInt(int(c.store.Spec.Gateway.Port)),
				},
			},
			InitialDelaySeconds: 10,
		},
		SecurityContext: mon.PodSecurityContext(),
	}

	if c.store.Spec.Gateway.SSLCertificateRef != "" {
		// Add a volume mount for the ssl certificate
		mount := v1.VolumeMount{Name: certVolumeName, MountPath: certDir, ReadOnly: true}
		container.VolumeMounts = append(container.VolumeMounts, mount)
	}

	return container
}

func (c *clusterConfig) startService() (string, error) {
	labels := c.getLabels()
	svc := &v1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      c.instanceName(),
			Namespace: c.store.Namespace,
			Labels:    labels,
		},
		Spec: v1.ServiceSpec{
			Selector: labels,
		},
	}
	k8sutil.SetOwnerRef(&svc.ObjectMeta, &c.ownerRef)
	if c.clusterSpec.Network.IsHost() {
		svc.Spec.ClusterIP = v1.ClusterIPNone
	}

	addPort(svc, "http", c.store.Spec.Gateway.Port)
	addPort(svc, "https", c.store.Spec.Gateway.SecurePort)

	svc, err := c.context.Clientset.CoreV1().Services(c.store.Namespace).Create(svc)
	if err != nil {
		if !kerrors.IsAlreadyExists(err) {
			return "", errors.Wrapf(err, "failed to create rgw service")
		}
		svc, err = c.context.Clientset.CoreV1().Services(c.store.Namespace).Get(c.instanceName(), metav1.GetOptions{})
		if err != nil {
			return "", errors.Wrapf(err, "failed to get existing service IP")
		}
		return svc.Spec.ClusterIP, nil
	}

	logger.Infof("Gateway service running at %s:%d", svc.Spec.ClusterIP, c.store.Spec.Gateway.Port)
	return svc.Spec.ClusterIP, nil
}

func addPort(service *v1.Service, name string, port int32) {
	if port == 0 {
		return
	}
	service.Spec.Ports = append(service.Spec.Ports, v1.ServicePort{
		Name:       name,
		Port:       port,
		TargetPort: intstr.FromInt(int(port)),
		Protocol:   v1.ProtocolTCP,
	})
}

func (c *clusterConfig) getLabels() map[string]string {
	labels := opspec.PodLabels(AppName, c.store.Namespace, "rgw", c.store.Name)
	labels["rook_object_store"] = c.store.Name
	return labels
}
