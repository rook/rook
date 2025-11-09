/*
Copyright 2025 The Rook Authors. All rights reserved.

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

package nvmeof

import (
	_ "embed"
	"fmt"
	"strings"

	"github.com/pkg/errors"
	cephv1 "github.com/rook/rook/pkg/apis/ceph.rook.io/v1"
	cephconfig "github.com/rook/rook/pkg/operator/ceph/config"
	"github.com/rook/rook/pkg/operator/ceph/controller"
	"github.com/rook/rook/pkg/operator/k8sutil"
	apps "k8s.io/api/apps/v1"
	v1 "k8s.io/api/core/v1"
	kerrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/apimachinery/pkg/util/validation"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

const (
	AppName             = "rook-ceph-nvmeof"
	nvmeofIOPort        = 4420
	nvmeofGatewayPort   = 5500
	nvmeofMonitorPort   = 5499
	nvmeofDiscoveryPort = 8009
	defaultNVMeOFImage  = "quay.io/ceph/nvmeof:1.5"
	configKey           = "config"
	serviceAccountName  = "rook-ceph-nvmeof"
)

// getPorts returns the configured ports with defaults
func getPorts(nvmeof *cephv1.CephNVMeOFGateway) (ioPort, gatewayPort, monitorPort, discoveryPort int32) {
	ioPort = nvmeofIOPort
	gatewayPort = nvmeofGatewayPort
	monitorPort = nvmeofMonitorPort
	discoveryPort = nvmeofDiscoveryPort

	if nvmeof.Spec.Ports != nil {
		if nvmeof.Spec.Ports.IOPort != 0 {
			ioPort = nvmeof.Spec.Ports.IOPort
		}
		if nvmeof.Spec.Ports.GatewayPort != 0 {
			gatewayPort = nvmeof.Spec.Ports.GatewayPort
		}
		if nvmeof.Spec.Ports.MonitorPort != 0 {
			monitorPort = nvmeof.Spec.Ports.MonitorPort
		}
		if nvmeof.Spec.Ports.DiscoveryPort != 0 {
			discoveryPort = nvmeof.Spec.Ports.DiscoveryPort
		}
	}

	return ioPort, gatewayPort, monitorPort, discoveryPort
}

//go:embed connectionconfig.sh
var connectionConfigScript string

func (r *ReconcileCephNVMeOFGateway) generateCephNVMeOFService(nvmeof *cephv1.CephNVMeOFGateway, daemonID string) *v1.Service {
	labels := getLabels(nvmeof, daemonID)
	serviceName := instanceName(nvmeof, daemonID)
	ioPort, gatewayPort, monitorPort, discoveryPort := getPorts(nvmeof)

	svc := &v1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      serviceName,
			Namespace: nvmeof.Namespace,
			Labels:    labels,
		},
		Spec: v1.ServiceSpec{
			Selector: labels,
			Ports: []v1.ServicePort{
				{
					Name:       "io",
					Port:       ioPort,
					TargetPort: intstr.FromInt(int(ioPort)),
					Protocol:   v1.ProtocolTCP,
				},
				{
					Name:       "gateway",
					Port:       gatewayPort,
					TargetPort: intstr.FromInt(int(gatewayPort)),
					Protocol:   v1.ProtocolTCP,
				},
				{
					Name:       "monitor",
					Port:       monitorPort,
					TargetPort: intstr.FromInt(int(monitorPort)),
					Protocol:   v1.ProtocolTCP,
				},
				{
					Name:       "discovery",
					Port:       discoveryPort,
					TargetPort: intstr.FromInt(int(discoveryPort)),
					Protocol:   v1.ProtocolTCP,
				},
			},
		},
	}

	hostNetwork := nvmeof.IsHostNetwork(r.cephClusterSpec)
	if hostNetwork {
		svc.Spec.ClusterIP = v1.ClusterIPNone
	}

	return svc
}

func (r *ReconcileCephNVMeOFGateway) createCephNVMeOFService(nvmeof *cephv1.CephNVMeOFGateway, daemonID string) error {
	s := r.generateCephNVMeOFService(nvmeof, daemonID)

	err := controllerutil.SetControllerReference(nvmeof, s, r.scheme)
	if err != nil {
		logger.Errorf("failed to set owner reference: %v", err)
		return errors.Wrapf(err, "failed to set owner reference to ceph nvmeof gateway %q", s)
	}

	svc, err := r.context.Clientset.CoreV1().Services(nvmeof.Namespace).Create(r.opManagerContext, s, metav1.CreateOptions{})
	if err != nil {
		if !kerrors.IsAlreadyExists(err) {
			logger.Errorf("failed to create service: %v", err)
			return errors.Wrap(err, "failed to create nvmeof gateway service")
		}
		logger.Infof("ceph nvmeof gateway service already created")
		return nil
	}

	_, _, _, discoveryPort := getPorts(nvmeof)
	logger.Infof("ceph nvmeof gateway service running at %s:%d", svc.Spec.ClusterIP, discoveryPort)
	return nil
}

func (r *ReconcileCephNVMeOFGateway) makeDeployment(nvmeof *cephv1.CephNVMeOFGateway, daemonID, configMapName, configHash string) (*apps.Deployment, error) {
	if r == nil {
		logger.Errorf("receiver is nil")
		return nil, errors.New("receiver is nil")
	}
	if r.clusterInfo == nil {
		logger.Errorf("clusterInfo is nil")
		return nil, errors.New("clusterInfo is nil")
	}
	if r.clusterInfo.CephVersion.String() == "" {
		logger.Errorf("CephVersion is empty")
		return nil, errors.New("CephVersion is empty")
	}

	resourceName := instanceName(nvmeof, daemonID)
	deployment := &apps.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      resourceName,
			Namespace: nvmeof.Namespace,
			Labels:    getLabels(nvmeof, daemonID),
		},
	}

	hostNetwork := nvmeof.IsHostNetwork(r.cephClusterSpec)

	k8sutil.AddRookVersionLabelToDeployment(deployment)
	controller.AddCephVersionLabelToDeployment(r.clusterInfo.CephVersion, deployment)
	nvmeof.Spec.Annotations.ApplyToObjectMeta(&deployment.ObjectMeta)
	nvmeof.Spec.Labels.ApplyToObjectMeta(&deployment.ObjectMeta)

	cephConfigVol, cephConfigMount := controller.ConfGeneratedInPodVolumeAndMount()
	gatewayConfigVol, gatewayConfigMount := gatewayConfigVolumeAndMount(configMapName)

	initContainer := r.createCephConfigInitContainer(nvmeof, daemonID, gatewayConfigMount)
	daemonContainer := r.daemonContainer(nvmeof, cephConfigMount)

	gatewayName := instanceName(nvmeof, daemonID)
	podSpec := v1.PodSpec{
		InitContainers: []v1.Container{
			initContainer,
		},
		Containers: []v1.Container{
			daemonContainer,
		},
		RestartPolicy: v1.RestartPolicyAlways,
		Volumes: []v1.Volume{
			cephConfigVol,
			adminKeyringVolume(),
			gatewayConfigVol,
		},
		HostNetwork:        hostNetwork,
		PriorityClassName:  nvmeof.Spec.PriorityClassName,
		SecurityContext:    &v1.PodSecurityContext{},
		ServiceAccountName: serviceAccountName,
	}

	// Ensure a stable hostname inside the pod (does NOT make the pod IP stable).
	// This matches the gateway instance name used elsewhere by the operator.
	if errs := validation.IsDNS1123Label(gatewayName); len(errs) == 0 {
		podSpec.Hostname = gatewayName
	} else {
		logger.Warningf("not setting pod hostname %q (must be a DNS-1123 label): %v", gatewayName, errs)
	}

	k8sutil.AddUnreachableNodeToleration(&podSpec)

	if hostNetwork {
		podSpec.DNSPolicy = v1.DNSClusterFirstWithHostNet
	}
	nvmeof.Spec.Placement.ApplyToPodSpec(&podSpec)

	podTemplateSpec := v1.PodTemplateSpec{
		ObjectMeta: metav1.ObjectMeta{
			Name:   resourceName,
			Labels: getLabels(nvmeof, daemonID),
			Annotations: map[string]string{
				"config-hash": configHash,
			},
		},
		Spec: podSpec,
	}

	if hostNetwork {
		podSpec.DNSPolicy = v1.DNSClusterFirstWithHostNet
	} else if r.cephClusterSpec.Network.IsMultus() {
		if err := k8sutil.ApplyMultus(r.clusterInfo.Namespace, &r.cephClusterSpec.Network, &podTemplateSpec.ObjectMeta); err != nil {
			logger.Errorf("failed to apply Multus configuration: %v", err)
			return nil, err
		}
	}

	nvmeof.Spec.Annotations.ApplyToObjectMeta(&podTemplateSpec.ObjectMeta)
	nvmeof.Spec.Labels.ApplyToObjectMeta(&podTemplateSpec.ObjectMeta)

	replicas := int32(1)
	revisionHistoryLimit := controller.RevisionHistoryLimit()
	deployment.Spec = apps.DeploymentSpec{
		RevisionHistoryLimit: revisionHistoryLimit,
		Selector:             &metav1.LabelSelector{MatchLabels: getLabels(nvmeof, daemonID)},
		Template:             podTemplateSpec,
		Replicas:             &replicas,
	}

	return deployment, nil
}

// createCephConfigInitContainer creates a minimal init container that generates
// /etc/ceph/ceph.conf, copies the admin keyring, and copies nvmeof.conf from the ConfigMap.
// This is needed for the gateway to connect to the Ceph cluster.
func (r *ReconcileCephNVMeOFGateway) createCephConfigInitContainer(nvmeof *cephv1.CephNVMeOFGateway, daemonID string, gatewayConfigMount v1.VolumeMount) v1.Container {
	_, cephConfigMount := controller.ConfGeneratedInPodVolumeAndMount()

	cephImage := r.cephClusterSpec.CephVersion.Image
	imagePullPolicy := controller.GetContainerImagePullPolicy(r.cephClusterSpec.CephVersion.ImagePullPolicy)

	// Build Ceph CLI arguments using Rook's helper functions
	// Use AdminFlags since the init container uses admin keyring for Ceph commands
	// Override the keyring path to use /etc/ceph/keyring (where the script copies it)
	cephArgs := controller.AdminFlags(r.clusterInfo)
	// Replace the keyring path with the one used by the init container script
	for i, arg := range cephArgs {
		if strings.HasPrefix(arg, "--keyring=") {
			cephArgs[i] = "--keyring=/etc/ceph/keyring"
			break
		}
	}

	// Use the embedded script and pass Ceph CLI args via "$@"
	script := connectionConfigScript

	gatewayName := instanceName(nvmeof, daemonID)
	poolName := nvmeof.Spec.Pool
	anaGroup := nvmeof.Spec.Group

	// Build environment variables using Rook's helper functions
	// DaemonEnvVars already includes StoredMonHostEnvVars() which provides ROOK_CEPH_MON_HOST
	envVars := controller.DaemonEnvVars(r.cephClusterSpec)
	envVars = append(envVars,
		v1.EnvVar{
			Name:  "GATEWAY_NAME",
			Value: gatewayName,
		},
		v1.EnvVar{
			Name:  "POOL_NAME",
			Value: poolName,
		},
		v1.EnvVar{
			Name:  "ANA_GROUP",
			Value: anaGroup,
		},
		k8sutil.PodIPEnvVar("POD_IP"),
	)

	privileged := true
	container := v1.Container{
		Name:            "generate-ceph-conf",
		Image:           cephImage,
		ImagePullPolicy: imagePullPolicy,
		Command: []string{
			"/bin/bash",
			"-c",
			script,
		},
		Args: cephArgs,
		Env:  envVars,
		VolumeMounts: []v1.VolumeMount{
			adminKeyringVolumeMount(),
			cephConfigMount,
			gatewayConfigMount,
		},
		Resources: nvmeof.Spec.Resources,
		SecurityContext: &v1.SecurityContext{
			Privileged: &privileged,
			Capabilities: &v1.Capabilities{
				Add: []v1.Capability{
					"SYS_ADMIN",
				},
				Drop: []v1.Capability{
					"NET_RAW",
				},
			},
		},
	}
	return container
}

func (r *ReconcileCephNVMeOFGateway) daemonContainer(nvmeof *cephv1.CephNVMeOFGateway, cephConfigMount v1.VolumeMount) v1.Container {
	privileged := true
	container := v1.Container{
		Name: "nvmeof-gateway",
		Args: []string{
			"-c",
			"/etc/ceph/nvmeof.conf",
		},

		Image:           getNVMeOFImage(nvmeof),
		ImagePullPolicy: v1.PullIfNotPresent,
		VolumeMounts: []v1.VolumeMount{
			cephConfigMount,
		},
		Env: func() []v1.EnvVar {
			envVars := controller.DaemonEnvVars(r.cephClusterSpec)
			// Build CEPH_ARGS using DefaultFlags which includes fsid, keyring, logging flags, and mon host flags
			cephArgs := cephconfig.DefaultFlags(r.clusterInfo.FSID, "/etc/ceph/keyring")
			cephArgsStr := strings.Join(cephArgs, " ")
			logger.Infof("CEPH_ARGS for nvmeof gateway: %s", cephArgsStr)
			envVars = append(envVars, v1.EnvVar{
				Name:  "CEPH_ARGS",
				Value: cephArgsStr,
			})
			return envVars
		}(),

		Ports: func() []v1.ContainerPort {
			ioPort, gatewayPort, monitorPort, discoveryPort := getPorts(nvmeof)
			return []v1.ContainerPort{
				{
					Name:          "io",
					ContainerPort: ioPort,
					Protocol:      v1.ProtocolTCP,
				},
				{
					Name:          "gateway",
					ContainerPort: gatewayPort,
					Protocol:      v1.ProtocolTCP,
				},
				{
					Name:          "monitor",
					ContainerPort: monitorPort,
					Protocol:      v1.ProtocolTCP,
				},
				{
					Name:          "discovery",
					ContainerPort: discoveryPort,
					Protocol:      v1.ProtocolTCP,
				},
			}
		}(),

		Resources: nvmeof.Spec.Resources,
		SecurityContext: &v1.SecurityContext{
			Privileged: &privileged,
			Capabilities: &v1.Capabilities{
				Add: []v1.Capability{
					"SYS_ADMIN",
				},
				Drop: []v1.Capability{
					"NET_RAW",
				},
			},
		},
	}

	if nvmeof.Spec.LivenessProbe != nil && !nvmeof.Spec.LivenessProbe.Disabled && nvmeof.Spec.LivenessProbe.Probe == nil {
		container.LivenessProbe = r.defaultLivenessProbe(nvmeof)
	}
	result := cephconfig.ConfigureLivenessProbe(container, nvmeof.Spec.LivenessProbe)
	return result
}

func (r *ReconcileCephNVMeOFGateway) defaultLivenessProbe(nvmeof *cephv1.CephNVMeOFGateway) *v1.Probe {
	ioPort, _, _, _ := getPorts(nvmeof)
	return controller.GenerateLivenessProbeTcpPort(ioPort, 10)
}

func getLabels(n *cephv1.CephNVMeOFGateway, daemonID string) map[string]string {
	return controller.CephDaemonAppLabels(
		AppName, n.Namespace, "nvmeof", n.Name+"-"+daemonID, n.Name,
		"cephnvmeofgateways.ceph.rook.io", true,
	)
}

func instanceName(nvmeof *cephv1.CephNVMeOFGateway, daemonID string) string {
	return fmt.Sprintf("%s-%s-%s", AppName, nvmeof.Name, daemonID)
}

// getNVMeOFImage returns the image to use for the NVMe-oF gateway.
// If the image is specified in the spec, it is used; otherwise, the default image is returned.
func getNVMeOFImage(nvmeof *cephv1.CephNVMeOFGateway) string {
	if nvmeof.Spec.Image != "" {
		return nvmeof.Spec.Image
	}
	return defaultNVMeOFImage
}

func gatewayConfigVolumeAndMount(configConfigMap string) (v1.Volume, v1.VolumeMount) {
	// Mount to /config temporarily - init container will copy to /etc/ceph/nvmeof.conf
	cfgDir := "/config"
	cfgVolName := "gateway-config"
	configMapSource := &v1.ConfigMapVolumeSource{
		LocalObjectReference: v1.LocalObjectReference{Name: configConfigMap},
		Items:                []v1.KeyToPath{{Key: configKey, Path: "nvmeof.conf"}},
	}
	v := v1.Volume{Name: cfgVolName, VolumeSource: v1.VolumeSource{ConfigMap: configMapSource}}
	m := v1.VolumeMount{Name: cfgVolName, MountPath: cfgDir, ReadOnly: true}
	return v, m
}

func adminKeyringVolume() v1.Volume {
	mode := int32(292)
	return v1.Volume{
		Name: "ceph-admin-keyring",
		VolumeSource: v1.VolumeSource{
			Secret: &v1.SecretVolumeSource{
				SecretName: "rook-ceph-admin-keyring",
				Items: []v1.KeyToPath{
					{
						Key:  "keyring",
						Path: "keyring",
						Mode: &mode,
					},
				},
			},
		},
	}
}

func adminKeyringVolumeMount() v1.VolumeMount {
	return v1.VolumeMount{
		Name:      "ceph-admin-keyring",
		MountPath: "/tmp/ceph/",
		ReadOnly:  true,
	}
}
