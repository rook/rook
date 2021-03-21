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
	"path"
	"strings"

	"github.com/hashicorp/vault/api"
	"github.com/libopenstorage/secrets/vault"
	"github.com/pkg/errors"
	cephv1 "github.com/rook/rook/pkg/apis/ceph.rook.io/v1"
	"github.com/rook/rook/pkg/daemon/ceph/osd/kms"
	cephconfig "github.com/rook/rook/pkg/operator/ceph/config"
	"github.com/rook/rook/pkg/operator/ceph/controller"
	"github.com/rook/rook/pkg/operator/k8sutil"
	apps "k8s.io/api/apps/v1"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
)

const (
	livenessProbePath = "/swift/healthcheck"
	// #nosec G101 since this is not leaking any hardcoded details
	setupVaultTokenFile = `
set -e

VAULT_TOKEN_OLD_PATH=%s
VAULT_TOKEN_NEW_PATH=%s

cp --verbose $VAULT_TOKEN_OLD_PATH $VAULT_TOKEN_NEW_PATH

chmod --verbose 400 $VAULT_TOKEN_NEW_PATH

chown --verbose ceph:ceph $VAULT_TOKEN_NEW_PATH
`
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
		Containers:    []v1.Container{c.makeDaemonContainer(rgwConfig)},
		RestartPolicy: v1.RestartPolicyAlways,
		Volumes: append(
			controller.DaemonVolumes(c.DataPathMap, rgwConfig.ResourceName),
			c.mimeTypesVolume(),
		),
		HostNetwork:       c.clusterSpec.Network.IsHost(),
		PriorityClassName: c.store.Spec.Gateway.PriorityClassName,
	}

	// If the log collector is enabled we add the side-car container
	if c.clusterSpec.LogCollector.Enabled {
		shareProcessNamespace := true
		podSpec.ShareProcessNamespace = &shareProcessNamespace
		podSpec.Containers = append(podSpec.Containers, *controller.LogCollectorContainer(strings.TrimPrefix(generateCephXUser(fmt.Sprintf("ceph-client.%s", rgwConfig.ResourceName)), "client."), c.clusterInfo.Namespace, *c.clusterSpec))
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
	if c.clusterSpec.Security.KeyManagementService.IsEnabled() {
		if c.clusterSpec.Security.KeyManagementService.IsTokenAuthEnabled() {
			podSpec.Volumes = append(podSpec.Volumes,
				kms.VaultTokenFileVolume(c.clusterSpec.Security.KeyManagementService.TokenSecretName))
			podSpec.InitContainers = append(podSpec.InitContainers,
				c.vaultTokenInitContainer(rgwConfig))
		}
	}
	c.store.Spec.Gateway.Placement.ApplyToPodSpec(&podSpec)

	// If host networking is not enabled, preferred pod anti-affinity is added to the rgw daemons
	labels := getLabels(c.store.Name, c.store.Namespace, false)
	k8sutil.SetNodeAntiAffinityForPod(&podSpec, c.clusterSpec.Network.IsHost(), v1.LabelHostname, labels, nil)

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

// The vault token is passed as Secret for rgw container. So it is mounted as read only.
// RGW has restrictions over vault token file, it should owned by same user(ceph) which
// rgw daemon runs and all other permission should be nil or zero. Here ownership can be
// changed with help of FSGroup but in openshift environments for security reasons it has
// predefined value, so it won't work there. Hence the token file is copied to containerDataDir
// from mounted secret then ownership/permissions are changed accordingly with help of a
// init container.
func (c *clusterConfig) vaultTokenInitContainer(rgwConfig *rgwConfig) v1.Container {
	_, volMount := kms.VaultVolumeAndMount(c.clusterSpec.Security.KeyManagementService.ConnectionDetails)
	return v1.Container{
		Name: "vault-initcontainer-token-file-setup",
		Command: []string{
			"/bin/bash",
			"-c",
			fmt.Sprintf(setupVaultTokenFile,
				path.Join(kms.EtcVaultDir, kms.VaultFileName), path.Join(c.DataPathMap.ContainerDataDir, kms.VaultFileName)),
		},
		Image: c.clusterSpec.CephVersion.Image,
		VolumeMounts: append(
			controller.DaemonVolumeMounts(c.DataPathMap, rgwConfig.ResourceName), volMount),
		Resources:       c.store.Spec.Gateway.Resources,
		SecurityContext: controller.PodSecurityContext(),
	}
}

func (c *clusterConfig) makeChownInitContainer(rgwConfig *rgwConfig) v1.Container {
	return controller.ChownCephDataDirsInitContainer(
		*c.DataPathMap,
		c.clusterSpec.CephVersion.Image,
		controller.DaemonVolumeMounts(c.DataPathMap, rgwConfig.ResourceName),
		c.store.Spec.Gateway.Resources,
		controller.PodSecurityContext(),
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
		SecurityContext: controller.PodSecurityContext(),
		WorkingDir:      cephconfig.VarLogCephDir,
	}

	// If the liveness probe is enabled
	configureLivenessProbe(&container, c.store.Spec.HealthCheck)
	if c.store.Spec.Gateway.SSLCertificateRef != "" {
		// Add a volume mount for the ssl certificate
		mount := v1.VolumeMount{Name: certVolumeName, MountPath: certDir, ReadOnly: true}
		container.VolumeMounts = append(container.VolumeMounts, mount)
	}
	if c.clusterSpec.Security.KeyManagementService.IsEnabled() {
		container.Args = append(container.Args,
			cephconfig.NewFlag("rgw crypt s3 kms backend",
				c.clusterSpec.Security.KeyManagementService.ConnectionDetails[kms.Provider]),
			cephconfig.NewFlag("rgw crypt vault addr",
				c.clusterSpec.Security.KeyManagementService.ConnectionDetails[api.EnvVaultAddress]),
		)
		if c.clusterSpec.Security.KeyManagementService.IsTokenAuthEnabled() {
			container.Args = append(container.Args,
				cephconfig.NewFlag("rgw crypt vault auth", kms.KMSTokenSecretNameKey),
				cephconfig.NewFlag("rgw crypt vault token file",
					path.Join(c.DataPathMap.ContainerDataDir, kms.VaultFileName)),
				cephconfig.NewFlag("rgw crypt vault prefix", c.vaultPrefixRGW()),
				cephconfig.NewFlag("rgw crypt vault secret engine",
					c.clusterSpec.Security.KeyManagementService.ConnectionDetails[kms.VaultSecretEngineKey]),
			)
		}
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
				// Set the liveness probe on the container to overwrite the default probe created by Rook
				container.LivenessProbe = cephconfig.GetLivenessProbeWithDefaults(probe, container.LivenessProbe)
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
		port = intstr.FromInt(int(c.store.Spec.Gateway.Port))
	}

	if c.store.Spec.Gateway.Port == 0 && c.store.Spec.Gateway.SecurePort != 0 && c.store.Spec.Gateway.SSLCertificateRef != "" {
		port = intstr.FromInt(int(c.store.Spec.Gateway.SecurePort))
	}
	return port
}

func (c *clusterConfig) generateService(cephObjectStore *cephv1.CephObjectStore) *v1.Service {
	svc := &v1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      instanceName(cephObjectStore.Name),
			Namespace: cephObjectStore.Namespace,
			Labels:    getLabels(cephObjectStore.Name, cephObjectStore.Namespace, true),
		},
	}

	if c.clusterSpec.Network.IsHost() {
		svc.Spec.ClusterIP = v1.ClusterIPNone
	}

	destPort := c.generateLiveProbePort()

	// When the cluster is external we must use the same one as the gateways are listening on
	if cephObjectStore.Spec.IsExternal() {
		destPort.IntVal = cephObjectStore.Spec.Gateway.Port
	} else {
		// If the cluster is not external we add the Selector
		svc.Spec = v1.ServiceSpec{
			Selector: getLabels(cephObjectStore.Name, cephObjectStore.Namespace, false),
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
	err := c.ownerInfo.SetControllerReference(endpoint)
	if err != nil {
		return errors.Wrapf(err, "failed to set owner reference to ceph object store endpoint %q", endpoint.Name)
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
	err := c.ownerInfo.SetControllerReference(service)
	if err != nil {
		return "", errors.Wrapf(err, "failed to set owner reference to ceph object store service %q", service.Name)
	}

	svc, err := k8sutil.CreateOrUpdateService(c.context.Clientset, cephObjectStore.Namespace, service)
	if err != nil {
		return "", errors.Wrapf(err, "failed to create or update object store %q service", cephObjectStore.Name)
	}

	logger.Infof("ceph object store gateway service running at %s", svc.Spec.ClusterIP)

	return svc.Spec.ClusterIP, nil
}

func (c *clusterConfig) vaultPrefixRGW() string {
	secretEngine := c.clusterSpec.Security.KeyManagementService.ConnectionDetails[kms.VaultSecretEngineKey]
	vaultPrefixPath := "/v1/"

	switch secretEngine {
	case kms.VaultKVSecretEngineKey:
		vaultPrefixPath = path.Join(vaultPrefixPath,
			c.clusterSpec.Security.KeyManagementService.ConnectionDetails[vault.VaultBackendPathKey])
	case kms.VaultTransitSecretEngineKey:
		vaultPrefixPath = path.Join(vaultPrefixPath, secretEngine, "/export/encryption-key")
	}

	return vaultPrefixPath
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
