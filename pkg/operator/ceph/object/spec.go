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
	"bytes"
	_ "embed"
	"fmt"
	"net/url"
	"path"
	"slices"
	"strings"
	"text/template"

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
	"k8s.io/apimachinery/pkg/util/validation"
)

const (
	serviceAccountName = "rook-ceph-rgw"
	sseKMS             = "ssekms"
	sseS3              = "sses3"
	vaultPrefix        = "/v1/"
	//nolint:gosec // since this is not leaking any hardcoded details
	setupVaultTokenFile = `
set -e

VAULT_TOKEN_OLD_PATH=%s
VAULT_TOKEN_NEW_PATH=%s
if [ -d $VAULT_TOKEN_OLD_PATH/ssekms ]
then
cp --recursive --verbose $VAULT_TOKEN_OLD_PATH/ssekms/..data/. $VAULT_TOKEN_NEW_PATH/ssekms
chmod --recursive --verbose 400 $VAULT_TOKEN_NEW_PATH/ssekms/*
chmod --verbose 700 $VAULT_TOKEN_NEW_PATH/ssekms
fi
if [ -d $VAULT_TOKEN_OLD_PATH/sses3 ]
then
cp --recursive --verbose $VAULT_TOKEN_OLD_PATH/sses3/..data/. $VAULT_TOKEN_NEW_PATH/sses3
chmod --recursive --verbose 400 $VAULT_TOKEN_NEW_PATH/sses3/*
chmod --verbose 700 $VAULT_TOKEN_NEW_PATH/sses3
fi
chmod --verbose 700 $VAULT_TOKEN_NEW_PATH
chown --recursive --verbose ceph:ceph $VAULT_TOKEN_NEW_PATH
`
)

var (
	//go:embed rgw-probe.sh
	rgwProbeScriptTemplate string

	rgwAPIwithoutS3 = []string{"s3website", "swift", "swift_auth", "admin", "sts", "iam", "notifications"}
)

type ProbeType string
type ProtocolType string

const (
	StartupProbeType   ProbeType = "startup"
	ReadinessProbeType ProbeType = "readiness"

	HTTPProtocol  ProtocolType = "HTTP"
	HTTPSProtocol ProtocolType = "HTTPS"
)

type rgwProbeConfig struct {
	ProbeType ProbeType

	Protocol ProtocolType
	Port     string
	Path     string
}

func (c *clusterConfig) createDeployment(rgwConfig *rgwConfig) (*apps.Deployment, error) {
	pod, err := c.makeRGWPodSpec(rgwConfig)
	if err != nil {
		return nil, err
	}
	strategy := apps.DeploymentStrategy{
		Type: apps.RecreateDeploymentStrategyType,
	}
	// Use the same keyring and have dedicated rgw instances reflected in the service map
	replicas := c.store.Spec.Gateway.Instances

	strategy.Type = apps.RollingUpdateDeploymentStrategyType
	strategy.RollingUpdate = &apps.RollingUpdateDeployment{
		MaxUnavailable: &intstr.IntOrString{IntVal: int32(1)},
		MaxSurge:       &intstr.IntOrString{IntVal: int32(0)},
	}
	d := &apps.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      rgwConfig.ResourceName,
			Namespace: c.store.Namespace,
			Labels:    getLabels(c.store.Name, c.store.Namespace, true),
		},
		Spec: apps.DeploymentSpec{
			RevisionHistoryLimit: controller.RevisionHistoryLimit(),
			Selector: &metav1.LabelSelector{
				MatchLabels: getLabels(c.store.Name, c.store.Namespace, false),
			},
			Template: pod,
			Replicas: &replicas,
			Strategy: strategy,
		},
	}
	k8sutil.AddRookVersionLabelToDeployment(d)
	c.store.Spec.Gateway.Annotations.ApplyToObjectMeta(&d.ObjectMeta)
	c.store.Spec.Gateway.Labels.ApplyToObjectMeta(&d.ObjectMeta)
	controller.AddCephVersionLabelToDeployment(c.clusterInfo.CephVersion, d)

	return d, nil
}

func (c *clusterConfig) makeRGWPodSpec(rgwConfig *rgwConfig) (v1.PodTemplateSpec, error) {
	rgwDaemonContainer, err := c.makeDaemonContainer(rgwConfig)
	if err != nil {
		return v1.PodTemplateSpec{}, err
	}

	hostNetwork := c.store.Spec.IsHostNetwork(c.clusterSpec)
	podSpec := v1.PodSpec{
		InitContainers: []v1.Container{
			c.makeChownInitContainer(rgwConfig),
		},
		Containers:    []v1.Container{rgwDaemonContainer},
		RestartPolicy: v1.RestartPolicyAlways,
		Volumes: append(
			controller.DaemonVolumes(c.DataPathMap, rgwConfig.ResourceName, c.clusterSpec.DataDirHostPath),
			c.mimeTypesVolume(),
		),
		HostNetwork:        hostNetwork,
		PriorityClassName:  c.store.Spec.Gateway.PriorityClassName,
		SecurityContext:    &v1.PodSecurityContext{},
		ServiceAccountName: serviceAccountName,
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
	s3Enabled, err := c.CheckRGWSSES3Enabled()
	if err != nil {
		return v1.PodTemplateSpec{}, err
	}
	if kmsEnabled || s3Enabled {
		v := v1.Volume{
			Name: rgwVaultVolumeName,
			VolumeSource: v1.VolumeSource{
				EmptyDir: &v1.EmptyDirVolumeSource{},
			},
		}
		podSpec.Volumes = append(podSpec.Volumes, v)

		if kmsEnabled && c.store.Spec.Security.KeyManagementService.IsTokenAuthEnabled() {
			vaultFileVol, _ := kms.VaultVolumeAndMountWithCustomName(c.store.Spec.Security.KeyManagementService.ConnectionDetails,
				c.store.Spec.Security.KeyManagementService.TokenSecretName, sseKMS)
			podSpec.Volumes = append(podSpec.Volumes, vaultFileVol)
		}
		if s3Enabled && c.store.Spec.Security.ServerSideEncryptionS3.IsTokenAuthEnabled() {
			vaultFileVol, _ := kms.VaultVolumeAndMountWithCustomName(c.store.Spec.Security.ServerSideEncryptionS3.ConnectionDetails,
				c.store.Spec.Security.ServerSideEncryptionS3.TokenSecretName, sseS3)
			podSpec.Volumes = append(podSpec.Volumes, vaultFileVol)
		}

		podSpec.InitContainers = append(podSpec.InitContainers,
			c.vaultTokenInitContainer(rgwConfig, kmsEnabled, s3Enabled))
	}
	c.store.Spec.Gateway.Placement.ApplyToPodSpec(&podSpec)

	// If host networking is not enabled, preferred pod anti-affinity is added to the rgw daemons
	labels := getLabels(c.store.Name, c.store.Namespace, false)
	k8sutil.SetNodeAntiAffinityForPod(&podSpec, c.store.Spec.IsHostNetwork(c.clusterSpec), v1.LabelHostname, labels, nil)

	podTemplateSpec := v1.PodTemplateSpec{
		ObjectMeta: metav1.ObjectMeta{
			Name:   rgwConfig.ResourceName,
			Labels: getLabels(c.store.Name, c.store.Namespace, true),
		},
		Spec: podSpec,
	}
	c.store.Spec.Gateway.Annotations.ApplyToObjectMeta(&podTemplateSpec.ObjectMeta)
	c.store.Spec.Gateway.Labels.ApplyToObjectMeta(&podTemplateSpec.ObjectMeta)

	if hostNetwork {
		podTemplateSpec.Spec.DNSPolicy = v1.DNSClusterFirstWithHostNet
	} else if c.clusterSpec.Network.IsMultus() {
		if err := k8sutil.ApplyMultus(c.clusterInfo.Namespace, &c.clusterSpec.Network, &podTemplateSpec.ObjectMeta); err != nil {
			return podTemplateSpec, err
		}
	}

	addVols, addMounts := c.store.Spec.Gateway.AdditionalVolumeMounts.GenerateVolumesAndMounts("/var/rgw/")
	podTemplateSpec.Spec.Volumes = append(podTemplateSpec.Spec.Volumes, addVols...)
	podTemplateSpec.Spec.Containers[0].VolumeMounts = append(podTemplateSpec.Spec.Containers[0].VolumeMounts, addMounts...)

	return podTemplateSpec, nil
}

func (c *clusterConfig) createCaBundleUpdateInitContainer(rgwConfig *rgwConfig) v1.Container {
	caBundleMount := v1.VolumeMount{Name: caBundleVolumeName, MountPath: caBundleSourceCustomDir, ReadOnly: true}
	volumeMounts := append(controller.DaemonVolumeMounts(c.DataPathMap, rgwConfig.ResourceName, c.clusterSpec.DataDirHostPath), caBundleMount)
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
		ImagePullPolicy: controller.GetContainerImagePullPolicy(c.clusterSpec.CephVersion.ImagePullPolicy),
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
func (c *clusterConfig) vaultTokenInitContainer(rgwConfig *rgwConfig, kmsEnabled, s3Enabled bool) v1.Container {
	var vaultVolMounts []v1.VolumeMount

	tmpVaultMount := v1.VolumeMount{Name: rgwVaultVolumeName, MountPath: rgwVaultDirName}
	vaultVolMounts = append(vaultVolMounts, tmpVaultMount)
	if kmsEnabled {
		_, ssekmsVaultVolMount := kms.VaultVolumeAndMountWithCustomName(c.store.Spec.Security.KeyManagementService.ConnectionDetails, "", sseKMS)
		vaultVolMounts = append(vaultVolMounts, ssekmsVaultVolMount)
	}
	if s3Enabled {
		_, sses3VaultVolMount := kms.VaultVolumeAndMountWithCustomName(c.store.Spec.Security.ServerSideEncryptionS3.ConnectionDetails, "", sseS3)
		vaultVolMounts = append(vaultVolMounts, sses3VaultVolMount)
	}
	return v1.Container{
		Name: "vault-initcontainer-token-file-setup",
		Command: []string{
			"/bin/bash",
			"-c",
			fmt.Sprintf(setupVaultTokenFile,
				kms.EtcVaultDir, rgwVaultDirName),
		},
		Image:           c.clusterSpec.CephVersion.Image,
		ImagePullPolicy: controller.GetContainerImagePullPolicy(c.clusterSpec.CephVersion.ImagePullPolicy),
		VolumeMounts: append(
			controller.DaemonVolumeMounts(c.DataPathMap, rgwConfig.ResourceName, c.clusterSpec.DataDirHostPath), vaultVolMounts...),
		Resources:       c.store.Spec.Gateway.Resources,
		SecurityContext: controller.PodSecurityContext(),
	}
}

func (c *clusterConfig) makeChownInitContainer(rgwConfig *rgwConfig) v1.Container {
	return controller.ChownCephDataDirsInitContainer(
		*c.DataPathMap,
		c.clusterSpec.CephVersion.Image,
		controller.GetContainerImagePullPolicy(c.clusterSpec.CephVersion.ImagePullPolicy),
		controller.DaemonVolumeMounts(c.DataPathMap, rgwConfig.ResourceName, c.clusterSpec.DataDirHostPath),
		c.store.Spec.Gateway.Resources,
		controller.PodSecurityContext(),
		"",
	)
}

func (c *clusterConfig) makeDaemonContainer(rgwConfig *rgwConfig) (v1.Container, error) {
	// start the rgw daemon in the foreground
	startupProbe, err := c.defaultStartupProbe()
	if err != nil {
		return v1.Container{}, errors.Wrap(err, "failed to generate default startup probe")
	}
	readinessProbe, err := c.defaultReadinessProbe()
	if err != nil {
		return v1.Container{}, errors.Wrap(err, "failed to generate default readiness probe")
	}

	container := v1.Container{
		Name:            "rgw",
		Image:           c.clusterSpec.CephVersion.Image,
		ImagePullPolicy: controller.GetContainerImagePullPolicy(c.clusterSpec.CephVersion.ImagePullPolicy),
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
			controller.DaemonVolumeMounts(c.DataPathMap, rgwConfig.ResourceName, c.clusterSpec.DataDirHostPath),
			c.mimeTypesVolumeMount(),
		),
		Env:             controller.DaemonEnvVars(c.clusterSpec),
		Resources:       c.store.Spec.Gateway.Resources,
		StartupProbe:    startupProbe,
		LivenessProbe:   noLivenessProbe(),
		ReadinessProbe:  readinessProbe,
		SecurityContext: controller.PodSecurityContext(),
		WorkingDir:      cephconfig.VarLogCephDir,
	}

	// If the startup probe is enabled
	container = cephconfig.ConfigureStartupProbe(container, c.store.Spec.HealthCheck.StartupProbe)
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
		logger.Errorf("failed to enable SSE-KMS. %v", err)
		return v1.Container{}, err
	}
	if kmsEnabled {
		logger.Debugf("enabliing SSE-KMS. %v", c.store.Spec.Security.KeyManagementService)
		container.Args = append(container.Args, c.sseKMSDefaultOptions(kmsEnabled)...)
		if c.store.Spec.Security.KeyManagementService.IsTokenAuthEnabled() {
			container.Args = append(container.Args, c.sseKMSVaultTokenOptions(kmsEnabled)...)
		}
		if c.store.Spec.Security.KeyManagementService.IsTLSEnabled() {
			container.Args = append(container.Args, c.sseKMSVaultTLSOptions(kmsEnabled)...)
		}
	}

	if flags := buildRGWConfigFlags(c.store); len(flags) != 0 {
		container.Args = append(container.Args, flags...)
	}

	s3EncryptionEnabled, err := c.CheckRGWSSES3Enabled()
	if err != nil {
		return v1.Container{}, err
	}
	if s3EncryptionEnabled {
		logger.Debugf("enabliing SSE-S3. %v", c.store.Spec.Security.ServerSideEncryptionS3)

		container.Args = append(container.Args, c.sseS3DefaultOptions(s3EncryptionEnabled)...)
		if c.store.Spec.Security.ServerSideEncryptionS3.IsTokenAuthEnabled() {
			container.Args = append(container.Args, c.sseS3VaultTokenOptions(s3EncryptionEnabled)...)
		}
		if c.store.Spec.Security.ServerSideEncryptionS3.IsTLSEnabled() {
			container.Args = append(container.Args, c.sseS3VaultTLSOptions(s3EncryptionEnabled)...)
		}
	}

	if s3EncryptionEnabled || kmsEnabled {
		vaultVolMount := v1.VolumeMount{Name: rgwVaultVolumeName, MountPath: rgwVaultDirName}
		container.VolumeMounts = append(container.VolumeMounts, vaultVolMount)
	}

	hostingOptions, err := c.addDNSNamesToRGWServer()
	if err != nil {
		return v1.Container{}, err
	}
	if hostingOptions != "" {
		container.Args = append(container.Args, hostingOptions)
	}

	// user configs are very last arguments so that they override what Rook might be setting before
	for flag, val := range c.store.Spec.Gateway.RgwCommandFlags {
		container.Args = append(container.Args, cephconfig.NewFlag(flag, val))
	}

	return container, nil
}

// configureReadinessProbe returns the desired readiness probe for a given daemon
func configureReadinessProbe(container *v1.Container, healthCheck cephv1.ObjectHealthCheckSpec) {
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

// RGW has internal mechanisms for restarting its processing if it gets stuck. any failures detected
// in a liveness probe are likely to either (1) be resolved by the RGW internally or (2) be a result
// of connection issues to the Ceph cluster. in the first case, restarting is unnecessary. in the
// second case, restarting will only cause more load to the Ceph cluster by causing RGWs to attempt
// to re-connect, potentially causing more issues with the Ceph cluster. forcing a restart of the
// RGW is more likely to cause issues than solve them, so do not implement this probe.
func noLivenessProbe() *v1.Probe {
	return nil
}

func (c *clusterConfig) defaultReadinessProbe() (*v1.Probe, error) {
	probePath, disableProbe := getRGWProbePath(c.store.Spec.Protocols)
	if disableProbe {
		logger.Infof("disabling startup probe for %q store", c.store.Name)
		return nil, nil
	}
	proto, port := c.endpointInfo()
	cfg := rgwProbeConfig{
		ProbeType: ReadinessProbeType,
		Protocol:  proto,
		Port:      port.String(),
		Path:      probePath,
	}
	script, err := renderProbe(cfg)
	if err != nil {
		return nil, err
	}

	probe := &v1.Probe{
		ProbeHandler: v1.ProbeHandler{
			Exec: &v1.ExecAction{
				Command: []string{
					"bash", "-c", script,
				},
			},
		},
		TimeoutSeconds:      5,
		InitialDelaySeconds: 10,
		// if RGWs aren't responding reliably, remove them from service routing until they are stable
		PeriodSeconds:    10,
		FailureThreshold: 3,
		SuccessThreshold: 3, // don't re-add too soon to "flappy" RGWs from being rout-able
	}

	return probe, nil
}

// getRGWProbePath - returns custom path for RGW probe and returns true if probe should be disabled.
func getRGWProbePath(protocolSpec cephv1.ProtocolSpec) (path string, disable bool) {
	enabledAPIs := buildRGWEnableAPIsConfigVal(protocolSpec)
	if len(enabledAPIs) == 0 {
		// all apis including s3 are enabled
		// using default s3 Probe
		return "", false
	}
	if slices.Contains(enabledAPIs, "s3") {
		// using default s3 Probe
		return "", false
	}
	if slices.Contains(enabledAPIs, "swift") {
		// using swift api for probe
		// calculate path for swift probe
		prefix := "/swift/"
		if protocolSpec.Swift != nil && protocolSpec.Swift.UrlPrefix != nil && *protocolSpec.Swift.UrlPrefix != "" {
			prefix = *protocolSpec.Swift.UrlPrefix
			if !strings.HasPrefix(prefix, "/") {
				prefix = "/" + prefix
			}
			if !strings.HasSuffix(prefix, "/") {
				prefix += "/"
			}
		}
		prefix += "info"
		return prefix, false
	}
	// both swift and s3 are disabled - disable probe.
	return "", true
}

func (c *clusterConfig) defaultStartupProbe() (*v1.Probe, error) {
	probePath, disableProbe := getRGWProbePath(c.store.Spec.Protocols)
	if disableProbe {
		logger.Infof("disabling startup probe for %q store", c.store.Name)
		return nil, nil
	}
	proto, port := c.endpointInfo()
	cfg := rgwProbeConfig{
		ProbeType: StartupProbeType,
		Protocol:  proto,
		Port:      port.String(),
		Path:      probePath,
	}

	script, err := renderProbe(cfg)
	if err != nil {
		return nil, err
	}

	probe := &v1.Probe{
		ProbeHandler: v1.ProbeHandler{
			Exec: &v1.ExecAction{
				Command: []string{
					"bash", "-c", script,
				},
			},
		},
		TimeoutSeconds:      5,
		InitialDelaySeconds: 10,
		// RGW's default init timeout is 300 seconds; give an extra margin before the pod should be
		// restarted by kubernetes
		PeriodSeconds:    10,
		FailureThreshold: 33,
	}

	return probe, nil
}

func (c *clusterConfig) endpointInfo() (ProtocolType, *intstr.IntOrString) {
	// The port the liveness probe needs to probe
	// Assume we run on SDN by default
	proto := HTTPProtocol
	port := intstr.FromInt(int(rgwPortInternalPort))

	// If Host Networking is enabled, the port from the spec must be reflected
	if c.store.Spec.IsHostNetwork(c.clusterSpec) {
		proto = HTTPProtocol
		port = intstr.FromInt(int(c.store.Spec.Gateway.Port))
	}

	if c.store.Spec.Gateway.Port == 0 && c.store.Spec.IsTLSEnabled() {
		proto = HTTPSProtocol
		port = intstr.FromInt(int(c.store.Spec.Gateway.SecurePort))
	}

	logger.Debugf("rgw %q probe port is %v", c.store.Namespace+"/"+c.store.Name, port)
	return proto, &port
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
	if c.store.Spec.IsHostNetwork(c.clusterSpec) {
		svc.Spec.ClusterIP = v1.ClusterIPNone
	}

	_, destPort := c.endpointInfo()

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

	k8sEndpointAddrs := []v1.EndpointAddress{}
	for _, rookEndpoint := range cephObjectStore.Spec.Gateway.ExternalRgwEndpoints {
		k8sEndpointAddr := v1.EndpointAddress{
			IP:       rookEndpoint.IP,
			Hostname: rookEndpoint.Hostname,
		}
		k8sEndpointAddrs = append(k8sEndpointAddrs, k8sEndpointAddr)
	}

	endpoints := &v1.Endpoints{
		ObjectMeta: metav1.ObjectMeta{
			Name:      instanceName(cephObjectStore.Name),
			Namespace: cephObjectStore.Namespace,
			Labels:    labels,
		},
		Subsets: []v1.EndpointSubset{
			{
				Addresses: k8sEndpointAddrs,
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

	_, err = k8sutil.CreateOrUpdateEndpoint(c.clusterInfo.Context, c.context.Clientset, cephObjectStore.Namespace, endpoint)
	if err != nil {
		return errors.Wrapf(err, "failed to create or update object store %q endpoint", cephObjectStore.Name)
	}

	return nil
}

func (c *clusterConfig) reconcileService(store *cephv1.CephObjectStore) error {
	service := c.generateService(store)
	// Set owner ref to the parent object
	err := c.ownerInfo.SetControllerReference(service)
	if err != nil {
		return errors.Wrapf(err, "failed to set owner reference to ceph object store service %q", service.Name)
	}

	svc, err := k8sutil.CreateOrUpdateService(c.clusterInfo.Context, c.context.Clientset, store.Namespace, service)
	if err != nil {
		return errors.Wrapf(err, "failed to create or update object store %q service", store.Name)
	}

	logger.Infof("ceph object store gateway service running at %s", svc.Spec.ClusterIP)

	return nil
}

func (c *clusterConfig) vaultPrefixRGW() string {
	secretEngine := c.store.Spec.Security.KeyManagementService.ConnectionDetails[kms.VaultSecretEngineKey]
	var vaultPrefixPath string
	switch secretEngine {
	case kms.VaultKVSecretEngineKey:
		vaultPrefixPath = path.Join(vaultPrefix,
			c.store.Spec.Security.KeyManagementService.ConnectionDetails[vault.VaultBackendPathKey], "/data")
	case kms.VaultTransitSecretEngineKey:
		vaultPrefixPath = path.Join(vaultPrefix, secretEngine)
	}

	return vaultPrefixPath
}

func (c *clusterConfig) CheckRGWKMS() (bool, error) {
	if c.store.Spec.Security != nil && c.store.Spec.Security.KeyManagementService.IsEnabled() {
		err := kms.ValidateConnectionDetails(c.clusterInfo.Context, c.context, &c.store.Spec.Security.KeyManagementService, c.store.Namespace)
		if err != nil {
			return false, err
		}
		secretEngine := c.store.Spec.Security.KeyManagementService.ConnectionDetails[kms.VaultSecretEngineKey]

		// currently RGW supports kv(version 2) and transit secret engines in vault for sse:kms
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

func (c *clusterConfig) CheckRGWSSES3Enabled() (bool, error) {
	if c.store.Spec.Security != nil && c.store.Spec.Security.ServerSideEncryptionS3.IsEnabled() {
		err := kms.ValidateConnectionDetails(c.clusterInfo.Context, c.context, &c.store.Spec.Security.ServerSideEncryptionS3, c.store.Namespace)
		if err != nil {
			return false, err
		}

		// currently RGW supports only transit secret engines in vault for sse:s3
		if c.store.Spec.Security.ServerSideEncryptionS3.ConnectionDetails[kms.VaultSecretEngineKey] != kms.VaultTransitSecretEngineKey {
			return false, errors.New("vault secret engine is not transit")
		}
		return true, nil
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
	labels := controller.CephDaemonAppLabels(AppName, namespace, cephconfig.RgwType, name, name, "cephobjectstores.ceph.rook.io", includeNewLabels)
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

// Following apis set the RGW options if requested, since they are used in unit tests for validating different scenarios
func (c *clusterConfig) sseKMSDefaultOptions(setOptions bool) []string {
	if setOptions {
		return []string{
			cephconfig.NewFlag("rgw crypt s3 kms backend",
				c.store.Spec.Security.KeyManagementService.ConnectionDetails[kms.Provider]),
			cephconfig.NewFlag("rgw crypt vault addr",
				c.store.Spec.Security.KeyManagementService.ConnectionDetails[api.EnvVaultAddress]),
		}
	}
	return []string{}
}

func (c *clusterConfig) sseS3DefaultOptions(setOptions bool) []string {
	if setOptions {
		return []string{
			cephconfig.NewFlag("rgw crypt sse s3 backend",
				c.store.Spec.Security.ServerSideEncryptionS3.ConnectionDetails[kms.Provider]),
			cephconfig.NewFlag("rgw crypt sse s3 vault addr",
				c.store.Spec.Security.ServerSideEncryptionS3.ConnectionDetails[api.EnvVaultAddress]),
		}
	}
	return []string{}
}

func (c *clusterConfig) sseKMSVaultTokenOptions(setOptions bool) []string {
	if setOptions {
		return []string{
			cephconfig.NewFlag("rgw crypt vault auth", kms.KMSTokenSecretNameKey),
			cephconfig.NewFlag("rgw crypt vault token file",
				path.Join(rgwVaultDirName, sseKMS, kms.VaultFileName)),
			cephconfig.NewFlag("rgw crypt vault prefix", c.vaultPrefixRGW()),
			cephconfig.NewFlag("rgw crypt vault secret engine",
				c.store.Spec.Security.KeyManagementService.ConnectionDetails[kms.VaultSecretEngineKey]),
		}
	}
	return []string{}
}

func (c *clusterConfig) sseS3VaultTokenOptions(setOptions bool) []string {
	if setOptions {
		return []string{
			cephconfig.NewFlag("rgw crypt sse s3 vault auth", kms.KMSTokenSecretNameKey),
			cephconfig.NewFlag("rgw crypt sse s3 vault token file",
				path.Join(rgwVaultDirName, sseS3, kms.VaultFileName)),
			cephconfig.NewFlag("rgw crypt sse s3 vault prefix",
				path.Join(vaultPrefix, c.store.Spec.Security.ServerSideEncryptionS3.ConnectionDetails[kms.VaultSecretEngineKey])),
			cephconfig.NewFlag("rgw crypt sse s3 vault secret engine",
				c.store.Spec.Security.ServerSideEncryptionS3.ConnectionDetails[kms.VaultSecretEngineKey]),
		}
	}
	return []string{}
}

func (c *clusterConfig) sseKMSVaultTLSOptions(setOptions bool) []string {
	var rgwOptions []string
	if setOptions {
		rgwOptions = append(rgwOptions, cephconfig.NewFlag("rgw crypt vault verify ssl", "true"))

		if kms.GetParam(c.store.Spec.Security.KeyManagementService.ConnectionDetails, api.EnvVaultClientCert) != "" {
			rgwOptions = append(rgwOptions,
				cephconfig.NewFlag("rgw crypt vault ssl clientcert", path.Join(rgwVaultDirName, sseKMS, kms.VaultCertFileName)))
		}
		if kms.GetParam(c.store.Spec.Security.KeyManagementService.ConnectionDetails, api.EnvVaultClientKey) != "" {
			rgwOptions = append(rgwOptions,
				cephconfig.NewFlag("rgw crypt vault ssl clientkey", path.Join(rgwVaultDirName, sseKMS, kms.VaultKeyFileName)))
		}
		if kms.GetParam(c.store.Spec.Security.KeyManagementService.ConnectionDetails, api.EnvVaultCACert) != "" {
			rgwOptions = append(rgwOptions,
				cephconfig.NewFlag("rgw crypt vault ssl cacert", path.Join(rgwVaultDirName, sseKMS, kms.VaultCAFileName)))
		}
	}
	return rgwOptions
}

func (c *clusterConfig) sseS3VaultTLSOptions(setOptions bool) []string {
	var rgwOptions []string
	if setOptions {
		rgwOptions = append(rgwOptions, cephconfig.NewFlag("rgw crypt sse s3 vault verify ssl", "true"))

		if kms.GetParam(c.store.Spec.Security.ServerSideEncryptionS3.ConnectionDetails, api.EnvVaultClientCert) != "" {
			rgwOptions = append(rgwOptions,
				cephconfig.NewFlag("rgw crypt sse s3 vault ssl clientcert", path.Join(rgwVaultDirName, sseS3, kms.VaultCertFileName)))
		}
		if kms.GetParam(c.store.Spec.Security.ServerSideEncryptionS3.ConnectionDetails, api.EnvVaultClientKey) != "" {
			rgwOptions = append(rgwOptions,
				cephconfig.NewFlag("rgw crypt sse s3 vault ssl clientkey", path.Join(rgwVaultDirName, sseS3, kms.VaultKeyFileName)))
		}
		if kms.GetParam(c.store.Spec.Security.ServerSideEncryptionS3.ConnectionDetails, api.EnvVaultCACert) != "" {
			rgwOptions = append(rgwOptions,
				cephconfig.NewFlag("rgw crypt sse s3 vault ssl cacert", path.Join(rgwVaultDirName, sseS3, kms.VaultCAFileName)))
		}
	}
	return rgwOptions
}

// Builds list of rgw config parameters which should be passed as CLI flags.
// Consider set config option as flag if BOTH criteria fulfilled:
//  1. config value is not secret
//  2. config change requires RGW daemon restart
//
// Otherwise set rgw config parameter to mon database in ./config.go -> setFlagsMonConfigStore()
// CLI flags override values from mon db: see ceph config docs: https://docs.ceph.com/en/latest/rados/configuration/ceph-conf/#config-sources
func buildRGWConfigFlags(objectStore *cephv1.CephObjectStore) []string {
	var res []string
	// todo: move all flags here
	if enableAPIs := buildRGWEnableAPIsConfigVal(objectStore.Spec.Protocols); len(enableAPIs) != 0 {
		res = append(res, cephconfig.NewFlag("rgw_enable_apis", strings.Join(enableAPIs, ",")))
		logger.Debugf("Enabling APIs for RGW instance %q: %s", objectStore.Name, enableAPIs)
	}
	return res
}

func buildRGWEnableAPIsConfigVal(protocolSpec cephv1.ProtocolSpec) []string {
	if len(protocolSpec.EnableAPIs) != 0 {
		// handle explicit enabledAPIS spec
		enabledAPIs := make([]string, len(protocolSpec.EnableAPIs))
		for i, v := range protocolSpec.EnableAPIs {
			enabledAPIs[i] = strings.TrimSpace(string(v))
		}
		return enabledAPIs
	}

	// if enabledAPIs not set, check if S3 should be disabled
	if protocolSpec.S3 != nil && protocolSpec.S3.Enabled != nil && !*protocolSpec.S3.Enabled { //nolint // disable deprecation check
		return rgwAPIwithoutS3
	}
	// see also https://docs.ceph.com/en/octopus/radosgw/config-ref/#swift-settings on disabling s3
	// when using '/' as prefix
	if protocolSpec.Swift != nil && protocolSpec.Swift.UrlPrefix != nil && *protocolSpec.Swift.UrlPrefix == "/" {
		logger.Warning("Forcefully disabled S3 as the swift prefix is given as a slash /. Ignoring any S3 options (including Enabled=true)!")
		return rgwAPIwithoutS3
	}
	return nil
}

func renderProbe(cfg rgwProbeConfig) (string, error) {
	var writer bytes.Buffer
	name := string(cfg.ProbeType) + "-probe"

	t := template.New(name)
	t, err := t.Parse(rgwProbeScriptTemplate)
	if err != nil {
		return "", errors.Wrapf(err, "failed to parse template %q", name)
	}

	if err := t.Execute(&writer, cfg); err != nil {
		return "", errors.Wrapf(err, "failed to render template %q", name)
	}

	return writer.String(), nil
}

func (c *clusterConfig) addDNSNamesToRGWServer() (string, error) {
	if c.store.Spec.Hosting == nil {
		return "", nil
	}
	if !c.store.AdvertiseEndpointIsSet() && len(c.store.Spec.Hosting.DNSNames) == 0 {
		return "", nil
	}
	if !c.clusterInfo.CephVersion.IsAtLeastReef() {
		return "", errors.New("rgw dns names are supported from ceph v18 onwards")
	}

	dnsNames := []string{}

	if c.store.AdvertiseEndpointIsSet() {
		dnsNames = append(dnsNames, c.store.Spec.Hosting.AdvertiseEndpoint.DnsName)
	}

	dnsNames = append(dnsNames, c.store.Spec.Hosting.DNSNames...)

	// add default RGW service domain name to ensure RGW doesn't reject it
	dnsNames = append(dnsNames, c.store.GetServiceDomainName())

	// add custom endpoints from zone spec if exists
	if c.store.Spec.Zone.Name != "" {
		zone, err := c.context.RookClientset.CephV1().CephObjectZones(c.store.Namespace).Get(c.clusterInfo.Context, c.store.Spec.Zone.Name, metav1.GetOptions{})
		if err != nil {
			return "", err
		}
		dnsNames = append(dnsNames, zone.Spec.CustomEndpoints...)
	}

	// validate dns names
	var hostNames []string
	for _, dnsName := range dnsNames {
		hostName, err := GetHostnameFromEndpoint(dnsName)
		if err != nil {
			return "", errors.Wrap(err,
				"failed to interpret endpoint from one of the following sources: CephObjectStore.spec.hosting.dnsNames, CephObjectZone.spec.customEndpoints")
		}
		hostNames = append(hostNames, hostName)
	}

	// remove duplicate host names
	checkDuplicate := make(map[string]bool)
	removeDuplicateHostNames := []string{}
	for _, hostName := range hostNames {
		if _, ok := checkDuplicate[hostName]; !ok {
			checkDuplicate[hostName] = true
			removeDuplicateHostNames = append(removeDuplicateHostNames, hostName)
		}
	}

	return cephconfig.NewFlag("rgw dns name", strings.Join(removeDuplicateHostNames, ",")), nil
}

func GetHostnameFromEndpoint(endpoint string) (string, error) {
	if len(endpoint) == 0 {
		return "", fmt.Errorf("endpoint cannot be empty string")
	}

	// if endpoint doesn't end in '/', Ceph adds it
	// Rook can do this also to get more accurate error results from this function
	if !strings.HasSuffix(endpoint, "/") {
		endpoint = endpoint + "/"
	}

	// url.Parse() requires a protocol to parse the host name correctly
	if !strings.Contains(endpoint, "://") {
		endpoint = "http://" + endpoint
	}

	// "net/url".Parse() assumes that a URL is a "path" with optional things surrounding it.
	// For Ceph RGWs, we assume an endpoint is a "hostname" with optional things surrounding it.
	// Because of this difference in fundamental assumption, Rook needs to adjust some endpoints
	// used as input to url.Parse() to allow the function to extract a hostname reliably. Also,
	// Rook needs to look at several parts of the url.Parse() output to identify more failure scenarios
	parsedURL, err := url.Parse(endpoint)
	if err != nil {
		return "", err
	}

	// error in this case: url.Parse("https://http://hostname") parses without error with `Host = "http:"`
	// also catches issue where user adds colon but no port number after
	if strings.HasSuffix(parsedURL.Host, ":") {
		return "", fmt.Errorf("host %q parsed from endpoint %q has invalid colon (:) placement", parsedURL.Host, endpoint)
	}

	hostname := parsedURL.Hostname()
	dnsErrs := validation.IsDNS1123Subdomain(parsedURL.Hostname())
	if len(dnsErrs) > 0 {
		return "", fmt.Errorf("hostname %q parsed from endpoint %q is not a valid DNS-1123 subdomain: %v", hostname, endpoint, strings.Join(dnsErrs, ", "))
	}

	return hostname, nil
}
