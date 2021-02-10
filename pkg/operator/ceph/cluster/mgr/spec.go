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
	"strings"

	"github.com/pkg/errors"
	cephv1 "github.com/rook/rook/pkg/apis/ceph.rook.io/v1"
	rookcephv1 "github.com/rook/rook/pkg/apis/ceph.rook.io/v1"
	rookv1 "github.com/rook/rook/pkg/apis/rook.io/v1"
	"github.com/rook/rook/pkg/daemon/ceph/client"
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
	podIPEnvVar               = "ROOK_POD_IP"
	serviceMetricName         = "http-metrics"
	ExternalMgrAppName        = "rook-ceph-mgr-external"
	ServiceExternalMetricName = "http-external-metrics"
)

func (c *Cluster) makeDeployment(mgrConfig *mgrConfig) (*apps.Deployment, error) {
	logger.Debugf("mgrConfig: %+v", mgrConfig)
	podSpec := v1.PodTemplateSpec{
		ObjectMeta: metav1.ObjectMeta{
			Name:   mgrConfig.ResourceName,
			Labels: c.getPodLabels(mgrConfig.DaemonID, true),
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
			Volumes:            controller.DaemonVolumes(mgrConfig.DataPathMap, mgrConfig.ResourceName),
			HostNetwork:        c.spec.Network.IsHost(),
			PriorityClassName:  cephv1.GetMgrPriorityClassName(c.spec.PriorityClassNames),
		},
	}

	// If the log collector is enabled we add the side-car container
	if c.spec.LogCollector.Enabled {
		shareProcessNamespace := true
		podSpec.Spec.ShareProcessNamespace = &shareProcessNamespace
		podSpec.Spec.Containers = append(podSpec.Spec.Containers, *controller.LogCollectorContainer(fmt.Sprintf("ceph-mgr.%s", mgrConfig.DaemonID), c.clusterInfo.Namespace, c.spec))
	}

	// Replace default unreachable node toleration
	k8sutil.AddUnreachableNodeToleration(&podSpec.Spec)

	// Only add the dashboard init container if dashboard is enabled
	if c.spec.Dashboard.Enabled {
		podSpec.Spec.InitContainers = append(podSpec.Spec.InitContainers, []v1.Container{
			c.makeSetServerAddrInitContainer(mgrConfig, "dashboard"),
		}...)
	}

	podSpec.Spec.InitContainers = append(podSpec.Spec.InitContainers, []v1.Container{
		c.makeSetServerAddrInitContainer(mgrConfig, "prometheus"),
	}...)

	// ceph config set commands want admin keyring
	podSpec.Spec.Volumes = append(podSpec.Spec.Volumes,
		keyring.Volume().Admin())
	if c.spec.Network.IsHost() {
		podSpec.Spec.DNSPolicy = v1.DNSClusterFirstWithHostNet
	} else if c.spec.Network.NetworkSpec.IsMultus() {
		if err := k8sutil.ApplyMultus(c.spec.Network.NetworkSpec, &podSpec.ObjectMeta); err != nil {
			return nil, err
		}
	}

	cephv1.GetMgrAnnotations(c.spec.Annotations).ApplyToObjectMeta(&podSpec.ObjectMeta)
	c.applyPrometheusAnnotations(&podSpec.ObjectMeta)
	cephv1.GetMgrLabels(c.spec.Labels).ApplyToObjectMeta(&podSpec.ObjectMeta)
	cephv1.GetMgrPlacement(c.spec.Placement).ApplyToPodSpec(&podSpec.Spec, true)

	replicas := int32(1)

	d := &apps.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      mgrConfig.ResourceName,
			Namespace: c.clusterInfo.Namespace,
			Labels:    c.getPodLabels(mgrConfig.DaemonID, true),
		},
		Spec: apps.DeploymentSpec{
			Selector: &metav1.LabelSelector{
				MatchLabels: c.getPodLabels(mgrConfig.DaemonID, false),
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
	k8sutil.SetOwnerRef(&d.ObjectMeta, &c.clusterInfo.OwnerRef)
	return d, nil
}

// if we do not need the http bind fix, then we need to be careful. if we are
// upgrading from a cluster that had the fix applied, then the fix is no longer
// needed, and furthermore, needs to be removed so that there is not a lingering
// ceph configuration option that contains an old ip.  by clearing the option,
// we let ceph bind to its default ANYADDR address.  However, since we don't
// know which version of Ceph we are may be upgrading _from_ we need to (a)
// always do this and (b) make sure that all forms of the configuration option
// are removed (see the init container factory method). Once the minimum
// supported version of Rook contains the fix, all of this can be removed.
func (c *Cluster) clearHTTPBindFix() error {
	// We only need to apply these changes once. No harm in once each time the operator restarts.
	if c.appliedHttpBind {
		return nil
	}
	for _, daemonID := range c.getDaemonIDs() {
		for _, module := range []string{"dashboard", "prometheus"} {
			// there are two forms of the configuration key that might exist which
			// depends not on the current version, but on the version that may be
			// the version being upgraded from.
			if _, err := client.MgrSetConfig(c.context, c.clusterInfo, daemonID,
				fmt.Sprintf("mgr/%s/server_addr", module), "", false); err != nil {
				return errors.Wrap(err, "failed to set config for an mgr daemon using v2 format")
			}

			// this is for the format used in v1.0
			// https://github.com/rook/rook/commit/11d318fb2f77a6ac9a8f2b9be42c826d3b4a93c3
			if _, err := client.MgrSetConfig(c.context, c.clusterInfo, daemonID,
				fmt.Sprintf("mgr/%s/%s/server_addr", module, daemonID), "", false); err != nil {
				return errors.Wrap(err, "failed to set config for an mgr daemon using v1 format")
			}
		}
	}
	c.appliedHttpBind = true
	return nil
}

func (c *Cluster) makeChownInitContainer(mgrConfig *mgrConfig) v1.Container {
	return controller.ChownCephDataDirsInitContainer(
		*mgrConfig.DataPathMap,
		c.spec.CephVersion.Image,
		controller.DaemonVolumeMounts(mgrConfig.DataPathMap, mgrConfig.ResourceName),
		cephv1.GetMgrResources(c.spec.Resources),
		controller.PodSecurityContext(),
	)
}

func (c *Cluster) makeSetServerAddrInitContainer(mgrConfig *mgrConfig, mgrModule string) v1.Container {
	// Commands produced for various Ceph major versions (differences highlighted)
	//  L: config-key set       mgr/<mod>/server_addr $(ROOK_CEPH_<MOD>_SERVER_ADDR)
	//  M: config     set mgr.a mgr/<mod>/server_addr $(ROOK_CEPH_<MOD>_SERVER_ADDR)
	//  N: config     set mgr.a mgr/<mod>/server_addr $(ROOK_CEPH_<MOD>_SERVER_ADDR) --force
	cfgSetArgs := []string{"config", "set"}
	cfgSetArgs = append(cfgSetArgs, fmt.Sprintf("mgr.%s", mgrConfig.DaemonID))
	cfgPath := fmt.Sprintf("mgr/%s/%s/server_addr", mgrModule, mgrConfig.DaemonID)
	cfgSetArgs = append(cfgSetArgs, cfgPath, "0.0.0.0")
	cfgSetArgs = append(cfgSetArgs, "--force")
	cfgSetArgs = append(cfgSetArgs, "--verbose")

	container := v1.Container{
		Name: "init-set-" + strings.ToLower(mgrModule) + "-server-addr",
		Command: []string{
			"ceph",
		},
		Args: append(
			controller.AdminFlags(c.clusterInfo),
			cfgSetArgs...,
		),
		Image: c.spec.CephVersion.Image,
		VolumeMounts: append(
			controller.DaemonVolumeMounts(mgrConfig.DataPathMap, mgrConfig.ResourceName),
			keyring.VolumeMount().Admin(),
		),
		Env: append(
			controller.DaemonEnvVars(c.spec.CephVersion.Image),
			c.cephMgrOrchestratorModuleEnvs()...,
		),
		Resources: cephv1.GetMgrResources(c.spec.Resources),
	}
	return container
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
		Image:        c.spec.CephVersion.Image,
		VolumeMounts: controller.DaemonVolumeMounts(mgrConfig.DataPathMap, mgrConfig.ResourceName),
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
				ContainerPort: int32(c.dashboardPort()),
				Protocol:      v1.ProtocolTCP,
			},
		},
		Env: append(
			controller.DaemonEnvVars(c.spec.CephVersion.Image),
			c.cephMgrOrchestratorModuleEnvs()...,
		),
		Resources:       cephv1.GetMgrResources(c.spec.Resources),
		SecurityContext: controller.PodSecurityContext(),
		LivenessProbe:   getDefaultMgrLivenessProbe(),
	}

	// If the liveness probe is enabled
	container = config.ConfigureLivenessProbe(rookcephv1.KeyMgr, container, c.spec.HealthCheck)

	// If host networking is enabled, we don't need a bind addr that is different from the public addr
	if !c.spec.Network.IsHost() {
		// Opposite of the above, --public-bind-addr will *not* still advertise on the previous
		// port, which makes sense because this is the pod IP, which changes with every new pod.
		container.Args = append(container.Args,
			config.NewFlag("public-addr", controller.ContainerEnvVarReference(podIPEnvVar)))
	}

	return container
}

func getDefaultMgrLivenessProbe() *v1.Probe {
	return &v1.Probe{
		Handler: v1.Handler{
			HTTPGet: &v1.HTTPGetAction{
				Path: "/",
				Port: intstr.FromInt(int(DefaultMetricsPort)),
			},
		},
		InitialDelaySeconds: 60,
	}
}

// MakeMetricsService generates the Kubernetes service object for the monitoring service
func (c *Cluster) MakeMetricsService(name, servicePortMetricName string) *v1.Service {
	labels := controller.AppLabels(AppName, c.clusterInfo.Namespace)
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
	if name != ExternalMgrAppName {
		svc.Spec.Selector = labels
	}

	k8sutil.SetOwnerRef(&svc.ObjectMeta, &c.clusterInfo.OwnerRef)
	return svc
}

func (c *Cluster) makeDashboardService(name string) *v1.Service {
	labels := controller.AppLabels(AppName, c.clusterInfo.Namespace)
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
			Selector: labels,
			Type:     v1.ServiceTypeClusterIP,
			Ports: []v1.ServicePort{
				{
					Name:     portName,
					Port:     int32(c.dashboardPort()),
					Protocol: v1.ProtocolTCP,
				},
			},
		},
	}
	k8sutil.SetOwnerRef(&svc.ObjectMeta, &c.clusterInfo.OwnerRef)
	return svc
}

func (c *Cluster) getPodLabels(daemonName string, includeNewLabels bool) map[string]string {
	labels := controller.CephDaemonAppLabels(AppName, c.clusterInfo.Namespace, "mgr", daemonName, includeNewLabels)
	// leave "instance" key for legacy usage
	labels["instance"] = daemonName
	return labels
}

func (c *Cluster) applyPrometheusAnnotations(objectMeta *metav1.ObjectMeta) {
	if len(cephv1.GetMgrAnnotations(c.spec.Annotations)) == 0 {
		t := rookv1.Annotations{
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
		{Name: "ROOK_CEPH_CLUSTER_CRD_VERSION", Value: rookcephv1.Version},
		{Name: "ROOK_CEPH_CLUSTER_CRD_NAME", Value: c.clusterInfo.NamespacedName().Name},
		k8sutil.PodIPEnvVar(podIPEnvVar),
	}
	return envVars
}

// CreateExternalMetricsEndpoints creates external metric endpoint
func CreateExternalMetricsEndpoints(namespace string, monitoringSpec cephv1.MonitoringSpec, ownerRef metav1.OwnerReference) *v1.Endpoints {
	labels := controller.AppLabels(AppName, namespace)

	endpoints := &v1.Endpoints{
		ObjectMeta: metav1.ObjectMeta{
			Name:      ExternalMgrAppName,
			Namespace: namespace,
			Labels:    labels,
		},
		Subsets: []v1.EndpointSubset{
			{
				Addresses: monitoringSpec.ExternalMgrEndpoints,
				Ports: []v1.EndpointPort{
					{
						Name:     ServiceExternalMetricName,
						Port:     int32(monitoringSpec.ExternalMgrPrometheusPort),
						Protocol: v1.ProtocolTCP,
					},
				},
			},
		},
	}

	k8sutil.SetOwnerRef(&endpoints.ObjectMeta, &ownerRef)
	return endpoints
}
