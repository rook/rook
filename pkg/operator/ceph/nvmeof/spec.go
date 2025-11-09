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
	"fmt"

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
	// AppName is the name of the app
	AppName             = "rook-ceph-nvmeof"
	nvmeofIOPort        = 4420
	nvmeofGatewayPort   = 5500
	nvmeofMonitorPort   = 5499
	nvmeofDiscoveryPort = 8009
	defaultNVMeOFImage  = "quay.io/ceph/nvmeof:1.5"
	configKey           = "config"
	serviceAccountName  = "ceph-nvmeof-gateway"
)

func (r *ReconcileCephNVMeOFGateway) generateCephNVMeOFService(nvmeof *cephv1.CephNVMeOFGateway, daemonID string) *v1.Service {
	labels := getLabels(nvmeof, daemonID, true)

	svc := &v1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      instanceName(nvmeof, daemonID),
			Namespace: nvmeof.Namespace,
			Labels:    labels,
		},
		Spec: v1.ServiceSpec{
			Selector: labels,
			Ports: []v1.ServicePort{
				{
					Name:       "io",
					Port:       nvmeofIOPort,
					TargetPort: intstr.FromInt(int(nvmeofIOPort)),
					Protocol:   v1.ProtocolTCP,
				},
				{
					Name:       "gateway",
					Port:       nvmeofGatewayPort,
					TargetPort: intstr.FromInt(int(nvmeofGatewayPort)),
					Protocol:   v1.ProtocolTCP,
				},
				{
					Name:       "monitor",
					Port:       nvmeofMonitorPort,
					TargetPort: intstr.FromInt(int(nvmeofMonitorPort)),
					Protocol:   v1.ProtocolTCP,
				},
				{
					Name:       "discovery",
					Port:       nvmeofDiscoveryPort,
					TargetPort: intstr.FromInt(int(nvmeofDiscoveryPort)),
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

	// Set owner ref to the parent object
	err := controllerutil.SetControllerReference(nvmeof, s, r.scheme)
	if err != nil {
		return errors.Wrapf(err, "failed to set owner reference to ceph nvmeof gateway %q", s)
	}

	svc, err := r.context.Clientset.CoreV1().Services(nvmeof.Namespace).Create(r.opManagerContext, s, metav1.CreateOptions{})
	if err != nil {
		if !kerrors.IsAlreadyExists(err) {
			return errors.Wrap(err, "failed to create nvmeof gateway service")
		}
		logger.Infof("ceph nvmeof gateway service already created")
		return nil
	}

	logger.Infof("ceph nvmeof gateway service running at %s:%d", svc.Spec.ClusterIP, nvmeofDiscoveryPort)
	return nil
}

func (r *ReconcileCephNVMeOFGateway) makeDeployment(nvmeof *cephv1.CephNVMeOFGateway, daemonID, configHash string) (*apps.Deployment, error) {
	resourceName := instanceName(nvmeof, daemonID)

	deployment := &apps.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      resourceName,
			Namespace: nvmeof.Namespace,
			Labels:    getLabels(nvmeof, daemonID, true),
		},
	}

	hostNetwork := nvmeof.IsHostNetwork(r.cephClusterSpec)

	k8sutil.AddRookVersionLabelToDeployment(deployment)
	controller.AddCephVersionLabelToDeployment(r.clusterInfo.CephVersion, deployment)
	nvmeof.Spec.Annotations.ApplyToObjectMeta(&deployment.ObjectMeta)
	nvmeof.Spec.Labels.ApplyToObjectMeta(&deployment.ObjectMeta)

	cephConfigVol, cephConfigMount := cephConfigVolumeAndMount()
	gatewayConfigVol, _ := gatewayConfigVolumeAndMount(nvmeof)

	podSpec := v1.PodSpec{
		InitContainers: []v1.Container{
			r.connectionConfigInitContainer(nvmeof),
		},
		Containers: []v1.Container{
			r.daemonContainer(nvmeof, cephConfigMount),
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

	k8sutil.AddUnreachableNodeToleration(&podSpec)

	if hostNetwork {
		podSpec.DNSPolicy = v1.DNSClusterFirstWithHostNet
	}
	nvmeof.Spec.Placement.ApplyToPodSpec(&podSpec)

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

	if hostNetwork {
		podSpec.DNSPolicy = v1.DNSClusterFirstWithHostNet
	} else if r.cephClusterSpec.Network.IsMultus() {
		if err := k8sutil.ApplyMultus(r.clusterInfo.Namespace, &r.cephClusterSpec.Network, &podTemplateSpec.ObjectMeta); err != nil {
			return nil, err
		}
	}

	nvmeof.Spec.Annotations.ApplyToObjectMeta(&podTemplateSpec.ObjectMeta)
	nvmeof.Spec.Labels.ApplyToObjectMeta(&podTemplateSpec.ObjectMeta)

	replicas := int32(1)
	deployment.Spec = apps.DeploymentSpec{
		RevisionHistoryLimit: controller.RevisionHistoryLimit(),
		Selector:             &metav1.LabelSelector{MatchLabels: getLabels(nvmeof, daemonID, false)},
		Template:             podTemplateSpec,
		Replicas:             &replicas,
	}

	return deployment, nil
}

func (r *ReconcileCephNVMeOFGateway) connectionConfigInitContainer(nvmeof *cephv1.CephNVMeOFGateway) v1.Container {
	_, cephConfigMount := cephConfigVolumeAndMount()
	_, gatewayConfigMount := gatewayConfigVolumeAndMount(nvmeof)

	poolName := nvmeof.Spec.Pool
	if poolName == "" {
		poolName = "nvmeofpool"
	}

	script := fmt.Sprintf(`
set -xEeuo pipefail

cat << EOF > /etc/ceph/ceph.conf
[global]
mon_host = $(CEPH_MON_HOST)
log_to_stderr = true
keyring = /etc/ceph/keyring
EOF

cp /tmp/ceph/keyring /etc/ceph

chmod 444 /etc/ceph/ceph.conf
chmod 440 /etc/ceph/keyring

sed -e "s/@@POD_NAME@@/${POD_NAME}/g" \
    -e "s/@@ANA_GROUP@@/${ANA_GROUP}/g" \
    -e "s/@@POD_IP@@/${POD_IP}/g" \
    < /config/ceph-nvmeof.conf > /etc/ceph/nvmeof.conf

ceph nvme-gw create ${POD_NAME} %s ${ANA_GROUP}
ceph nvme-gw show %s ${ANA_GROUP}
`, poolName, poolName)

	container := v1.Container{
		Name:            "generate-minimal-ceph-conf",
		Image:           r.cephClusterSpec.CephVersion.Image,
		ImagePullPolicy: controller.GetContainerImagePullPolicy(r.cephClusterSpec.CephVersion.ImagePullPolicy),
		Command: []string{
			"/bin/bash",
			"-c",
			script,
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
				Name: "POD_NAME",
				ValueFrom: &v1.EnvVarSource{
					FieldRef: &v1.ObjectFieldSelector{
						FieldPath: "metadata.name",
					},
				},
			},
			{
				Name: "ANA_GROUP",
				ValueFrom: &v1.EnvVarSource{
					FieldRef: &v1.ObjectFieldSelector{
						FieldPath: "metadata.namespace",
					},
				},
			},
			{
				Name: "POD_IP",
				ValueFrom: &v1.EnvVarSource{
					FieldRef: &v1.ObjectFieldSelector{
						FieldPath: "status.podIP",
					},
				},
			},
		},
		VolumeMounts: []v1.VolumeMount{
			adminKeyringVolumeMount(),
			gatewayConfigMount,
			cephConfigMount,
		},
		Resources:       nvmeof.Spec.Resources,
		SecurityContext: privilegedSecurityContext(),
	}

	return container
}

func (r *ReconcileCephNVMeOFGateway) daemonContainer(nvmeof *cephv1.CephNVMeOFGateway, cephConfigMount v1.VolumeMount) v1.Container {
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

		Ports: []v1.ContainerPort{
			{
				Name:          "io",
				ContainerPort: nvmeofIOPort,
				Protocol:      v1.ProtocolTCP,
			},
			{
				Name:          "gateway",
				ContainerPort: nvmeofGatewayPort,
				Protocol:      v1.ProtocolTCP,
			},
			{
				Name:          "monitor",
				ContainerPort: nvmeofMonitorPort,
				Protocol:      v1.ProtocolTCP,
			},
			{
				Name:          "discovery",
				ContainerPort: nvmeofDiscoveryPort,
				Protocol:      v1.ProtocolTCP,
			},
		},

		Resources:       nvmeof.Spec.Resources,
		SecurityContext: privilegedSecurityContext(),
		LivenessProbe:   r.defaultLivenessProbe(),
	}

	return cephconfig.ConfigureLivenessProbe(container, nvmeof.Spec.LivenessProbe)
}

func (r *ReconcileCephNVMeOFGateway) defaultLivenessProbe() *v1.Probe {
	return controller.GenerateLivenessProbeTcpPort(nvmeofIOPort, 10)
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

func gatewayConfigVolumeAndMount(nvmeof *cephv1.CephNVMeOFGateway) (v1.Volume, v1.VolumeMount) {
	configMapName := nvmeof.Spec.ConfigMapRef
	if configMapName == "" {
		configMapName = fmt.Sprintf("rook-ceph-nvmeof-%s-config", nvmeof.Name)
	}

	v := v1.Volume{
		Name: "gateway-config",
		VolumeSource: v1.VolumeSource{
			Projected: &v1.ProjectedVolumeSource{
				DefaultMode: func() *int32 { mode := int32(420); return &mode }(),
				Sources: []v1.VolumeProjection{
					{
						ConfigMap: &v1.ConfigMapProjection{
							LocalObjectReference: v1.LocalObjectReference{
								Name: configMapName,
							},
							Items: []v1.KeyToPath{
								{
									Key:  configKey,
									Mode: func() *int32 { mode := int32(420); return &mode }(),
									Path: "ceph-nvmeof.conf",
								},
							},
						},
					},
				},
			},
		},
	}
	m := v1.VolumeMount{
		Name:      "gateway-config",
		MountPath: "/config",
		ReadOnly:  true,
	}
	return v, m
}

func privilegedSecurityContext() *v1.SecurityContext {
	privileged := true
	return &v1.SecurityContext{
		Privileged: &privileged,
		Capabilities: &v1.Capabilities{
			Add: []v1.Capability{
				"SYS_ADMIN",
			},
			Drop: []v1.Capability{
				"ALL",
			},
		},
	}
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
