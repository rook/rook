/*
Copyright 2016 The Rook Authors. All rights reserved.

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

package mon

import (
	"fmt"
	"net"
	"os"

	"github.com/rook/rook/pkg/operator/ceph/config"

	opspec "github.com/rook/rook/pkg/operator/ceph/spec"
	"github.com/rook/rook/pkg/operator/k8sutil"
	apps "k8s.io/api/apps/v1"
	"k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/kubernetes/pkg/kubelet/apis"
)

const (
	// Full path of command used to invoke the monmap tool
	monmaptoolCommand = "/usr/bin/monmaptool"
	// Full path of the command used to invoke the Ceph mon daemon
	cephMonCommand = "ceph-mon"

	monmapFile = "monmap"
)

func (c *Cluster) getLabels(daemonName string) map[string]string {
	// Mons have a service for each mon, so the additional pod data is relevant for its services
	// Use pod labels to keep "mon: id" for legacy
	labels := opspec.PodLabels(appName, c.Namespace, "mon", daemonName)
	// Add "mon_cluster: <namespace>" for legacy
	labels[monClusterAttr] = c.Namespace
	return labels
}

func (c *Cluster) makeDeployment(monConfig *monConfig, hostname string) *apps.Deployment {
	d := &apps.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      monConfig.ResourceName,
			Namespace: c.Namespace,
			Labels:    c.getLabels(monConfig.DaemonName),
		},
	}
	k8sutil.SetOwnerRef(c.context.Clientset, c.Namespace, &d.ObjectMeta, &c.ownerRef)

	pod := c.makeMonPod(monConfig, hostname)
	replicaCount := int32(1)
	d.Spec = apps.DeploymentSpec{
		Selector: &metav1.LabelSelector{
			MatchLabels: c.getLabels(monConfig.DaemonName),
		},
		Template: v1.PodTemplateSpec{
			ObjectMeta: pod.ObjectMeta,
			Spec:       pod.Spec,
		},
		Replicas: &replicaCount,
		Strategy: apps.DeploymentStrategy{
			Type: apps.RecreateDeploymentStrategyType,
		},
	}

	return d
}

/*
 * Pod spec
 */

func (c *Cluster) makeMonPod(monConfig *monConfig, hostname string) *v1.Pod {
	logger.Debug("monConfig: %+v", monConfig)
	podSpec := v1.PodSpec{
		InitContainers: []v1.Container{
			c.makeMonFSInitContainer(monConfig),
		},
		Containers: []v1.Container{
			c.makeMonDaemonContainer(monConfig),
		},
		RestartPolicy: v1.RestartPolicyAlways,
		NodeSelector:  map[string]string{apis.LabelHostname: hostname},
		Volumes:       opspec.DaemonVolumes(monConfig.DataPathMap, keyringStoreName),
		HostNetwork:   c.HostNetwork,
	}
	if c.HostNetwork {
		podSpec.DNSPolicy = v1.DNSClusterFirstWithHostNet
	}
	c.placement.ApplyToPodSpec(&podSpec)
	// remove Pod (anti-)affinity because we have our own placement logic
	c.placement.PodAffinity = nil
	c.placement.PodAntiAffinity = nil

	pod := &v1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:        monConfig.ResourceName,
			Namespace:   c.Namespace,
			Labels:      c.getLabels(monConfig.DaemonName),
			Annotations: map[string]string{},
		},
		Spec: podSpec,
	}

	return pod
}

/*
 * Container specs
 */

// Init and daemon containers require the same context, so we call it 'pod' context
func podSecurityContext() *v1.SecurityContext {
	privileged := false
	if os.Getenv("ROOK_HOSTPATH_REQUIRES_PRIVILEGED") == "true" {
		privileged = true
	}
	return &v1.SecurityContext{Privileged: &privileged}
}

func (c *Cluster) makeMonFSInitContainer(monConfig *monConfig) v1.Container {
	return v1.Container{
		Name: "init-mon-fs",
		Command: []string{
			cephMonCommand,
		},
		Args: append(
			opspec.DaemonFlags(c.clusterInfo, config.MonType, monConfig.DaemonName),
			"--mkfs",
		),
		Image:           c.cephVersion.Image,
		VolumeMounts:    opspec.DaemonVolumeMounts(monConfig.DataPathMap, keyringStoreName),
		SecurityContext: podSecurityContext(),
		// filesystem creation does not require ports to be exposed
		Env:       opspec.DaemonEnvVars(),
		Resources: c.resources,
	}
}

func (c *Cluster) makeMonDaemonContainer(monConfig *monConfig) v1.Container {
	return v1.Container{
		Name: "mon",
		Command: []string{
			cephMonCommand,
		},
		Args: append(
			opspec.DaemonFlags(c.clusterInfo, config.MonType, monConfig.DaemonName),
			"--foreground",
			config.NewFlag("public-addr", joinHostPort(monConfig.PublicIP, monConfig.Port)),
			config.NewFlag("public-bind-addr", joinHostPort("$(ROOK_PRIVATE_IP)", monConfig.Port)),
		),
		Image:           c.cephVersion.Image,
		VolumeMounts:    opspec.DaemonVolumeMounts(monConfig.DataPathMap, keyringStoreName),
		SecurityContext: podSecurityContext(),
		Ports: []v1.ContainerPort{
			{
				Name:          "client",
				ContainerPort: monConfig.Port,
				Protocol:      v1.ProtocolTCP,
			},
		},
		Env: append(
			opspec.DaemonEnvVars(),
			k8sutil.PodIPEnvVar(k8sutil.PrivateIPEnvVar),
		),
		Resources: c.resources,
	}
}

func joinHostPort(host string, port int32) string {
	return net.JoinHostPort(host, fmt.Sprintf("%d", port))
}
