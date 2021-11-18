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
	"reflect"
	"strings"

	"github.com/hashicorp/vault/api"
	"github.com/libopenstorage/secrets/vault"
	"github.com/pkg/errors"
	cephv1 "github.com/rook/rook/pkg/apis/ceph.rook.io/v1"
	"github.com/rook/rook/pkg/daemon/ceph/osd/kms"
	cephconfig "github.com/rook/rook/pkg/operator/ceph/config"
	"github.com/rook/rook/pkg/operator/ceph/controller"
	cephver "github.com/rook/rook/pkg/operator/ceph/version"
	"github.com/rook/rook/pkg/operator/k8sutil"
	apps "k8s.io/api/apps/v1"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
)

const (
	readinessProbePath = "/swift/healthcheck"
	// #nosec G101 since this is not leaking any hardcoded details
	setupVaultTokenFile = `
set -e

VAULT_TOKEN_OLD_PATH=%s
VAULT_TOKEN_NEW_PATH=%s

cp --recursive --verbose $VAULT_TOKEN_OLD_PATH/..data/. $VAULT_TOKEN_NEW_PATH

chmod --recursive --verbose 400 $VAULT_TOKEN_NEW_PATH/*
chmod --verbose 700 $VAULT_TOKEN_NEW_PATH
chown --recursive --verbose ceph:ceph $VAULT_TOKEN_NEW_PATH
`
)

func (c *clusterConfig) createDeployment(rgwConfig *rgwConfig) (*apps.Deployment, error) {
	pod, err := c.makeRGWPodSpec(rgwConfig)
	if err != nil {
		return nil, err
	}
	replicas := int32(1)
	// On Pacific, we can use the same keyring and have dedicated rgw instances reflected in the service map
	if c.clusterInfo.CephVersion.IsAtLeastPacific() {
		replicas = c.store.Spec.Gateway.Instances
	}
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
	rgwDaemonContainer := c.makeDaemonContainer(rgwConfig)
	if reflect.DeepEqual(rgwDaemonContainer, v1.Container{}) {
		return v1.PodTemplateSpec{}, errors.New("got empty container for RGW daemon")
	}
	podSpec := v1.PodSpec{
		InitContainers: []v1.Container{
			c.makeChownInitContainer(rgwConfig),
		},
		Containers:    []v1.Container{rgwDaemonContainer},
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
		podSpec.Containers = append(podSpec.Containers, *controller.LogCollectorContainer(getDaemonName(rgwConfig), c.clusterInfo.Namespace, *c.clusterSpec))
	}

	// Replace default unreachable node toleration
	k8sutil.AddUnreachableNodeToleration(&podSpec)

	// Set the ssl cert if specified
	if c.store.Spec.Gateway.SecurePort != 0 {
		secretVolSrc, err := c.generateVolumeSourceWithTLSSecret()
		if err != nil {
			return v1.PodTemplateSpec{}, err
		}
		certVol := v1.Volume{
			Name: certVolumeName,
			VolumeSource: v1.VolumeSource{
				Secret: secretVolSrc,
			}}
		podSpec.Volumes = append(podSpec.Volumes, certVol)
	}
	// Check custom caBundle provided
	if c.store.Spec.Gateway.CaBundleRef != "" {
		customCaBundleVolSrc, err := c.generateVolumeSourceWithCaBundleSecret()
		if err != nil {
			return v1.PodTemplateSpec{}, err
		}
		customCaBundleVol := v1.Volume{
			Name: caBundleVolumeName,
			VolumeSource: v1.VolumeSource{
				Secret: customCaBundleVolSrc,
			}}
		podSpec.Volumes = append(podSpec.Volumes, customCaBundleVol)
		updatedCaBundleVol := v1.Volume{
			Name: caBundleUpdatedVolumeName,
			VolumeSource: v1.VolumeSource{
				EmptyDir: &v1.EmptyDirVolumeSource{},
			}}
		podSpec.Volumes = append(podSpec.Volumes, updatedCaBundleVol)
		podSpec.InitContainers = append(podSpec.InitContainers,
			c.createCaBundleUpdateInitContainer(rgwConfig))
	}
	kmsEnabled, err := c.CheckRGWKMS()
	if err != nil {
		return v1.PodTemplateSpec{}, err
	}
	if kmsEnabled {
		if c.store.Spec.Security.KeyManagementService.IsTokenAuthEnabled() {
			vaultFileVol, _ := kms.VaultVolumeAndMount(c.store.Spec.Security.KeyManagementService.ConnectionDetails,
				c.store.Spec.Security.KeyManagementService.TokenSecretName)
			tmpvolume := v1.Volume{
				Name: rgwVaultVolumeName,
				VolumeSource: v1.VolumeSource{
					EmptyDir: &v1.EmptyDirVolumeSource{},
				},
			}

			podSpec.Volumes = append(podSpec.Volumes, vaultFileVol, tmpvolume)
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
		if err := k8sutil.ApplyMultus(c.clusterSpec.Network, &podTemplateSpec.ObjectMeta); err != nil {
			return podTemplateSpec, err
		}
	}

	return podTemplateSpec, nil
}

func (c *clusterConfig) createCaBundleUpdateInitContainer(rgwConfig *rgwConfig) v1.Container {
	caBundleMount := v1.VolumeMount{Name: caBundleVolumeName, MountPath: caBundleSourceCustomDir, ReadOnly: true}
	volumeMounts := append(controller.DaemonVolumeMounts(c.DataPathMap, rgwConfig.ResourceName), caBundleMount)
	updatedCaBundleDir := "/tmp/new-ca-bundle/"
	updatedBundleMount := v1.VolumeMount{Name: caBundleUpdatedVolumeName, MountPath: updatedCaBundleDir, ReadOnly: false}
	volumeMounts = append(volumeMounts, updatedBundleMount)
	return v1.Container{
		Name:    "update-ca-bundle-initcontainer",
		Command: []string{"/bin/bash", "-c"},
		// copy all content of caBundleExtractedDir to avoid directory mount itself
		Args: []string{
			fmt.Sprintf("/usr/bin/update-ca-trust extract; cp -rf %s/* %s", caBundleExtractedDir, updatedCaBundleDir),
		},
		Image:           c.clusterSpec.CephVersion.Image,
		VolumeMounts:    volumeMounts,
		Resources:       c.store.Spec.Gateway.Resources,
		SecurityContext: controller.PodSecurityContext(),
	}
}

// The vault token is passed as Secret for rgw container. So it is mounted as read only.
// RGW has restrictions over vault token file, it should owned by same user (ceph) which
// rgw daemon runs and all other permission should be nil or zero. Here ownership can be
// changed with help of FSGroup but in openshift environments for security reasons it has
// predefined value, so it won't work there. Hence the token file and certs (if present)
// are copied to other volume from mounted secrets then ownership/permissions are changed
// accordingly with help of an init container.
func (c *clusterConfig) vaultTokenInitContainer(rgwConfig *rgwConfig) v1.Container {
	_, srcVaultVolMount := kms.VaultVolumeAndMount(c.store.Spec.Security.KeyManagementService.ConnectionDetails, "")
	tmpVaultMount := v1.VolumeMount{Name: rgwVaultVolumeName, MountPath: rgwVaultDirName}
	return v1.Container{
		Name: "vault-initcontainer-token-file-setup",
		Command: []string{
			"/bin/bash",
			"-c",
			fmt.Sprintf(setupVaultTokenFile,
				kms.EtcVaultDir, rgwVaultDirName),
		},
		Image: c.clusterSpec.CephVersion.Image,
		VolumeMounts: append(
			controller.DaemonVolumeMounts(c.DataPathMap, rgwConfig.ResourceName), srcVaultVolMount, tmpVaultMount),
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
		LivenessProbe:   c.defaultLivenessProbe(),
		ReadinessProbe:  c.defaultReadinessProbe(),
		SecurityContext: controller.PodSecurityContext(),
		WorkingDir:      cephconfig.VarLogCephDir,
	}

	// If the liveness probe is enabled
	configureLivenessProbe(&container, c.store.Spec.HealthCheck)
	// If the readiness probe is enabled
	configureReadinessProbe(&container, c.store.Spec.HealthCheck)
	if c.store.Spec.IsTLSEnabled() {
		// Add a volume mount for the ssl certificate
		mount := v1.VolumeMount{Name: certVolumeName, MountPath: certDir, ReadOnly: true}
		container.VolumeMounts = append(container.VolumeMounts, mount)
	}
	if c.store.Spec.Gateway.CaBundleRef != "" {
		updatedBundleMount := v1.VolumeMount{Name: caBundleUpdatedVolumeName, MountPath: caBundleExtractedDir, ReadOnly: true}
		container.VolumeMounts = append(container.VolumeMounts, updatedBundleMount)
	}
	kmsEnabled, err := c.CheckRGWKMS()
	if err != nil {
		logger.Errorf("failed to enable KMS. %v", err)
		return v1.Container{}
	}
	if kmsEnabled {
		container.Args = append(container.Args,
			cephconfig.NewFlag("rgw crypt s3 kms backend",
				c.store.Spec.Security.KeyManagementService.ConnectionDetails[kms.Provider]),
			cephconfig.NewFlag("rgw crypt vault addr",
				c.store.Spec.Security.KeyManagementService.ConnectionDetails[api.EnvVaultAddress]),
		)
		if c.store.Spec.Security.KeyManagementService.IsTokenAuthEnabled() {
			container.Args = append(container.Args,
				cephconfig.NewFlag("rgw crypt vault auth", kms.KMSTokenSecretNameKey),
				cephconfig.NewFlag("rgw crypt vault token file",
					path.Join(rgwVaultDirName, kms.VaultFileName)),
				cephconfig.NewFlag("rgw crypt vault prefix", c.vaultPrefixRGW()),
				cephconfig.NewFlag("rgw crypt vault secret engine",
					c.store.Spec.Security.KeyManagementService.ConnectionDetails[kms.VaultSecretEngineKey]),
			)
		}
		if c.store.Spec.Security.KeyManagementService.IsTLSEnabled() &&
			c.clusterInfo.CephVersion.IsAtLeast(cephver.CephVersion{Major: 16, Minor: 2, Extra: 6}) {
			container.Args = append(container.Args,
				cephconfig.NewFlag("rgw crypt vault verify ssl", "true"))
			if kms.GetParam(c.store.Spec.Security.KeyManagementService.ConnectionDetails, api.EnvVaultClientCert) != "" {
				container.Args = append(container.Args,
					cephconfig.NewFlag("rgw crypt vault ssl clientcert", path.Join(rgwVaultDirName, kms.VaultCertFileName)))
			}
			if kms.GetParam(c.store.Spec.Security.KeyManagementService.ConnectionDetails, api.EnvVaultClientKey) != "" {
				container.Args = append(container.Args,
					cephconfig.NewFlag("rgw crypt vault ssl clientkey", path.Join(rgwVaultDirName, kms.VaultKeyFileName)))
			}
			if kms.GetParam(c.store.Spec.Security.KeyManagementService.ConnectionDetails, api.EnvVaultCACert) != "" {
				container.Args = append(container.Args,
					cephconfig.NewFlag("rgw crypt vault ssl cacert", path.Join(rgwVaultDirName, kms.VaultCAFileName)))
			}
		}
		vaultVolMount := v1.VolumeMount{Name: rgwVaultVolumeName, MountPath: rgwVaultDirName}
		container.VolumeMounts = append(container.VolumeMounts, vaultVolMount)
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
				container.LivenessProbe = cephconfig.GetProbeWithDefaults(probe, container.LivenessProbe)
			}
		} else {
			container.LivenessProbe = nil
		}
	}
}

// configureReadinessProbe returns the desired readiness probe for a given daemon
func configureReadinessProbe(container *v1.Container, healthCheck cephv1.BucketHealthCheckSpec) {
	if ok := healthCheck.ReadinessProbe; ok != nil {
		if !healthCheck.ReadinessProbe.Disabled {
			probe := healthCheck.ReadinessProbe.Probe
			// If the spec value is empty, let's use a default
			if probe != nil {
				// Set the readiness probe on the container to overwrite the default probe created by Rook
				container.ReadinessProbe = cephconfig.GetProbeWithDefaults(probe, container.ReadinessProbe)
			}
		} else {
			container.ReadinessProbe = nil
		}
	}
}

func (c *clusterConfig) defaultLivenessProbe() *v1.Probe {
	return &v1.Probe{
		Handler: v1.Handler{
			TCPSocket: &v1.TCPSocketAction{
				Port: c.generateProbePort(),
			},
		},
		InitialDelaySeconds: 10,
	}
}

func (c *clusterConfig) defaultReadinessProbe() *v1.Probe {
	return &v1.Probe{
		Handler: v1.Handler{
			HTTPGet: &v1.HTTPGetAction{
				Path:   readinessProbePath,
				Port:   c.generateProbePort(),
				Scheme: c.generateReadinessProbeScheme(),
			},
		},
		InitialDelaySeconds: 10,
	}
}

func (c *clusterConfig) generateReadinessProbeScheme() v1.URIScheme {
	// Default to HTTP
	uriScheme := v1.URISchemeHTTP

	// If rgw is configured to use a secured port we need get on https://
	// Only do this when the Non-SSL port is not used
	if c.store.Spec.Gateway.Port == 0 && c.store.Spec.IsTLSEnabled() {
		uriScheme = v1.URISchemeHTTPS
	}

	return uriScheme
}

func (c *clusterConfig) generateProbePort() intstr.IntOrString {
	// The port the liveness probe needs to probe
	// Assume we run on SDN by default
	port := intstr.FromInt(int(rgwPortInternalPort))

	// If Host Networking is enabled, the port from the spec must be reflected
	if c.clusterSpec.Network.IsHost() {
		port = intstr.FromInt(int(c.store.Spec.Gateway.Port))
	}

	if c.store.Spec.Gateway.Port == 0 && c.store.Spec.IsTLSEnabled() {
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

	if c.store.Spec.Gateway.Service != nil {
		c.store.Spec.Gateway.Service.Annotations.ApplyToObjectMeta(&svc.ObjectMeta)
	}
	if c.clusterSpec.Network.IsHost() {
		svc.Spec.ClusterIP = v1.ClusterIPNone
	}

	destPort := c.generateProbePort()

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
	secretEngine := c.store.Spec.Security.KeyManagementService.ConnectionDetails[kms.VaultSecretEngineKey]
	vaultPrefixPath := "/v1/"

	switch secretEngine {
	case kms.VaultKVSecretEngineKey:
		vaultPrefixPath = path.Join(vaultPrefixPath,
			c.store.Spec.Security.KeyManagementService.ConnectionDetails[vault.VaultBackendPathKey], "/data")
	case kms.VaultTransitSecretEngineKey:
		if c.clusterInfo.CephVersion.IsAtLeastPacific() {
			vaultPrefixPath = path.Join(vaultPrefixPath, secretEngine, "/transit")
		} else {
			vaultPrefixPath = path.Join(vaultPrefixPath, secretEngine, "/export/encryption-key")
		}
	}

	return vaultPrefixPath
}

func (c *clusterConfig) CheckRGWKMS() (bool, error) {
	if c.store.Spec.Security != nil && c.store.Spec.Security.KeyManagementService.IsEnabled() {
		err := kms.ValidateConnectionDetails(c.context, c.store.Spec.Security, c.store.Namespace)
		if err != nil {
			return false, err
		}
		secretEngine := c.store.Spec.Security.KeyManagementService.ConnectionDetails[kms.VaultSecretEngineKey]

		// currently RGW supports kv(version 2) and transit secret engines in vault
		switch secretEngine {
		case kms.VaultKVSecretEngineKey:
			kvVers := c.store.Spec.Security.KeyManagementService.ConnectionDetails[vault.VaultBackendKey]
			if kvVers != "" {
				if kvVers != "v2" {
					return false, errors.New("failed to validate vault kv version, only v2 is supported")
				}
			} else {
				// If VAUL_BACKEND is not specified let's assume it's v2
				logger.Warningf("%s is not set, assuming the only supported version 2", vault.VaultBackendKey)
				c.store.Spec.Security.KeyManagementService.ConnectionDetails[vault.VaultBackendKey] = "v2"
			}
			return true, nil
		case kms.VaultTransitSecretEngineKey:
			return true, nil
		default:
			return false, errors.New("failed to validate vault secret engine")

		}
	}

	return false, nil
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

func (c *clusterConfig) generateVolumeSourceWithTLSSecret() (*v1.SecretVolumeSource, error) {
	// Keep the TLS secret as secure as possible in the container. Give only user read perms.
	// Because the Secret mount is owned by "root" and fsGroup breaks on OCP since we cannot predict it
	// Also, we don't want to change the SCC for fsGroup to RunAsAny since it has a major broader impact
	// Let's open the permissions a bit more so that everyone can read the cert.
	userReadOnly := int32(0444)
	var secretVolSrc *v1.SecretVolumeSource
	if c.store.Spec.Gateway.SSLCertificateRef != "" {
		secretVolSrc = &v1.SecretVolumeSource{
			SecretName: c.store.Spec.Gateway.SSLCertificateRef,
		}
		secretType, err := c.rgwTLSSecretType(c.store.Spec.Gateway.SSLCertificateRef)
		if err != nil {
			return nil, err
		}
		switch secretType {
		case v1.SecretTypeOpaque:
			secretVolSrc.Items = []v1.KeyToPath{
				{Key: certKeyName, Path: certFilename, Mode: &userReadOnly},
			}
		case v1.SecretTypeTLS:
			secretVolSrc.Items = []v1.KeyToPath{
				{Key: v1.TLSCertKey, Path: certFilename, Mode: &userReadOnly},
				{Key: v1.TLSPrivateKeyKey, Path: certKeyFileName, Mode: &userReadOnly},
			}
		}
	} else if c.store.Spec.GetServiceServingCert() != "" {
		secretVolSrc = &v1.SecretVolumeSource{
			SecretName: c.store.Spec.GetServiceServingCert(),
			Items: []v1.KeyToPath{
				{Key: v1.TLSCertKey, Path: certFilename, Mode: &userReadOnly},
				{Key: v1.TLSPrivateKeyKey, Path: certKeyFileName, Mode: &userReadOnly},
			}}
	} else {
		return nil, errors.New("no TLS certificates found")
	}

	return secretVolSrc, nil
}

func (c *clusterConfig) generateVolumeSourceWithCaBundleSecret() (*v1.SecretVolumeSource, error) {
	// Keep the ca-bundle as secure as possible in the container. Give only user read perms.
	// Same as above for generateVolumeSourceWithTLSSecret function.
	userReadOnly := int32(0400)
	caBundleVolSrc := &v1.SecretVolumeSource{
		SecretName: c.store.Spec.Gateway.CaBundleRef,
	}
	secretType, err := c.rgwTLSSecretType(c.store.Spec.Gateway.CaBundleRef)
	if err != nil {
		return nil, err
	}
	if secretType != v1.SecretTypeOpaque {
		return nil, errors.New("CaBundle secret should be 'Opaque' type")
	}
	caBundleVolSrc.Items = []v1.KeyToPath{
		{Key: caBundleKeyName, Path: caBundleFileName, Mode: &userReadOnly},
	}
	return caBundleVolSrc, nil
}

func (c *clusterConfig) rgwTLSSecretType(secretName string) (v1.SecretType, error) {
	rgwTlsSecret, err := c.context.Clientset.CoreV1().Secrets(c.clusterInfo.Namespace).Get(c.clusterInfo.Context, secretName, metav1.GetOptions{})
	if rgwTlsSecret != nil {
		return rgwTlsSecret.Type, nil
	}
	return "", errors.Wrapf(err, "no Kubernetes secrets referring TLS certificates found")
}

func getDaemonName(rgwConfig *rgwConfig) string {
	return fmt.Sprintf("ceph-%s", generateCephXUser(rgwConfig.ResourceName))
}
