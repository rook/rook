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
	cephv1 "github.com/rook/rook/pkg/apis/ceph.rook.io/v1"
	"github.com/rook/rook/pkg/operator/ceph/cluster/mon"
	cephconfig "github.com/rook/rook/pkg/operator/ceph/config"
	"github.com/rook/rook/pkg/operator/ceph/controller"
	"github.com/rook/rook/pkg/operator/k8sutil"
	apps "k8s.io/api/apps/v1"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

const (
	livenessProbePath = "/swift/healthcheck"
)

func (c *clusterConfig) createDeployment(rgwConfig *rgwConfig) *apps.Deployment {
	replicas := int32(1)
	d := &apps.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      rgwConfig.ResourceName,
			Namespace: c.store.Namespace,
			Labels:    getLabels(c.store.Name, c.store.Namespace),
		},
		Spec: apps.DeploymentSpec{
			Selector: &metav1.LabelSelector{
				MatchLabels: getLabels(c.store.Name, c.store.Namespace),
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
	controller.AddCephVersionLabelToDeployment(c.clusterInfo.CephVersion, d)

	return d
}

func (c *clusterConfig) makeRGWPodSpec(rgwConfig *rgwConfig) v1.PodTemplateSpec {
	// Supplying ceph to FsGroup in SecurityContext of the pod
	cephgid := int64(167)
	podSpec := v1.PodSpec{
		InitContainers: []v1.Container{
			c.makeChownInitContainer(rgwConfig),
		},
		Containers: []v1.Container{
			c.makeDaemonContainer(rgwConfig),
		},
		RestartPolicy: v1.RestartPolicyAlways,
		Volumes: append(
			controller.DaemonVolumes(c.DataPathMap, rgwConfig.ResourceName),
			c.mimeTypesVolume(),
		),
		HostNetwork:       c.clusterSpec.Network.IsHost(),
		PriorityClassName: c.store.Spec.Gateway.PriorityClassName,
		SecurityContext: &v1.PodSecurityContext{
			FSGroup: &cephgid,
		},
	}
	// Replace default unreachable node toleration
	k8sutil.AddUnreachableNodeToleration(&podSpec)

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
	preferredDuringScheduling := false
	k8sutil.SetNodeAntiAffinityForPod(&podSpec, c.store.Spec.Gateway.Placement, c.clusterSpec.Network.IsHost(), preferredDuringScheduling, getLabels(c.store.Name, c.store.Namespace),
		nil)

	podTemplateSpec := v1.PodTemplateSpec{
		ObjectMeta: metav1.ObjectMeta{
			Name:   rgwConfig.ResourceName,
			Labels: getLabels(c.store.Name, c.store.Namespace),
		},
		Spec: podSpec,
	}
	c.store.Spec.Gateway.Annotations.ApplyToObjectMeta(&podTemplateSpec.ObjectMeta)

	if c.clusterSpec.Network.IsHost() {
		podTemplateSpec.Spec.DNSPolicy = v1.DNSClusterFirstWithHostNet
	} else if c.clusterSpec.Network.IsMultus() {
		k8sutil.ApplyMultus(c.Network.NetworkSpec, &podTemplateSpec.ObjectMeta)
	}

	return podTemplateSpec
}

func (c *clusterConfig) makeChownInitContainer(rgwConfig *rgwConfig) v1.Container {
	return controller.ChownCephDataDirsInitContainer(
		*c.DataPathMap,
		c.clusterSpec.CephVersion.Image,
		controller.DaemonVolumeMounts(c.DataPathMap, rgwConfig.ResourceName),
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
				controller.DaemonFlags(c.clusterInfo, strings.TrimPrefix(generateCephXUser(rgwConfig.ResourceName), "client.")),
				"--foreground",
				cephconfig.NewFlag("rgw frontends", fmt.Sprintf("%s %s", rgwFrontendName, c.portString())),
				cephconfig.NewFlag("host", controller.ContainerEnvVarReference("POD_NAME")),
				cephconfig.NewFlag("rgw-mime-types-file", mimeTypesMountPath()),
			),
		),
		VolumeMounts: append(
			controller.DaemonVolumeMounts(c.DataPathMap, rgwConfig.ResourceName),
			c.mimeTypesVolumeMount(),
		),
		Env:             controller.DaemonEnvVars(c.clusterSpec.CephVersion.Image),
		Resources:       c.store.Spec.Gateway.Resources,
		LivenessProbe:   c.generateLiveProbe(),
		SecurityContext: mon.PodSecurityContext(),
	}

	if c.store.Spec.Gateway.SSLCertificateRef != "" {
		// Add a volume mount for the ssl certificate
		mount := v1.VolumeMount{Name: certVolumeName, MountPath: certDir, ReadOnly: true}
		container.VolumeMounts = append(container.VolumeMounts, mount)
	}

	return container
}

func (c *clusterConfig) generateLiveProbe() *v1.Probe {
	return &v1.Probe{
		Handler: v1.Handler{
			HTTPGet: &v1.HTTPGetAction{
				Path:   livenessProbePath,
				Port:   c.generateLiveProbePort(),
				Scheme: c.generateLiveProbeScheme(),
			},
		},
		InitialDelaySeconds: 10,
	}
}

func (c *clusterConfig) generateLiveProbeScheme() v1.URIScheme {
	// Default to HTTP
	uriScheme := v1.URISchemeHTTP

	// If rgw is configured to use a secured port we need get on https://
	// Only do this when the Non-SSL port is not used
	if c.store.Spec.Gateway.Port == 0 && c.store.Spec.Gateway.SecurePort != 0 && c.store.Spec.Gateway.SSLCertificateRef != "" {
		uriScheme = v1.URISchemeHTTPS
	}

	return uriScheme
}

func (c *clusterConfig) generateLiveProbePort() intstr.IntOrString {
	// The port the liveness probe needs to probe
	// Assume we run on SDN by default
	port := intstr.FromInt(int(rgwPortInternalPort))

	// If Host Networking is enabled, the port from the spec must be reflected
	if c.clusterSpec.Network.IsHost() {
		if c.store.Spec.Gateway.Port == 0 && c.store.Spec.Gateway.SecurePort != 0 && c.store.Spec.Gateway.SSLCertificateRef != "" {
			port = intstr.FromInt(int(c.store.Spec.Gateway.SecurePort))
		} else {
			port = intstr.FromInt(int(c.store.Spec.Gateway.Port))
		}
	}

	return port
}

func (c *clusterConfig) generateService(cephObjectStore *cephv1.CephObjectStore) *v1.Service {
	labels := getLabels(cephObjectStore.Name, cephObjectStore.Namespace)
	svc := &v1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      instanceName(cephObjectStore.Name),
			Namespace: cephObjectStore.Namespace,
			Labels:    labels,
		},
		Spec: v1.ServiceSpec{
			Selector: labels,
		},
	}

	if c.clusterSpec.Network.IsHost() {
		svc.Spec.ClusterIP = v1.ClusterIPNone
	}

	destPort := c.generateLiveProbePort()
	addPort(svc, "http", cephObjectStore.Spec.Gateway.Port, destPort.IntVal)
	addPort(svc, "https", cephObjectStore.Spec.Gateway.SecurePort, cephObjectStore.Spec.Gateway.SecurePort)

	return svc
}

func (c *clusterConfig) reconcileService(cephObjectStore *cephv1.CephObjectStore) (string, error) {
	service := c.generateService(cephObjectStore)
	// Set owner ref to the parent object
	err := controllerutil.SetControllerReference(cephObjectStore, service, c.scheme)
	if err != nil {
		return "", errors.Wrapf(err, "failed to set owner reference to ceph object store")
	}

	svc, err := k8sutil.CreateOrUpdateService(c.context.Clientset, cephObjectStore.Namespace, service)
	if err != nil {
		return "", errors.Wrapf(err, "failed to create or update object store %q service", cephObjectStore.Name)
	}

	logger.Infof("ceph object store gateway service running at %s:%d", svc.Spec.ClusterIP, cephObjectStore.Spec.Gateway.Port)
	return svc.Spec.ClusterIP, nil
}

func addPort(service *v1.Service, name string, port, destPort int32) {
	if port == 0 || destPort == 0 {
		return
	}
	service.Spec.Ports = append(service.Spec.Ports, v1.ServicePort{
		Name:       name,
		Port:       port,
		TargetPort: intstr.FromInt(int(destPort)),
		Protocol:   v1.ProtocolTCP,
	})
}

func getLabels(name, namespace string) map[string]string {
	labels := controller.PodLabels(AppName, namespace, "rgw", name)
	labels["rook_object_store"] = name
	return labels
}
