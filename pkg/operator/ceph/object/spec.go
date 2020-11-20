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
	cephutil "github.com/rook/rook/pkg/daemon/ceph/util"
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

func (c *clusterConfig) createDeployment(rgwConfig *rgwConfig) (*apps.Deployment, error) {
	pod, err := c.makeRGWPodSpec(rgwConfig)
	if err != nil {
		return nil, err
	}
	replicas := int32(1)
	d := &apps.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      rgwConfig.ResourceName,
			Namespace: c.store.Namespace,
			Labels:    getLabels(c.store.Name, c.store.Namespace, true),
		},
		Spec: apps.DeploymentSpec{
			Selector: &metav1.LabelSelector{
				MatchLabels: getLabels(c.store.Name, c.store.Namespace, false),
			},
			Template: pod,
			Replicas: &replicas,
			Strategy: apps.DeploymentStrategy{
				Type: apps.RecreateDeploymentStrategyType,
			},
		},
	}
	k8sutil.AddRookVersionLabelToDeployment(d)
	c.store.Spec.Gateway.Annotations.ApplyToObjectMeta(&d.ObjectMeta)
	c.store.Spec.Gateway.Labels.ApplyToObjectMeta(&d.ObjectMeta)
	controller.AddCephVersionLabelToDeployment(c.clusterInfo.CephVersion, d)

	return d, nil
}

func (c *clusterConfig) makeRGWPodSpec(rgwConfig *rgwConfig) (v1.PodTemplateSpec, error) {
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
		// use service account
		ServiceAccountName: cephutil.DefaultServiceAccount,
	}
	// Replace default unreachable node toleration
	k8sutil.AddUnreachableNodeToleration(&podSpec)

	// Set the ssl cert if specified
	if c.store.Spec.Gateway.SSLCertificateRef != "" {
		// Keep the SSL secret as secure as possible in the container. Give only user read perms.
		// Because the Secret mount is owned by "root" and fsGroup breaks on OCP since we cannot predict it
		// Also, we don't want to change the SCC for fsGroup to RunAsAny since it has a major broader impact
		// Let's open the permissions a bit more so that everyone can read the cert.
		userReadOnly := int32(0444)
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

	// If host networking is not enabled, preferred pod anti-affinity is added to the rgw daemons
	labels := getLabels(c.store.Name, c.store.Namespace, false)
	k8sutil.SetNodeAntiAffinityForPod(&podSpec, c.store.Spec.Gateway.Placement, c.clusterSpec.Network.IsHost(), labels, nil)

	podTemplateSpec := v1.PodTemplateSpec{
		ObjectMeta: metav1.ObjectMeta{
			Name:   rgwConfig.ResourceName,
			Labels: getLabels(c.store.Name, c.store.Namespace, true),
		},
		Spec: podSpec,
	}
	c.store.Spec.Gateway.Annotations.ApplyToObjectMeta(&podTemplateSpec.ObjectMeta)
	c.store.Spec.Gateway.Labels.ApplyToObjectMeta(&podTemplateSpec.ObjectMeta)

	if c.clusterSpec.Network.IsHost() {
		podTemplateSpec.Spec.DNSPolicy = v1.DNSClusterFirstWithHostNet
	} else if c.clusterSpec.Network.IsMultus() {
		if err := k8sutil.ApplyMultus(c.clusterSpec.Network.NetworkSpec, &podTemplateSpec.ObjectMeta); err != nil {
			return podTemplateSpec, err
		}
	}

	return podTemplateSpec, nil
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
			controller.DaemonFlags(c.clusterInfo, c.clusterSpec,
				strings.TrimPrefix(generateCephXUser(rgwConfig.ResourceName), "client.")),
			"--foreground",
			cephconfig.NewFlag("rgw frontends", fmt.Sprintf("%s %s", rgwFrontendName, c.portString())),
			cephconfig.NewFlag("host", controller.ContainerEnvVarReference(k8sutil.PodNameEnvVar)),
			cephconfig.NewFlag("rgw-mime-types-file", mimeTypesMountPath()),
			cephconfig.NewFlag("rgw realm", rgwConfig.Realm),
			cephconfig.NewFlag("rgw zonegroup", rgwConfig.ZoneGroup),
			cephconfig.NewFlag("rgw zone", rgwConfig.Zone),
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

	// If the liveness probe is enabled
	configureLivenessProbe(&container, c.store.Spec.HealthCheck)
	if c.store.Spec.Gateway.SSLCertificateRef != "" {
		// Add a volume mount for the ssl certificate
		mount := v1.VolumeMount{Name: certVolumeName, MountPath: certDir, ReadOnly: true}
		container.VolumeMounts = append(container.VolumeMounts, mount)
	}

	return container
}

// configureLivenessProbe returns the desired liveness probe for a given daemon
func configureLivenessProbe(container *v1.Container, healthCheck cephv1.BucketHealthCheckSpec) {
	if ok := healthCheck.LivenessProbe; ok != nil {
		if !healthCheck.LivenessProbe.Disabled {
			probe := healthCheck.LivenessProbe.Probe
			// If the spec value is empty, let's use a default
			if probe != nil {
				container.LivenessProbe = probe
			}
		} else {
			container.LivenessProbe = nil
		}
	}
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
	labels := getLabels(cephObjectStore.Name, cephObjectStore.Namespace, true)
	svc := &v1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      instanceName(cephObjectStore.Name),
			Namespace: cephObjectStore.Namespace,
			Labels:    labels,
		},
	}

	if c.clusterSpec.Network.IsHost() {
		svc.Spec.ClusterIP = v1.ClusterIPNone
	}

	destPort := c.generateLiveProbePort()

	// When the cluster is external we must use the same one as the gateways are listening on
	if c.clusterSpec.External.Enable {
		destPort.IntVal = cephObjectStore.Spec.Gateway.Port
	} else {
		// If the cluster is not external we add the Selector
		svc.Spec = v1.ServiceSpec{
			Selector: labels,
		}
	}
	addPort(svc, "http", cephObjectStore.Spec.Gateway.Port, destPort.IntVal)
	addPort(svc, "https", cephObjectStore.Spec.Gateway.SecurePort, cephObjectStore.Spec.Gateway.SecurePort)

	return svc
}

func (c *clusterConfig) generateEndpoint(cephObjectStore *cephv1.CephObjectStore) *v1.Endpoints {
	labels := getLabels(cephObjectStore.Name, cephObjectStore.Namespace, true)

	endpoints := &v1.Endpoints{
		ObjectMeta: metav1.ObjectMeta{
			Name:      instanceName(cephObjectStore.Name),
			Namespace: cephObjectStore.Namespace,
			Labels:    labels,
		},
		Subsets: []v1.EndpointSubset{
			{
				Addresses: cephObjectStore.Spec.Gateway.ExternalRgwEndpoints,
			},
		},
	}

	addPortToEndpoint(endpoints, "http", cephObjectStore.Spec.Gateway.Port)
	addPortToEndpoint(endpoints, "https", cephObjectStore.Spec.Gateway.SecurePort)

	return endpoints
}

func (c *clusterConfig) reconcileExternalEndpoint(cephObjectStore *cephv1.CephObjectStore) error {
	logger.Info("reconciling external object store service")

	endpoint := c.generateEndpoint(cephObjectStore)
	// Set owner ref to the parent object
	err := controllerutil.SetControllerReference(cephObjectStore, endpoint, c.scheme)
	if err != nil {
		return errors.Wrap(err, "failed to set owner reference to ceph object store endpoint")
	}

	_, err = k8sutil.CreateOrUpdateEndpoint(c.context.Clientset, cephObjectStore.Namespace, endpoint)
	if err != nil {
		return errors.Wrapf(err, "failed to create or update object store %q endpoint", cephObjectStore.Name)
	}

	return nil
}

func (c *clusterConfig) reconcileService(cephObjectStore *cephv1.CephObjectStore) (string, error) {
	service := c.generateService(cephObjectStore)
	// Set owner ref to the parent object
	err := controllerutil.SetControllerReference(cephObjectStore, service, c.scheme)
	if err != nil {
		return "", errors.Wrap(err, "failed to set owner reference to ceph object store service")
	}

	svc, err := k8sutil.CreateOrUpdateService(c.context.Clientset, cephObjectStore.Namespace, service)
	if err != nil {
		return "", errors.Wrapf(err, "failed to create or update object store %q service", cephObjectStore.Name)
	}

	logger.Infof("ceph object store gateway service running at %s", svc.Spec.ClusterIP)

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

func addPortToEndpoint(endpoints *v1.Endpoints, name string, port int32) {
	if port == 0 {
		return
	}
	endpoints.Subsets[0].Ports = append(endpoints.Subsets[0].Ports, v1.EndpointPort{
		Name:     name,
		Port:     port,
		Protocol: v1.ProtocolTCP,
	},
	)
}

func getLabels(name, namespace string, includeNewLabels bool) map[string]string {
	labels := controller.CephDaemonAppLabels(AppName, namespace, "rgw", name, includeNewLabels)
	labels["rook_object_store"] = name
	return labels
}
