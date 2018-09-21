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
	"path"

	mondaemon "github.com/rook/rook/pkg/daemon/ceph/mon"
	opspec "github.com/rook/rook/pkg/operator/ceph/spec"
	"github.com/rook/rook/pkg/operator/k8sutil"
	"k8s.io/api/core/v1"
	extensions "k8s.io/api/extensions/v1beta1"
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

func (c *Cluster) getLabels(name string) map[string]string {
	return map[string]string{
		k8sutil.AppAttr: appName,
		"mon":           name,
		monClusterAttr:  c.Namespace,
	}
}

func (c *Cluster) makeDeployment(monConfig *monConfig, hostname string) *extensions.Deployment {
	d := &extensions.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      monConfig.ResourceName,
			Namespace: c.Namespace,
			Labels:    c.getLabels(monConfig.DaemonName),
		},
	}
	k8sutil.SetOwnerRef(c.context.Clientset, c.Namespace, &d.ObjectMeta, &c.ownerRef)

	pod := c.makeMonPod(monConfig, hostname)
	replicaCount := int32(1)
	d.Spec = extensions.DeploymentSpec{
		Template: v1.PodTemplateSpec{
			ObjectMeta: pod.ObjectMeta,
			Spec:       pod.Spec,
		},
		Replicas: &replicaCount,
		Strategy: extensions.DeploymentStrategy{
			Type: extensions.RecreateDeploymentStrategyType,
		},
	}

	return d
}

/*
 * Pod spec
 */

func (c *Cluster) makeMonPod(monConfig *monConfig, hostname string) *v1.Pod {
	podSpec := v1.PodSpec{
		InitContainers: []v1.Container{
			// Config file init performed by Rook
			c.makeConfigInitContainer(monConfig),
			// Ceph monmap init performed by 'monmaptool'
			c.makeMonmapInitContainer(monConfig),
			// mon filesystem init performed by mon daemon
			c.makeMonFSInitContainer(monConfig),
		},
		Containers: []v1.Container{
			c.makeMonDaemonContainer(monConfig),
		},
		RestartPolicy: v1.RestartPolicyAlways,
		NodeSelector:  map[string]string{apis.LabelHostname: hostname},
		Volumes:       opspec.PodVolumes(c.dataDirHostPath),
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

func (c *Cluster) makeConfigInitContainer(monConfig *monConfig) v1.Container {
	return v1.Container{
		Name: opspec.ConfigInitContainerName,
		Args: []string{
			"ceph",
			mondaemon.InitCommand,
			fmt.Sprintf("--config-dir=%s", k8sutil.DataDir),
			fmt.Sprintf("--name=%s", monConfig.DaemonName),
			fmt.Sprintf("--port=%d", monConfig.Port),
			fmt.Sprintf("--fsid=%s", c.clusterInfo.FSID),
		},
		Image: k8sutil.MakeRookImage(c.Version),
		Env: []v1.EnvVar{
			k8sutil.PodIPEnvVar(k8sutil.PrivateIPEnvVar),
			{Name: k8sutil.PublicIPEnvVar, Value: monConfig.PublicIP},
			ClusterNameEnvVar(c.Namespace),
			EndpointEnvVar(),
			SecretEnvVar(),
			AdminSecretEnvVar(),
			k8sutil.ConfigOverrideEnvVar(),
		},
		VolumeMounts:    opspec.RookVolumeMounts(),
		SecurityContext: podSecurityContext(),
		Resources:       c.resources,
	}
}

func (c *Cluster) monmapFilePath(monConfig *monConfig) string {
	return path.Join(
		mondaemon.GetMonRunDirPath(c.context.ConfigDir, monConfig.DaemonName),
		monmapFile,
	)
}

func (c *Cluster) makeMonmapInitContainer(monConfig *monConfig) v1.Container {
	// Add mons w/ monmaptool w/ args: [--add <mon-name> <mon-endpoint>]...
	monmapAddMonArgs := []string{}
	for _, mon := range c.clusterInfo.Monitors {
		monmapAddMonArgs = append(monmapAddMonArgs, "--add", mon.Name, mon.Endpoint)
	}

	return v1.Container{
		Name: "monmap-init",
		Command: []string{
			monmaptoolCommand,
		},
		Args: append(
			[]string{
				c.monmapFilePath(monConfig),
				"--create",
				"--clobber",
				"--fsid", c.clusterInfo.FSID,
			},
			monmapAddMonArgs...,
		),
		Image:           k8sutil.MakeRookImage(c.Version), // TODO: ceph:<vers> image
		VolumeMounts:    opspec.CephVolumeMounts(),
		SecurityContext: podSecurityContext(),
		// monmap creation does not require ports to be exposed
		Resources: c.resources,
	}
}

// args needed for all ceph-mon calls
func (c *Cluster) cephMonCommonArgs(monConfig *monConfig) []string {
	return []string{
		"--name", fmt.Sprintf("mon.%s", monConfig.DaemonName),
		"--mon-data", mondaemon.GetMonDataDirPath(c.context.ConfigDir, monConfig.DaemonName),
	}
}

func (c *Cluster) makeMonFSInitContainer(monConfig *monConfig) v1.Container {
	return v1.Container{
		Name: "mon-fs-init",
		Command: []string{
			cephMonCommand,
		},
		Args: append(
			[]string{
				"--mkfs",
				"--monmap", c.monmapFilePath(monConfig),
			},
			c.cephMonCommonArgs(monConfig)...,
		),
		Image:           k8sutil.MakeRookImage(c.Version), // TODO: ceph:<vers> image
		VolumeMounts:    opspec.CephVolumeMounts(),
		SecurityContext: podSecurityContext(),
		// filesystem creation does not require ports to be exposed
		Resources: c.resources,
	}
}

func (c *Cluster) makeMonDaemonContainer(monConfig *monConfig) v1.Container {
	return v1.Container{
		// The operator has set up the mon's service already, so the IP that the mon should
		// broadcast as its own (--public-addr) is known. But the pod's IP, which the mon should
		// bind to (--public-bind-addr) isn't known until runtime. 3 solutions were considered for
		// resolving this issue:
		// 1. Rook in config init sets "public_bind_addr" in the Ceph config file
		//    - Chosen solution, but is not as transparent to inspection as using commandline arg
		// 2. Use bash to do variable substitution with the pod IP env var; but bash is a poor PID1
		// 3. Use tini to do var substitution as above; but tini doesn't exist in the ceph images.
		Name: "mon",
		Command: []string{
			cephMonCommand,
		},
		Args: append(
			[]string{
				"--foreground",
				"--public-addr", joinHostPort(monConfig.PublicIP, monConfig.Port),
				// --public-bind-addr is set in the config file at init time
			},
			c.cephMonCommonArgs(monConfig)...,
		),
		Image:           k8sutil.MakeRookImage(c.Version), // TODO: ceph:<vers> image
		VolumeMounts:    opspec.CephVolumeMounts(),
		SecurityContext: podSecurityContext(),
		Ports: []v1.ContainerPort{
			{
				Name:          "client",
				ContainerPort: monConfig.Port,
				Protocol:      v1.ProtocolTCP,
			},
		},
		Resources: c.resources,
	}
}

func joinHostPort(host string, port int32) string {
	return net.JoinHostPort(host, fmt.Sprintf("%d", port))
}
