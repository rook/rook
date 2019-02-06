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
	"strconv"
	"strings"

	cephv1 "github.com/rook/rook/pkg/apis/ceph.rook.io/v1"
	"github.com/rook/rook/pkg/operator/ceph/config"
	"github.com/rook/rook/pkg/operator/ceph/config/keyring"
	opspec "github.com/rook/rook/pkg/operator/ceph/spec"
	"github.com/rook/rook/pkg/operator/k8sutil"
	"k8s.io/api/core/v1"
	extensions "k8s.io/api/extensions/v1beta1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func (c *Cluster) makeDeployment(mgrConfig *mgrConfig) *extensions.Deployment {
	podSpec := v1.PodTemplateSpec{
		ObjectMeta: metav1.ObjectMeta{
			Name:   mgrConfig.ResourceName,
			Labels: c.getPodLabels(mgrConfig.DaemonID),
			Annotations: map[string]string{"prometheus.io/scrape": "true",
				"prometheus.io/port": strconv.Itoa(metricsPort)},
		},
		Spec: v1.PodSpec{
			InitContainers: []v1.Container{
				c.makeSetServerAddrInitContainer(mgrConfig, "dashboard"),
				c.makeSetServerAddrInitContainer(mgrConfig, "prometheus"),
			},
			Containers: []v1.Container{
				c.makeMgrDaemonContainer(mgrConfig),
			},
			ServiceAccountName: serviceAccountName,
			RestartPolicy:      v1.RestartPolicyAlways,
			Volumes: append(
				opspec.DaemonVolumes(mgrConfig.DataPathMap, mgrConfig.ResourceName),
				keyring.Volume().Admin(), // ceph config set commands want admin keyring
			),
			HostNetwork: c.HostNetwork,
		},
	}
	if c.HostNetwork {
		podSpec.Spec.DNSPolicy = v1.DNSClusterFirstWithHostNet
	}
	c.placement.ApplyToPodSpec(&podSpec.Spec)
	if c.cephVersion.Name == cephv1.Luminous || c.cephVersion.Name == "" {
		// prepend the keyring-copy workaround for luminous clusters
		podSpec.Spec.InitContainers = append(
			[]v1.Container{c.makeCopyKeyringInitContainer(mgrConfig)},
			podSpec.Spec.InitContainers...)
	}

	replicas := int32(1)
	d := &extensions.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      mgrConfig.ResourceName,
			Namespace: c.Namespace,
			Labels:    c.getPodLabels(mgrConfig.DaemonID),
		},
		Spec: extensions.DeploymentSpec{
			Template: podSpec,
			Replicas: &replicas,
			Strategy: extensions.DeploymentStrategy{
				Type: extensions.RecreateDeploymentStrategyType,
			},
		},
	}
	k8sutil.SetOwnerRef(c.context.Clientset, c.Namespace, &d.ObjectMeta, &c.ownerRef)
	return d
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
	envVarName := fmt.Sprintf("ROOK_CEPH_%s_SERVER_ADDR", strings.ToUpper(mgrModule))
	envVarReference := fmt.Sprintf("$(%s)", envVarName)
	cfgSetArgs := []string{"config", "set"}
	if c.cephVersion.Name == cephv1.Luminous || c.cephVersion.Name == "" {
		cfgSetArgs[0] = "config-key"
	} else {
		cfgSetArgs = append(cfgSetArgs, fmt.Sprintf("mgr.%s", mgrConfig.DaemonID))
	}
	cfgPath := fmt.Sprintf("mgr/%s/server_addr", mgrModule)
	cfgSetArgs = append(cfgSetArgs, cfgPath, envVarReference)
	if cephv1.VersionAtLeast(c.cephVersion.Name, cephv1.Nautilus) {
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
			opspec.DaemonEnvVars(),
			k8sutil.PodIPEnvVar(envVarName),
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
			opspec.DaemonFlags(c.clusterInfo, config.MgrType, mgrConfig.DaemonID),
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
		Env:       opspec.DaemonEnvVars(),
		Resources: c.resources,
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

	k8sutil.SetOwnerRef(c.context.Clientset, c.Namespace, &svc.ObjectMeta, &c.ownerRef)
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
	k8sutil.SetOwnerRef(c.context.Clientset, c.Namespace, &svc.ObjectMeta, &c.ownerRef)
	return svc
}

func (c *Cluster) getPodLabels(daemonName string) map[string]string {
	labels := opspec.PodLabels(appName, c.Namespace, "mgr", daemonName)
	// leave "instance" key for legacy usage
	labels["instance"] = daemonName
	return labels
}
