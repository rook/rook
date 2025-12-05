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
	"bytes"
	_ "embed"
	"fmt"
	"text/template"

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

//go:embed connectionconfig.sh
var connectionConfigScript string

type connectionConfigTemplate struct {
	PoolName string
}

func (r *ReconcileCephNVMeOFGateway) generateCephNVMeOFService(nvmeof *cephv1.CephNVMeOFGateway, daemonID string) *v1.Service {
	logger.Debugf("generateCephNVMeOFService() started: gateway=%s/%s, daemonID=%s",
		nvmeof.Namespace, nvmeof.Name, daemonID)

	logger.Debug("generateCephNVMeOFService() generating labels")
	labels := getLabels(nvmeof, daemonID, true)
	logger.Debugf("generateCephNVMeOFService() labels generated: %v", labels)

	serviceName := instanceName(nvmeof, daemonID)
	logger.Debugf("generateCephNVMeOFService() service name: %s", serviceName)

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
	logger.Infof("ceph nvmeof gateway service running at %s:%d", svc.Spec.ClusterIP, nvmeofDiscoveryPort)
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
	gatewayConfigVol, _ := gatewayConfigVolumeAndMount(configMapName)
	logger.Debugf("makeDeployment() gateway config volume: %s", gatewayConfigVol.Name)

	logger.Debug("makeDeployment() creating pod spec")
	logger.Debug("makeDeployment() creating init container spec")
	initContainer := r.connectionConfigInitContainer(nvmeof, configMapName)
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

func renderConnectionConfig(poolName string) (string, error) {
	var writer bytes.Buffer
	t := template.New("connection-config")
	t, err := t.Parse(connectionConfigScript)
	if err != nil {
		return "", errors.Wrapf(err, "failed to parse connection config template")
	}

	config := connectionConfigTemplate{
		PoolName: poolName,
	}

	if err := t.Execute(&writer, config); err != nil {
		return "", errors.Wrapf(err, "failed to render connection config template")
	}

	return writer.String(), nil
}

func (r *ReconcileCephNVMeOFGateway) connectionConfigInitContainer(nvmeof *cephv1.CephNVMeOFGateway, configMapName string) v1.Container {
	logger.Debugf("connectionConfigInitContainer() started: gateway=%s/%s, configMapName=%s", nvmeof.Namespace, nvmeof.Name, configMapName)

	_, cephConfigMount := cephConfigVolumeAndMount()
	logger.Debugf("connectionConfigInitContainer() ceph config mount: %s", cephConfigMount.MountPath)
	_, gatewayConfigMount := gatewayConfigVolumeAndMount(configMapName)
	logger.Debugf("connectionConfigInitContainer() gateway config mount: %s", gatewayConfigMount.MountPath)

	poolName := nvmeof.Spec.Pool
	logger.Debugf("connectionConfigInitContainer() using pool: %s", poolName)

	logger.Debug("connectionConfigInitContainer() generating init script")
	script, err := renderConnectionConfig(poolName)
	if err != nil {
		logger.Errorf("connectionConfigInitContainer() failed to render script: %v", err)
		script = fmt.Sprintf(`echo "Failed to render connection config script: %v" && exit 1`, err)
	}
	logger.Debugf("connectionConfigInitContainer() init script generated (length: %d bytes)", len(script))

	logger.Debug("connectionConfigInitContainer() creating container spec")
	cephImage := r.cephClusterSpec.CephVersion.Image
	imagePullPolicy := controller.GetContainerImagePullPolicy(r.cephClusterSpec.CephVersion.ImagePullPolicy)
	logger.Debugf("connectionConfigInitContainer() container image: %s, pullPolicy: %s", cephImage, imagePullPolicy)

	container := v1.Container{
		Name:            "generate-minimal-ceph-conf",
		Image:           cephImage,
		ImagePullPolicy: imagePullPolicy,
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
	logger.Debugf("connectionConfigInitContainer() container spec created: %d env vars, %d volume mounts",
		len(container.Env), len(container.VolumeMounts))
	logger.Debugf("connectionConfigInitContainer() completed: container name=%s", container.Name)
	return container
}

func (r *ReconcileCephNVMeOFGateway) daemonContainer(nvmeof *cephv1.CephNVMeOFGateway, cephConfigMount v1.VolumeMount) v1.Container {
	logger.Debugf("daemonContainer() started: gateway=%s/%s", nvmeof.Namespace, nvmeof.Name)
	logger.Debugf("daemonContainer() ceph config mount path: %s", cephConfigMount.MountPath)

	logger.Debug("daemonContainer() creating daemon container spec")
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
	}
	logger.Debugf("daemonContainer() container spec created: image=%s, %d ports, %d env vars, %d volume mounts",
		container.Image, len(container.Ports), len(container.Env), len(container.VolumeMounts))
	logger.Debugf("daemonContainer() ports: io=%d, gateway=%d, monitor=%d, discovery=%d",
		nvmeofIOPort, nvmeofGatewayPort, nvmeofMonitorPort, nvmeofDiscoveryPort)

	logger.Debug("daemonContainer() configuring liveness probe")
	if nvmeof.Spec.LivenessProbe != nil && !nvmeof.Spec.LivenessProbe.Disabled && nvmeof.Spec.LivenessProbe.Probe == nil {
		container.LivenessProbe = r.defaultLivenessProbe()
	}
	result := cephconfig.ConfigureLivenessProbe(container, nvmeof.Spec.LivenessProbe)
	logger.Debugf("daemonContainer() liveness probe configured: enabled=%v", result.LivenessProbe != nil)
	logger.Debugf("daemonContainer() completed: container name=%s", result.Name)
	return result
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

func gatewayConfigVolumeAndMount(configConfigMap string) (v1.Volume, v1.VolumeMount) {
	logger.Debugf("gatewayConfigVolumeAndMount() started: configmap=%s", configConfigMap)

	cfgDir := "/config"
	cfgVolName := "gateway-config"
	configMapSource := &v1.ConfigMapVolumeSource{
		LocalObjectReference: v1.LocalObjectReference{Name: configConfigMap},
		Items:                []v1.KeyToPath{{Key: configKey, Path: "nvmeof.conf"}},
	}
	v := v1.Volume{Name: cfgVolName, VolumeSource: v1.VolumeSource{ConfigMap: configMapSource}}
	m := v1.VolumeMount{Name: cfgVolName, MountPath: cfgDir}
	logger.Debugf("gatewayConfigVolumeAndMount() volume created: name=%s, configmap=%s, key=%s, path=nvmeof.conf, mountPath=%s",
		v.Name, configConfigMap, configKey, m.MountPath)
	logger.Debug("gatewayConfigVolumeAndMount() completed")
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
