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
	cephclient "github.com/rook/rook/pkg/daemon/ceph/client"
	cephconfig "github.com/rook/rook/pkg/operator/ceph/config"
	"github.com/rook/rook/pkg/operator/ceph/controller"
	"github.com/rook/rook/pkg/operator/k8sutil"
	apps "k8s.io/api/apps/v1"
	v1 "k8s.io/api/core/v1"
	kerrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
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
	serviceAccountName  = "ceph-nvmeof-gateway"
)

// getPorts returns the configured ports with defaults
func getPorts(nvmeof *cephv1.CephNVMeOFGateway) (ioPort, gatewayPort, monitorPort, discoveryPort int32) {
	ioPort = nvmeofIOPort
	gatewayPort = nvmeofGatewayPort
	monitorPort = nvmeofMonitorPort
	discoveryPort = nvmeofDiscoveryPort

	if nvmeof.Spec.Ports != nil {
		if nvmeof.Spec.Ports.IOPort != nil {
			ioPort = *nvmeof.Spec.Ports.IOPort
		}
		if nvmeof.Spec.Ports.GatewayPort != nil {
			gatewayPort = *nvmeof.Spec.Ports.GatewayPort
		}
		if nvmeof.Spec.Ports.MonitorPort != nil {
			monitorPort = *nvmeof.Spec.Ports.MonitorPort
		}
		if nvmeof.Spec.Ports.DiscoveryPort != nil {
			discoveryPort = *nvmeof.Spec.Ports.DiscoveryPort
		}
	}

	return ioPort, gatewayPort, monitorPort, discoveryPort
}

//go:embed connectionconfig.sh
var connectionConfigScript string

func (r *ReconcileCephNVMeOFGateway) generateCephNVMeOFService(nvmeof *cephv1.CephNVMeOFGateway, daemonID string) *v1.Service {
	logger.Debugf("generateCephNVMeOFService() started: gateway=%s/%s, daemonID=%s",
		nvmeof.Namespace, nvmeof.Name, daemonID)

	logger.Debug("generateCephNVMeOFService() generating labels")
	labels := getLabels(nvmeof, daemonID, true)
	logger.Debugf("generateCephNVMeOFService() labels generated: %v", labels)

	serviceName := instanceName(nvmeof, daemonID)
	logger.Debugf("generateCephNVMeOFService() service name: %s", serviceName)

	logger.Debug("generateCephNVMeOFService() getting configured ports")
	ioPort, gatewayPort, monitorPort, discoveryPort := getPorts(nvmeof)
	logger.Debugf("generateCephNVMeOFService() ports: io=%d, gateway=%d, monitor=%d, discovery=%d",
		ioPort, gatewayPort, monitorPort, discoveryPort)

	logger.Debug("generateCephNVMeOFService() creating service spec with ports")
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
	logger.Debugf("generateCephNVMeOFService() service spec created with %d ports", len(svc.Spec.Ports))

	hostNetwork := nvmeof.IsHostNetwork(r.cephClusterSpec)
	logger.Debugf("generateCephNVMeOFService() hostNetwork check: %v", hostNetwork)
	if hostNetwork {
		logger.Debug("generateCephNVMeOFService() hostNetwork enabled, setting ClusterIP to None")
		svc.Spec.ClusterIP = v1.ClusterIPNone
	}

	logger.Debugf("generateCephNVMeOFService() completed: service=%s/%s", svc.Namespace, svc.Name)
	return svc
}

func (r *ReconcileCephNVMeOFGateway) createCephNVMeOFService(nvmeof *cephv1.CephNVMeOFGateway, daemonID string) error {
	logger.Debugf("createCephNVMeOFService() started: gateway=%s/%s, daemonID=%s",
		nvmeof.Namespace, nvmeof.Name, daemonID)

	logger.Debug("createCephNVMeOFService() generating service spec")
	s := r.generateCephNVMeOFService(nvmeof, daemonID)

	logger.Debugf("createCephNVMeOFService() setting owner reference on service %s", s.Name)
	err := controllerutil.SetControllerReference(nvmeof, s, r.scheme)
	if err != nil {
		logger.Errorf("createCephNVMeOFService() failed to set owner reference: %v", err)
		return errors.Wrapf(err, "failed to set owner reference to ceph nvmeof gateway %q", s)
	}
	logger.Debug("createCephNVMeOFService() owner reference set successfully")

	logger.Debugf("createCephNVMeOFService() creating service %s in namespace %s", s.Name, nvmeof.Namespace)
	svc, err := r.context.Clientset.CoreV1().Services(nvmeof.Namespace).Create(r.opManagerContext, s, metav1.CreateOptions{})
	if err != nil {
		if !kerrors.IsAlreadyExists(err) {
			logger.Errorf("createCephNVMeOFService() failed to create service: %v", err)
			return errors.Wrap(err, "failed to create nvmeof gateway service")
		}
		logger.Debugf("createCephNVMeOFService() service %s already exists", s.Name)
		logger.Infof("ceph nvmeof gateway service already created")
		return nil
	}

	logger.Debugf("createCephNVMeOFService() service created successfully: name=%s, clusterIP=%s",
		svc.Name, svc.Spec.ClusterIP)
	_, _, _, discoveryPort := getPorts(nvmeof)
	logger.Infof("ceph nvmeof gateway service running at %s:%d", svc.Spec.ClusterIP, discoveryPort)
	logger.Debugf("createCephNVMeOFService() completed successfully")
	return nil
}

func (r *ReconcileCephNVMeOFGateway) makeDeployment(nvmeof *cephv1.CephNVMeOFGateway, daemonID, configMapName, configHash string) (*apps.Deployment, error) {
	logger.Debugf("makeDeployment() started: gateway=%s/%s, daemonID=%s, configMapName=%s, configHash=%s",
		nvmeof.Namespace, nvmeof.Name, daemonID, configMapName, configHash)

	if r == nil {
		logger.Errorf("makeDeployment() receiver is nil")
		return nil, errors.New("receiver is nil")
	}
	if r.clusterInfo == nil {
		logger.Errorf("makeDeployment() clusterInfo is nil")
		return nil, errors.New("clusterInfo is nil")
	}
	if r.clusterInfo.CephVersion.String() == "" {
		logger.Errorf("makeDeployment() CephVersion is empty")
		return nil, errors.New("CephVersion is empty")
	}

	resourceName := instanceName(nvmeof, daemonID)
	logger.Debugf("makeDeployment() resource name: %s", resourceName)

	logger.Debug("makeDeployment() creating deployment object")
	deployment := &apps.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      resourceName,
			Namespace: nvmeof.Namespace,
			Labels:    getLabels(nvmeof, daemonID, true),
		},
	}

	hostNetwork := nvmeof.IsHostNetwork(r.cephClusterSpec)
	logger.Debugf("makeDeployment() hostNetwork: %v", hostNetwork)

	logger.Debug("makeDeployment() adding rook version label")
	k8sutil.AddRookVersionLabelToDeployment(deployment)
	logger.Debugf("makeDeployment() adding ceph version label: %s", r.clusterInfo.CephVersion.String())
	controller.AddCephVersionLabelToDeployment(r.clusterInfo.CephVersion, deployment)
	logger.Debug("makeDeployment() applying annotations and labels from spec")
	nvmeof.Spec.Annotations.ApplyToObjectMeta(&deployment.ObjectMeta)
	nvmeof.Spec.Labels.ApplyToObjectMeta(&deployment.ObjectMeta)

	logger.Debug("makeDeployment() creating volume mounts")
	cephConfigVol, cephConfigMount := cephConfigVolumeAndMount()
	logger.Debugf("makeDeployment() ceph config volume: %s, mount path: %s", cephConfigVol.Name, cephConfigMount.MountPath)
	gatewayConfigVol, gatewayConfigMount := gatewayConfigVolumeAndMount(configMapName)
	logger.Debugf("makeDeployment() gateway config volume: %s, mount path: %s", gatewayConfigVol.Name, gatewayConfigMount.MountPath)

	logger.Debug("makeDeployment() creating pod spec")
	logger.Debug("makeDeployment() creating init container for ceph.conf and nvmeof.conf")
	initContainer := r.createCephConfigInitContainer(nvmeof, daemonID, gatewayConfigMount)
	logger.Debugf("makeDeployment() init container: name=%s, image=%s", initContainer.Name, initContainer.Image)

	logger.Debug("makeDeployment() creating daemon container spec")
	daemonContainer := r.daemonContainer(nvmeof, cephConfigMount)
	logger.Debugf("makeDeployment() daemon container: name=%s, image=%s", daemonContainer.Name, daemonContainer.Image)

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
	logger.Debugf("makeDeployment() pod spec created: %d init containers, %d containers, %d volumes",
		len(podSpec.InitContainers), len(podSpec.Containers), len(podSpec.Volumes))
	logger.Debugf("makeDeployment() service account: %s", serviceAccountName)
	if nvmeof.Spec.PriorityClassName != "" {
		logger.Debugf("makeDeployment() priority class: %s", nvmeof.Spec.PriorityClassName)
	}

	logger.Debug("makeDeployment() adding unreachable node toleration")
	k8sutil.AddUnreachableNodeToleration(&podSpec)

	if hostNetwork {
		logger.Debug("makeDeployment() hostNetwork enabled, setting DNS policy")
		podSpec.DNSPolicy = v1.DNSClusterFirstWithHostNet
	}
	logger.Debug("makeDeployment() applying placement configuration")
	nvmeof.Spec.Placement.ApplyToPodSpec(&podSpec)

	logger.Debug("makeDeployment() creating pod template spec")
	podTemplateSpec := v1.PodTemplateSpec{
		ObjectMeta: metav1.ObjectMeta{
			Name:   resourceName,
			Labels: getLabels(nvmeof, daemonID, true),
			Annotations: map[string]string{
				"config-hash": configHash,
			},
		},
		Spec: podSpec,
	}
	logger.Debugf("makeDeployment() pod template created: name=%s, config-hash=%s", resourceName, configHash)

	if hostNetwork {
		logger.Debug("makeDeployment() hostNetwork enabled, setting DNS policy again")
		podSpec.DNSPolicy = v1.DNSClusterFirstWithHostNet
	} else if r.cephClusterSpec.Network.IsMultus() {
		logger.Debug("makeDeployment() Multus network detected, applying Multus configuration")
		if err := k8sutil.ApplyMultus(r.clusterInfo.Namespace, &r.cephClusterSpec.Network, &podTemplateSpec.ObjectMeta); err != nil {
			logger.Errorf("makeDeployment() failed to apply Multus configuration: %v", err)
			return nil, err
		}
		logger.Debug("makeDeployment() Multus configuration applied successfully")
	} else {
		logger.Debug("makeDeployment() no Multus network, using default networking")
	}

	logger.Debug("makeDeployment() applying annotations and labels to pod template")
	nvmeof.Spec.Annotations.ApplyToObjectMeta(&podTemplateSpec.ObjectMeta)
	nvmeof.Spec.Labels.ApplyToObjectMeta(&podTemplateSpec.ObjectMeta)

	logger.Debug("makeDeployment() setting deployment spec")
	replicas := int32(1)
	revisionHistoryLimit := controller.RevisionHistoryLimit()
	deployment.Spec = apps.DeploymentSpec{
		RevisionHistoryLimit: revisionHistoryLimit,
		Selector:             &metav1.LabelSelector{MatchLabels: getLabels(nvmeof, daemonID, false)},
		Template:             podTemplateSpec,
		Replicas:             &replicas,
	}
	if revisionHistoryLimit != nil {
		logger.Debugf("makeDeployment() deployment spec set: replicas=%d, revisionHistoryLimit=%d",
			replicas, *revisionHistoryLimit)
	} else {
		logger.Debugf("makeDeployment() deployment spec set: replicas=%d, revisionHistoryLimit=nil",
			replicas)
	}

	logger.Debugf("makeDeployment() completed successfully: deployment=%s/%s", deployment.Namespace, deployment.Name)
	return deployment, nil
}

// createCephConfigInitContainer creates a minimal init container that generates
// /etc/ceph/ceph.conf, copies the admin keyring, and copies nvmeof.conf from the ConfigMap.
// This is needed for the gateway to connect to the Ceph cluster.
func (r *ReconcileCephNVMeOFGateway) createCephConfigInitContainer(nvmeof *cephv1.CephNVMeOFGateway, daemonID string, gatewayConfigMount v1.VolumeMount) v1.Container {
	logger.Debugf("createCephConfigInitContainer() started: gateway=%s/%s, daemonID=%s", nvmeof.Namespace, nvmeof.Name, daemonID)

	_, cephConfigMount := cephConfigVolumeAndMount()
	logger.Debugf("createCephConfigInitContainer() ceph config mount: %s", cephConfigMount.MountPath)
	logger.Debugf("createCephConfigInitContainer() gateway config mount: %s", gatewayConfigMount.MountPath)

	cephImage := r.cephClusterSpec.CephVersion.Image
	imagePullPolicy := controller.GetContainerImagePullPolicy(r.cephClusterSpec.CephVersion.ImagePullPolicy)
	logger.Debugf("createCephConfigInitContainer() container image: %s, pullPolicy: %s", cephImage, imagePullPolicy)

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
	envVars := controller.DaemonEnvVars(r.cephClusterSpec)
	envVars = append(envVars,
		v1.EnvVar{
			Name: "CEPH_MON_HOST",
			ValueFrom: &v1.EnvVarSource{
				SecretKeyRef: &v1.SecretKeySelector{
					LocalObjectReference: v1.LocalObjectReference{Name: "rook-ceph-config"},
					Key:                  "mon_host",
				},
			},
		},
		v1.EnvVar{
			Name:  "POD_NAME",
			Value: gatewayName,
		},
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
	logger.Debugf("createCephConfigInitContainer() container spec created: %d env vars, %d volume mounts",
		len(container.Env), len(container.VolumeMounts))
	logger.Debugf("createCephConfigInitContainer() completed: container name=%s", container.Name)
	return container
}

func (r *ReconcileCephNVMeOFGateway) daemonContainer(nvmeof *cephv1.CephNVMeOFGateway, cephConfigMount v1.VolumeMount) v1.Container {
	logger.Debugf("daemonContainer() started: gateway=%s/%s", nvmeof.Namespace, nvmeof.Name)
	logger.Debugf("daemonContainer() ceph config mount path: %s", cephConfigMount.MountPath)

	logger.Debug("daemonContainer() creating daemon container spec")
	privileged := true
	container := v1.Container{
		Name: "nvmeof-gateway",
		Args: []string{
			"-c",
			"/etc/ceph/nvmeof.conf",
		},

		Image:           defaultNVMeOFImage,
		ImagePullPolicy: v1.PullIfNotPresent,
		VolumeMounts: []v1.VolumeMount{
			cephConfigMount,
		},
		Env: []v1.EnvVar{
			{
				Name: "CEPH_MON_HOST",
				ValueFrom: &v1.EnvVarSource{
					SecretKeyRef: &v1.SecretKeySelector{
						LocalObjectReference: v1.LocalObjectReference{Name: "rook-ceph-config"},
						Key:                  "mon_host",
					},
				},
			},
			{
				Name:  "CEPH_ARGS",
				Value: "--mon-host $(CEPH_MON_HOST) --keyring /etc/ceph/keyring",
			},
		},

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
	logger.Debugf("daemonContainer() container spec created: image=%s, %d ports, %d env vars, %d volume mounts",
		container.Image, len(container.Ports), len(container.Env), len(container.VolumeMounts))
	ioPort, gatewayPort, monitorPort, discoveryPort := getPorts(nvmeof)
	logger.Debugf("daemonContainer() ports: io=%d, gateway=%d, monitor=%d, discovery=%d",
		ioPort, gatewayPort, monitorPort, discoveryPort)

	logger.Debug("daemonContainer() configuring liveness probe")
	if nvmeof.Spec.LivenessProbe != nil && !nvmeof.Spec.LivenessProbe.Disabled && nvmeof.Spec.LivenessProbe.Probe == nil {
		container.LivenessProbe = r.defaultLivenessProbe(nvmeof)
	}
	result := cephconfig.ConfigureLivenessProbe(container, nvmeof.Spec.LivenessProbe)
	logger.Debugf("daemonContainer() liveness probe configured: enabled=%v", result.LivenessProbe != nil)
	logger.Debugf("daemonContainer() completed: container name=%s", result.Name)
	return result
}

func (r *ReconcileCephNVMeOFGateway) defaultLivenessProbe(nvmeof *cephv1.CephNVMeOFGateway) *v1.Probe {
	ioPort, _, _, _ := getPorts(nvmeof)
	return controller.GenerateLivenessProbeTcpPort(ioPort, 10)
}

func getLabels(n *cephv1.CephNVMeOFGateway, daemonID string, includeNewLabels bool) map[string]string {
	labels := controller.CephDaemonAppLabels(
		AppName, n.Namespace, "nvmeof", n.Name+"-"+daemonID, n.Name,
		"cephnvmeofgateways.ceph.rook.io", includeNewLabels,
	)
	labels[CephNVMeOFGatewayNameLabelKey] = n.Name
	labels["instance"] = daemonID
	return labels
}

func cephConfigVolumeAndMount() (v1.Volume, v1.VolumeMount) {
	cfgDir := cephclient.DefaultConfigDir // /etc/ceph
	volName := k8sutil.PathToVolumeName(cfgDir)
	v := v1.Volume{Name: volName, VolumeSource: v1.VolumeSource{EmptyDir: &v1.EmptyDirVolumeSource{}}}
	m := v1.VolumeMount{Name: volName, MountPath: cfgDir}
	return v, m
}

func instanceName(nvmeof *cephv1.CephNVMeOFGateway, daemonID string) string {
	return fmt.Sprintf("%s-%s-%s", AppName, nvmeof.Name, daemonID)
}

func gatewayConfigVolumeAndMount(configConfigMap string) (v1.Volume, v1.VolumeMount) {
	logger.Debugf("gatewayConfigVolumeAndMount() started: configmap=%s", configConfigMap)

	// Mount to /config temporarily - init container will copy to /etc/ceph/nvmeof.conf
	cfgDir := "/config"
	cfgVolName := "gateway-config"
	configMapSource := &v1.ConfigMapVolumeSource{
		LocalObjectReference: v1.LocalObjectReference{Name: configConfigMap},
		Items:                []v1.KeyToPath{{Key: configKey, Path: "nvmeof.conf"}},
	}
	v := v1.Volume{Name: cfgVolName, VolumeSource: v1.VolumeSource{ConfigMap: configMapSource}}
	m := v1.VolumeMount{Name: cfgVolName, MountPath: cfgDir, ReadOnly: true}
	logger.Debugf("gatewayConfigVolumeAndMount() volume created: name=%s, configmap=%s, key=%s, path=nvmeof.conf, mountPath=%s",
		v.Name, configConfigMap, configKey, m.MountPath)
	logger.Debug("gatewayConfigVolumeAndMount() completed")
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
