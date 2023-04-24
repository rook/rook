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

package mgr

import (
	"fmt"
	"os"
	"strconv"

	"github.com/pkg/errors"
	cephv1 "github.com/rook/rook/pkg/apis/ceph.rook.io/v1"
	"github.com/rook/rook/pkg/daemon/ceph/client"
	"github.com/rook/rook/pkg/operator/ceph/cluster/mon"
	"github.com/rook/rook/pkg/operator/ceph/config"
	"github.com/rook/rook/pkg/operator/ceph/config/keyring"
	"github.com/rook/rook/pkg/operator/ceph/controller"
	"github.com/rook/rook/pkg/operator/k8sutil"
	apps "k8s.io/api/apps/v1"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
)

const (
	podIPEnvVar       = "ROOK_POD_IP"
	serviceMetricName = "http-metrics"
)

func (c *Cluster) makeDeployment(mgrConfig *mgrConfig) (*apps.Deployment, error) {
	logger.Debugf("mgrConfig: %+v", mgrConfig)

	volumes := controller.DaemonVolumes(mgrConfig.DataPathMap, mgrConfig.ResourceName, c.spec.DataDirHostPath)
	if c.spec.Network.IsMultus() {
		adminKeyringVol, _ := keyring.Volume().Admin(), keyring.VolumeMount().Admin()
		volumes = append(volumes, adminKeyringVol)
	}

	podSpec := v1.PodTemplateSpec{
		ObjectMeta: metav1.ObjectMeta{
			Name:   mgrConfig.ResourceName,
			Labels: c.getPodLabels(mgrConfig, true),
		},
		Spec: v1.PodSpec{
			InitContainers: []v1.Container{
				c.makeChownInitContainer(mgrConfig),
			},
			Containers: []v1.Container{
				c.makeMgrDaemonContainer(mgrConfig),
			},
			ServiceAccountName: serviceAccountName,
			RestartPolicy:      v1.RestartPolicyAlways,
			Volumes:            volumes,
			HostNetwork:        c.spec.Network.IsHost(),
			PriorityClassName:  cephv1.GetMgrPriorityClassName(c.spec.PriorityClassNames),
		},
	}
	cephv1.GetMgrPlacement(c.spec.Placement).ApplyToPodSpec(&podSpec.Spec)

	// Run the sidecar and require anti affinity only if there are multiple mgrs
	if c.spec.Mgr.Count > 1 {
		podSpec.Spec.Containers = append(podSpec.Spec.Containers, c.makeMgrSidecarContainer(mgrConfig))
		matchLabels := controller.AppLabels(AppName, c.clusterInfo.Namespace)
		podSpec.Spec.Volumes = append(podSpec.Spec.Volumes, mon.CephSecretVolume())

		// Stretch the mgrs across hosts by default, or across a bigger failure domain for stretch clusters
		topologyKey := v1.LabelHostname
		if c.spec.IsStretchCluster() {
			topologyKey = mon.StretchFailureDomainLabel(c.spec)
		}
		k8sutil.SetNodeAntiAffinityForPod(&podSpec.Spec, !c.spec.Mgr.AllowMultiplePerNode, topologyKey, matchLabels, nil)
	}

	// If the log collector is enabled we add the side-car container
	if c.spec.LogCollector.Enabled {
		shareProcessNamespace := true
		podSpec.Spec.ShareProcessNamespace = &shareProcessNamespace
		podSpec.Spec.Containers = append(podSpec.Spec.Containers, *controller.LogCollectorContainer(fmt.Sprintf("ceph-mgr.%s", mgrConfig.DaemonID), c.clusterInfo.Namespace, c.spec))
	}

	// Replace default unreachable node toleration
	k8sutil.AddUnreachableNodeToleration(&podSpec.Spec)

	if c.spec.Network.IsHost() {
		podSpec.Spec.DNSPolicy = v1.DNSClusterFirstWithHostNet
	} else if c.spec.Network.IsMultus() {
		if err := k8sutil.ApplyMultus(c.spec.Network, &podSpec.ObjectMeta); err != nil {
			return nil, err
		}
		podSpec.Spec.Containers = append(podSpec.Spec.Containers, c.makeCmdProxySidecarContainer(mgrConfig))
	}

	cephv1.GetMgrAnnotations(c.spec.Annotations).ApplyToObjectMeta(&podSpec.ObjectMeta)
	c.applyPrometheusAnnotations(&podSpec.ObjectMeta)
	cephv1.GetMgrLabels(c.spec.Labels).ApplyToObjectMeta(&podSpec.ObjectMeta)

	replicas := int32(1)

	d := &apps.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      mgrConfig.ResourceName,
			Namespace: c.clusterInfo.Namespace,
			Labels:    c.getPodLabels(mgrConfig, true),
		},
		Spec: apps.DeploymentSpec{
			Selector: &metav1.LabelSelector{
				MatchLabels: c.getPodLabels(mgrConfig, false),
			},
			Template: podSpec,
			Replicas: &replicas,
			Strategy: apps.DeploymentStrategy{
				Type: apps.RecreateDeploymentStrategyType,
			},
		},
	}
	k8sutil.AddRookVersionLabelToDeployment(d)
	cephv1.GetMgrLabels(c.spec.Labels).ApplyToObjectMeta(&d.ObjectMeta)
	controller.AddCephVersionLabelToDeployment(c.clusterInfo.CephVersion, d)
	err := c.clusterInfo.OwnerInfo.SetControllerReference(d)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to set owner reference to mgr deployment %q", d.Name)
	}
	return d, nil
}

func (c *Cluster) makeChownInitContainer(mgrConfig *mgrConfig) v1.Container {
	return controller.ChownCephDataDirsInitContainer(
		*mgrConfig.DataPathMap,
		c.spec.CephVersion.Image,
		controller.GetContainerImagePullPolicy(c.spec.CephVersion.ImagePullPolicy),
		controller.DaemonVolumeMounts(mgrConfig.DataPathMap, mgrConfig.ResourceName, c.spec.DataDirHostPath),
		cephv1.GetMgrResources(c.spec.Resources),
		controller.PodSecurityContext(),
		"",
	)
}

func (c *Cluster) makeMgrDaemonContainer(mgrConfig *mgrConfig) v1.Container {

	container := v1.Container{
		Name: "mgr",
		Command: []string{
			"ceph-mgr",
		},
		Args: append(
			controller.DaemonFlags(c.clusterInfo, &c.spec, mgrConfig.DaemonID),
			// for ceph-mgr cephfs
			// see https://github.com/ceph/ceph-csi/issues/486 for more details
			config.NewFlag("client-mount-uid", "0"),
			config.NewFlag("client-mount-gid", "0"),
			"--foreground",
		),
		Image:           c.spec.CephVersion.Image,
		ImagePullPolicy: controller.GetContainerImagePullPolicy(c.spec.CephVersion.ImagePullPolicy),
		VolumeMounts:    controller.DaemonVolumeMounts(mgrConfig.DataPathMap, mgrConfig.ResourceName, c.spec.DataDirHostPath),
		Ports: []v1.ContainerPort{
			{
				Name:          "mgr",
				ContainerPort: int32(6800),
				Protocol:      v1.ProtocolTCP,
			},
			{
				Name:          "http-metrics",
				ContainerPort: int32(DefaultMetricsPort),
				Protocol:      v1.ProtocolTCP,
			},
			{
				Name:          "dashboard",
				ContainerPort: int32(c.dashboardInternalPort()),
				Protocol:      v1.ProtocolTCP,
			},
		},
		Env: append(
			controller.DaemonEnvVars(c.spec.CephVersion.Image),
			c.cephMgrOrchestratorModuleEnvs()...,
		),
		Resources:       cephv1.GetMgrResources(c.spec.Resources),
		SecurityContext: controller.PodSecurityContext(),
		StartupProbe:    controller.GenerateStartupProbeExecDaemon(config.MgrType, mgrConfig.DaemonID),
		LivenessProbe:   controller.GenerateLivenessProbeExecDaemon(config.MgrType, mgrConfig.DaemonID),
		WorkingDir:      config.VarLogCephDir,
	}

	container = config.ConfigureStartupProbe(container, c.spec.HealthCheck.StartupProbe[cephv1.KeyMgr])
	container = config.ConfigureLivenessProbe(container, c.spec.HealthCheck.LivenessProbe[cephv1.KeyMgr])

	// If host networking is enabled, we don't need a bind addr that is different from the public addr
	if !c.spec.Network.IsHost() {
		// Opposite of the above, --public-bind-addr will *not* still advertise on the previous
		// port, which makes sense because this is the pod IP, which changes with every new pod.
		container.Args = append(container.Args,
			config.NewFlag("public-addr", controller.ContainerEnvVarReference(podIPEnvVar)))
	}

	return container
}

func (c *Cluster) makeMgrSidecarContainer(mgrConfig *mgrConfig) v1.Container {
	envVars := []v1.EnvVar{
		{Name: "ROOK_CLUSTER_ID", Value: string(c.clusterInfo.OwnerInfo.GetUID())},
		{Name: "ROOK_CLUSTER_NAME", Value: string(c.clusterInfo.NamespacedName().Name)},
		k8sutil.PodIPEnvVar(k8sutil.PrivateIPEnvVar),
		k8sutil.PodIPEnvVar(k8sutil.PublicIPEnvVar),
		mon.PodNamespaceEnvVar(c.clusterInfo.Namespace),
		mon.EndpointEnvVar(),
		mon.CephUsernameEnvVar(),
		k8sutil.ConfigOverrideEnvVar(),
		{Name: "ROOK_DASHBOARD_ENABLED", Value: strconv.FormatBool(c.spec.Dashboard.Enabled)},
		{Name: "ROOK_MONITORING_ENABLED", Value: strconv.FormatBool(c.spec.Monitoring.Enabled)},
		{Name: "ROOK_UPDATE_INTERVAL", Value: "15s"},
		{Name: "ROOK_DAEMON_NAME", Value: mgrConfig.DaemonID},
		{Name: "ROOK_CEPH_VERSION", Value: "ceph version " + c.clusterInfo.CephVersion.String()},
	}

	return v1.Container{
		Args:            []string{"ceph", "mgr", "watch-active"},
		Name:            "watch-active",
		Image:           c.rookVersion,
		ImagePullPolicy: controller.GetContainerImagePullPolicy(c.spec.CephVersion.ImagePullPolicy),
		Env:             envVars,
		Resources:       cephv1.GetMgrSidecarResources(c.spec.Resources),
		SecurityContext: controller.PrivilegedContext(true),
		VolumeMounts:    []v1.VolumeMount{mon.CephSecretVolumeMount()},
	}
}

func (c *Cluster) makeCmdProxySidecarContainer(mgrConfig *mgrConfig) v1.Container {
	_, adminKeyringVolMount := keyring.Volume().Admin(), keyring.VolumeMount().Admin()
	container := v1.Container{
		Name:            client.CommandProxyInitContainerName,
		Command:         []string{"sleep"},
		Args:            []string{"infinity"},
		Image:           c.spec.CephVersion.Image,
		ImagePullPolicy: controller.GetContainerImagePullPolicy(c.spec.CephVersion.ImagePullPolicy),
		VolumeMounts:    append(controller.DaemonVolumeMounts(mgrConfig.DataPathMap, mgrConfig.ResourceName, c.spec.DataDirHostPath), adminKeyringVolMount),
		Env:             append(controller.DaemonEnvVars(c.spec.CephVersion.Image), v1.EnvVar{Name: "CEPH_ARGS", Value: fmt.Sprintf("-m $(ROOK_CEPH_MON_HOST) -k %s", keyring.VolumeMount().AdminKeyringFilePath())}),
		Resources:       cephv1.GetMgrResources(c.spec.Resources),
		SecurityContext: controller.PodSecurityContext(),
	}

	return container
}

// MakeMetricsService generates the Kubernetes service object for the monitoring service
func (c *Cluster) MakeMetricsService(name, servicePortMetricName string) (*v1.Service, error) {

	labels := controller.AppLabels(AppName, c.clusterInfo.Namespace)
	selectorLabels := c.buildSelectorLabels(labels)

	svc := &v1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: c.clusterInfo.Namespace,
			Labels:    labels,
		},
		Spec: v1.ServiceSpec{
			Type: v1.ServiceTypeClusterIP,
			Ports: []v1.ServicePort{
				{
					Name:     servicePortMetricName,
					Port:     int32(DefaultMetricsPort),
					Protocol: v1.ProtocolTCP,
				},
			},
		},
	}

	// If the cluster is external we don't need to add the selector
	if name != controller.ExternalMgrAppName {
		svc.Spec.Selector = selectorLabels
	}

	err := c.clusterInfo.OwnerInfo.SetControllerReference(svc)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to set owner reference to monitoring service %q", svc.Name)
	}
	return svc, nil
}

func (c *Cluster) makeDashboardService(name string) (*v1.Service, error) {

	labels := controller.AppLabels(AppName, c.clusterInfo.Namespace)
	selectorLabels := c.buildSelectorLabels(labels)

	portName := "https-dashboard"
	if !c.spec.Dashboard.SSL {
		portName = "http-dashboard"
	}
	svc := &v1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      fmt.Sprintf("%s-dashboard", name),
			Namespace: c.clusterInfo.Namespace,
			Labels:    labels,
		},
		Spec: v1.ServiceSpec{
			Selector: selectorLabels,
			Type:     v1.ServiceTypeClusterIP,
			Ports: []v1.ServicePort{
				{
					Name: portName,
					Port: int32(c.dashboardPublicPort()),
					TargetPort: intstr.IntOrString{
						IntVal: int32(c.dashboardInternalPort()),
					},
					Protocol: v1.ProtocolTCP,
				},
			},
		},
	}
	err := c.clusterInfo.OwnerInfo.SetControllerReference(svc)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to set owner reference to dashboard service %q", svc.Name)
	}
	return svc, nil
}

func (c *Cluster) getPodLabels(mgrConfig *mgrConfig, includeNewLabels bool) map[string]string {
	labels := controller.CephDaemonAppLabels(AppName, c.clusterInfo.Namespace, config.MgrType, mgrConfig.DaemonID, c.clusterInfo.NamespacedName().Name, "cephclusters.ceph.rook.io", includeNewLabels)
	// leave "instance" key for legacy usage
	labels["instance"] = mgrConfig.DaemonID
	if includeNewLabels {
		// default to the active mgr label, and allow the sidecar to update if it's in standby mode
		labels[mgrRoleLabelName] = activeMgrStatus
	}
	return labels
}

func (c *Cluster) applyPrometheusAnnotations(objectMeta *metav1.ObjectMeta) {
	if len(cephv1.GetMgrAnnotations(c.spec.Annotations)) == 0 {
		t := cephv1.Annotations{
			"prometheus.io/scrape": "true",
			"prometheus.io/port":   strconv.Itoa(int(DefaultMetricsPort)),
		}

		t.ApplyToObjectMeta(objectMeta)
	}
}

func (c *Cluster) cephMgrOrchestratorModuleEnvs() []v1.EnvVar {
	operatorNamespace := os.Getenv(k8sutil.PodNamespaceEnvVar)
	envVars := []v1.EnvVar{
		{Name: "ROOK_OPERATOR_NAMESPACE", Value: operatorNamespace},
		{Name: "ROOK_CEPH_CLUSTER_CRD_VERSION", Value: cephv1.Version},
		{Name: "ROOK_CEPH_CLUSTER_CRD_NAME", Value: c.clusterInfo.NamespacedName().Name},
		k8sutil.PodIPEnvVar(podIPEnvVar),
	}
	return envVars
}

func (c *Cluster) buildSelectorLabels(labels map[string]string) map[string]string {
	selectorLabels := make(map[string]string)
	for k, v := range labels {
		selectorLabels[k] = v
	}
	selectorLabels["mgr_role"] = "active"
	return selectorLabels
}
