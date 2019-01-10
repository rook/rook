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

	mgrdaemon "github.com/rook/rook/pkg/daemon/ceph/mgr"
	opmon "github.com/rook/rook/pkg/operator/ceph/cluster/mon"
	opspec "github.com/rook/rook/pkg/operator/ceph/spec"
	"github.com/rook/rook/pkg/operator/k8sutil"
	"k8s.io/api/core/v1"
	extensions "k8s.io/api/extensions/v1beta1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	mgrDaemonCommand = "ceph-mgr"
)

func (c *Cluster) makeDeployment(mgrConfig *mgrConfig, port int) *extensions.Deployment {
	podSpec := v1.PodTemplateSpec{
		ObjectMeta: metav1.ObjectMeta{
			Name:   mgrConfig.ResourceName,
			Labels: c.getPodLabels(mgrConfig.DaemonName),
			Annotations: map[string]string{"prometheus.io/scrape": "true",
				"prometheus.io/port": strconv.Itoa(metricsPort)},
		},
		Spec: v1.PodSpec{
			InitContainers: []v1.Container{
				// Config file init performed by Rook
				c.makeConfigInitContainer(mgrConfig),
			},
			Containers: []v1.Container{
				c.makeMgrDaemonContainer(mgrConfig, port),
			},
			ServiceAccountName: serviceAccountName,
			RestartPolicy:      v1.RestartPolicyAlways,
			Volumes:            opspec.PodVolumes(""),
			HostNetwork:        c.HostNetwork,
		},
	}
	if c.HostNetwork {
		podSpec.Spec.DNSPolicy = v1.DNSClusterFirstWithHostNet
	}
	c.placement.ApplyToPodSpec(&podSpec.Spec)

	replicas := int32(1)
	d := &extensions.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      mgrConfig.ResourceName,
			Namespace: c.Namespace,
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

func (c *Cluster) makeConfigInitContainer(mgrConfig *mgrConfig) v1.Container {
	return v1.Container{
		Name: opspec.ConfigInitContainerName,
		Args: []string{
			"ceph",
			mgrdaemon.InitCommand,
			fmt.Sprintf("--config-dir=%s", k8sutil.DataDir),
			fmt.Sprintf("--mgr-name=%s", mgrConfig.DaemonName),
		},
		Image: k8sutil.MakeRookImage(c.rookVersion),
		Env: []v1.EnvVar{
			// Set '--mgr-keyring' flag with an env var sourced from the secret
			{Name: "ROOK_MGR_KEYRING",
				ValueFrom: &v1.EnvVarSource{
					SecretKeyRef: &v1.SecretKeySelector{
						LocalObjectReference: v1.LocalObjectReference{Name: mgrConfig.ResourceName},
						Key:                  opspec.KeyringSecretKeyName,
					}}},
			k8sutil.PodIPEnvVar(k8sutil.PrivateIPEnvVar),
			k8sutil.PodIPEnvVar(k8sutil.PublicIPEnvVar),
			k8sutil.PodIPEnvVar("ROOK_MGR_MODULE_SERVER_ADDR"),
			{Name: "ROOK_CEPH_VERSION_NAME", Value: c.cephVersion.Name},
			opmon.EndpointEnvVar(),
			opmon.SecretEnvVar(),
			opmon.AdminSecretEnvVar(),
			k8sutil.ConfigOverrideEnvVar(),
		},
		VolumeMounts: opspec.RookVolumeMounts(),
		// config file creation does not require ports to be open
		Resources: c.resources,
	}
}

func (c *Cluster) makeMgrDaemonContainer(mgrConfig *mgrConfig, port int) v1.Container {
	container := v1.Container{
		Name: "mgr",
		Command: []string{
			mgrDaemonCommand,
		},
		Args: []string{
			"--foreground",
			"--id", mgrConfig.DaemonName,
			// do not add the '--cluster/--conf/--keyring' flags; rook wants their default values
		},
		Image:        c.cephVersion.Image,
		VolumeMounts: opspec.CephVolumeMounts(),
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
				ContainerPort: int32(port),
				Protocol:      v1.ProtocolTCP,
			},
		},
		Env:       k8sutil.ClusterDaemonEnvVars(),
		Resources: c.resources,
	}
	container.Env = append(container.Env, opmon.ClusterNameEnvVar(c.Namespace))
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
