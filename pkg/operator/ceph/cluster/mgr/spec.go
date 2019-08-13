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

	rookcephv1 "github.com/rook/rook/pkg/apis/ceph.rook.io/v1"
	"github.com/rook/rook/pkg/daemon/ceph/client"
	"github.com/rook/rook/pkg/operator/ceph/cluster/mon"
	"github.com/rook/rook/pkg/operator/ceph/config"
	"github.com/rook/rook/pkg/operator/ceph/config/keyring"
	opspec "github.com/rook/rook/pkg/operator/ceph/spec"
	cephver "github.com/rook/rook/pkg/operator/ceph/version"
	"github.com/rook/rook/pkg/operator/k8sutil"
	rookversion "github.com/rook/rook/pkg/version"
	apps "k8s.io/api/apps/v1"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
)

func (c *Cluster) makeDeployment(mgrConfig *mgrConfig) *apps.Deployment {
	podSpec := v1.PodTemplateSpec{
		ObjectMeta: metav1.ObjectMeta{
			Name:   mgrConfig.ResourceName,
			Labels: c.getPodLabels(mgrConfig.DaemonID),
		},
		Spec: v1.PodSpec{
			InitContainers: []v1.Container{},
			Containers: []v1.Container{
				c.makeMgrDaemonContainer(mgrConfig),
			},
			ServiceAccountName: serviceAccountName,
			RestartPolicy:      v1.RestartPolicyAlways,
			Volumes:            opspec.DaemonVolumes(mgrConfig.DataPathMap, mgrConfig.ResourceName),
			HostNetwork:        c.HostNetwork,
		},
	}

	// if the fix is needed, then the following init containers are created
	// which explicitly configure the server_addr Ceph configuration option to
	// be equal to the pod's IP address. Note that when the fix is not needed,
	// there is additional work done to clear fixes after upgrades. See
	// clearHttpBindFix() method for more details.
	if c.needHttpBindFix() {
		podSpec.Spec.InitContainers = []v1.Container{
			c.makeSetServerAddrInitContainer(mgrConfig, "dashboard"),
			c.makeSetServerAddrInitContainer(mgrConfig, "prometheus"),
		}
		// ceph config set commands want admin keyring
		podSpec.Spec.Volumes = append(podSpec.Spec.Volumes,
			keyring.Volume().Admin())
	}

	if c.HostNetwork {
		podSpec.Spec.DNSPolicy = v1.DNSClusterFirstWithHostNet
	}
	c.annotations.ApplyToObjectMeta(&podSpec.ObjectMeta)
	c.placement.ApplyToPodSpec(&podSpec.Spec)

	replicas := int32(1)
	if len(c.annotations) == 0 {
		prometheusAnnotations := map[string]string{
			"prometheus.io/scrape": "true",
			"prometheus.io/port":   strconv.Itoa(metricsPort),
		}
		podSpec.ObjectMeta.Annotations = prometheusAnnotations
	}
	d := &apps.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      mgrConfig.ResourceName,
			Namespace: c.Namespace,
			Labels:    c.getPodLabels(mgrConfig.DaemonID),
		},
		Spec: apps.DeploymentSpec{
			Selector: &metav1.LabelSelector{
				MatchLabels: c.getPodLabels(mgrConfig.DaemonID),
			},
			Template: podSpec,
			Replicas: &replicas,
			Strategy: apps.DeploymentStrategy{
				Type: apps.RecreateDeploymentStrategyType,
			},
		},
	}
	k8sutil.AddRookVersionLabelToDeployment(d)
	opspec.AddCephVersionLabelToDeployment(c.clusterInfo.CephVersion, d)
	k8sutil.SetOwnerRef(&d.ObjectMeta, &c.ownerRef)
	return d
}

func (c *Cluster) needHttpBindFix() bool {
	needed := true

	// if mimic and >= 13.2.6
	if c.clusterInfo.CephVersion.IsMimic() &&
		c.clusterInfo.CephVersion.IsAtLeast(cephver.CephVersion{Major: 13, Minor: 2, Extra: 6}) {
		needed = false
	}

	// if >= 14.1.1
	if c.clusterInfo.CephVersion.IsAtLeast(cephver.CephVersion{Major: 14, Minor: 1, Extra: 1}) {
		needed = false
	}

	return needed
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
func (c *Cluster) clearHttpBindFix(mgrConfig *mgrConfig) {
	for _, module := range []string{"dashboard", "prometheus"} {
		// there are two forms of the configuration key that might exist which
		// depends not on the current version, but on the version that may be
		// the version being upgraded from.
		for _, ver := range []cephver.CephVersion{cephver.Mimic} {
			changed, err := client.MgrSetConfig(c.context, c.Namespace, mgrConfig.DaemonID, ver,
				fmt.Sprintf("mgr/%s/server_addr", module), "", false)
			logger.Infof("clearing http bind fix mod=%s ver=%s changed=%t err=%+v", module, &ver, changed, err)

			// this is for the format used in v1.0
			// https://github.com/rook/rook/commit/11d318fb2f77a6ac9a8f2b9be42c826d3b4a93c3
			changed, err = client.MgrSetConfig(c.context, c.Namespace, mgrConfig.DaemonID, ver,
				fmt.Sprintf("mgr/%s/%s/server_addr", module, mgrConfig.DaemonID), "", false)
			logger.Infof("clearing http bind fix mod=%s ver=%s changed=%t err=%+v", module, &ver, changed, err)
		}
	}
}

func (c *Cluster) makeCopyKeyringInitContainer(mgrConfig *mgrConfig) v1.Container {
	// mgr does not obey `--keyring=/etc/ceph/keyring-store/keyring` flag, and always reports:
	// "unable to find a keyring on /var/lib/ceph/mgr/ceph-a/keyring: (2) No such file or directory"
	// Workaround: copy the keyring to the container data dir
	container := v1.Container{
		Name: "init-copy-keyring-to-data-dir",
		Command: []string{
			"cp",
		},
		Args: []string{
			"--verbose",
			"--preserve=all",
			"--force", // overwrite existing keyring if exists
			keyring.VolumeMount().KeyringFilePath(),
			mgrConfig.DataPathMap.ContainerDataDir,
		},
		Image:        c.cephVersion.Image,
		VolumeMounts: opspec.DaemonVolumeMounts(mgrConfig.DataPathMap, mgrConfig.ResourceName),
		// no env vars needed
		Resources: c.resources,
	}
	return container
}

func (c *Cluster) makeSetServerAddrInitContainer(mgrConfig *mgrConfig, mgrModule string) v1.Container {
	// Commands produced for various Ceph major versions (differences highlighted)
	//  L: config-key set       mgr/<mod>/server_addr $(ROOK_CEPH_<MOD>_SERVER_ADDR)
	//  M: config     set mgr.a mgr/<mod>/server_addr $(ROOK_CEPH_<MOD>_SERVER_ADDR)
	//  N: config     set mgr.a mgr/<mod>/server_addr $(ROOK_CEPH_<MOD>_SERVER_ADDR) --force
	podIPEnvVar := "ROOK_POD_IP"
	cfgSetArgs := []string{"config", "set"}
	cfgSetArgs = append(cfgSetArgs, fmt.Sprintf("mgr.%s", mgrConfig.DaemonID))
	cfgPath := fmt.Sprintf("mgr/%s/%s/server_addr", mgrModule, mgrConfig.DaemonID)
	cfgSetArgs = append(cfgSetArgs, cfgPath, opspec.ContainerEnvVarReference(podIPEnvVar))
	if c.clusterInfo.CephVersion.IsAtLeastNautilus() {
		cfgSetArgs = append(cfgSetArgs, "--force")
	}
	cfgSetArgs = append(cfgSetArgs, "--verbose")

	container := v1.Container{
		Name: "init-set-" + strings.ToLower(mgrModule) + "-server-addr",
		Command: []string{
			"ceph",
		},
		Args: append(
			opspec.AdminFlags(c.clusterInfo),
			cfgSetArgs...,
		),
		Image: c.cephVersion.Image,
		VolumeMounts: append(
			opspec.DaemonVolumeMounts(mgrConfig.DataPathMap, mgrConfig.ResourceName),
			keyring.VolumeMount().Admin(),
		),
		Env: append(
			append(
				opspec.DaemonEnvVars(c.cephVersion.Image),
				k8sutil.PodIPEnvVar(podIPEnvVar),
			),
			c.cephMgrOrchestratorModuleEnvs()...,
		),
		Resources: c.resources,
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
			opspec.DaemonFlags(c.clusterInfo, mgrConfig.DaemonID),
			// for ceph-mgr cephfs
			// see https://github.com/ceph/ceph-csi/issues/486 for more details
			config.NewFlag("client-mount-uid", "0"),
			config.NewFlag("client-mount-gid", "0"),
			"--foreground",
		),
		Image:        c.cephVersion.Image,
		VolumeMounts: opspec.DaemonVolumeMounts(mgrConfig.DataPathMap, mgrConfig.ResourceName),
		Ports: []v1.ContainerPort{
			{
				Name:          "mgr",
				ContainerPort: int32(6800),
				Protocol:      v1.ProtocolTCP,
			},
			{
				Name:          "http-metrics",
				ContainerPort: int32(metricsPort),
				Protocol:      v1.ProtocolTCP,
			},
			{
				Name:          "dashboard",
				ContainerPort: int32(mgrConfig.DashboardPort),
				Protocol:      v1.ProtocolTCP,
			},
		},
		Env: append(
			opspec.DaemonEnvVars(c.cephVersion.Image),
			c.cephMgrOrchestratorModuleEnvs()...,
		),
		Resources: c.resources,
		LivenessProbe: &v1.Probe{
			Handler: v1.Handler{
				HTTPGet: &v1.HTTPGetAction{
					Path: "/",
					Port: intstr.FromInt(metricsPort),
				},
			},
			InitialDelaySeconds: 60,
		},
		Lifecycle:       opspec.PodLifeCycle(""),
		SecurityContext: mon.PodSecurityContext(),
	}
	return container
}

func (c *Cluster) makeMetricsService(name string) *v1.Service {
	labels := opspec.AppLabels(appName, c.Namespace)
	svc := &v1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: c.Namespace,
			Labels:    labels,
		},
		Spec: v1.ServiceSpec{
			Selector: labels,
			Type:     v1.ServiceTypeClusterIP,
			Ports: []v1.ServicePort{
				{
					Name:     "http-metrics",
					Port:     int32(metricsPort),
					Protocol: v1.ProtocolTCP,
				},
			},
		},
	}

	k8sutil.SetOwnerRef(&svc.ObjectMeta, &c.ownerRef)
	return svc
}

func (c *Cluster) makeDashboardService(name string, port int) *v1.Service {
	labels := opspec.AppLabels(appName, c.Namespace)
	svc := &v1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      fmt.Sprintf("%s-dashboard", name),
			Namespace: c.Namespace,
			Labels:    labels,
		},
		Spec: v1.ServiceSpec{
			Selector: labels,
			Type:     v1.ServiceTypeClusterIP,
			Ports: []v1.ServicePort{
				{
					Name:     "https-dashboard",
					Port:     int32(port),
					Protocol: v1.ProtocolTCP,
				},
			},
		},
	}
	k8sutil.SetOwnerRef(&svc.ObjectMeta, &c.ownerRef)
	return svc
}

func (c *Cluster) getPodLabels(daemonName string) map[string]string {
	labels := opspec.PodLabels(appName, c.Namespace, "mgr", daemonName)
	// leave "instance" key for legacy usage
	labels["instance"] = daemonName
	return labels
}

func (c *Cluster) cephMgrOrchestratorModuleEnvs() []v1.EnvVar {
	operatorNamespace := os.Getenv(k8sutil.PodNamespaceEnvVar)
	envVars := []v1.EnvVar{
		{Name: "ROOK_OPERATOR_NAMESPACE", Value: operatorNamespace},
		{Name: "ROOK_CEPH_CLUSTER_CRD_VERSION", Value: rookcephv1.Version},
		{Name: "ROOK_VERSION", Value: rookversion.Version},
		{Name: "ROOK_CEPH_CLUSTER_CRD_NAME", Value: c.clusterInfo.Name},
	}
	return envVars
}
