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

package mds

import (
	"fmt"

	"github.com/pkg/errors"
	"github.com/rook/rook/pkg/clusterd"
	"github.com/rook/rook/pkg/operator/ceph/cluster/mon"
	opspec "github.com/rook/rook/pkg/operator/ceph/spec"
	"github.com/rook/rook/pkg/operator/k8sutil"
	apps "k8s.io/api/apps/v1"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	mdsDaemonCommand = "ceph-mds"
	// MDS cache memory limit should be set to 50-60% of RAM reserved for the MDS container
	// MDS uses approximately 125% of the value of mds_cache_memory_limit in RAM.
	// Eventually we will tune this automatically: http://tracker.ceph.com/issues/36663
	mdsCacheMemoryLimitFactor = 0.5
)

func (c *Cluster) makeDeployment(mdsConfig *mdsConfig) *apps.Deployment {
	podSpec := v1.PodTemplateSpec{
		ObjectMeta: metav1.ObjectMeta{
			Name:   mdsConfig.ResourceName,
			Labels: c.podLabels(mdsConfig),
		},
		Spec: v1.PodSpec{
			InitContainers: []v1.Container{
				c.makeChownInitContainer(mdsConfig),
			},
			Containers: []v1.Container{
				c.makeMdsDaemonContainer(mdsConfig),
			},
			RestartPolicy:     v1.RestartPolicyAlways,
			Volumes:           opspec.DaemonVolumes(mdsConfig.DataPathMap, mdsConfig.ResourceName),
			HostNetwork:       c.clusterSpec.Network.IsHost(),
			PriorityClassName: c.fs.Spec.MetadataServer.PriorityClassName,
		},
	}
	// Replace default unreachable node toleration
	k8sutil.AddUnreachableNodeToleration(&podSpec.Spec)

	if c.clusterSpec.Network.IsHost() {
		podSpec.Spec.DNSPolicy = v1.DNSClusterFirstWithHostNet
	}
	c.fs.Spec.MetadataServer.Annotations.ApplyToObjectMeta(&podSpec.ObjectMeta)
	c.fs.Spec.MetadataServer.Placement.ApplyToPodSpec(&podSpec.Spec)

	replicas := int32(1)
	d := &apps.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      mdsConfig.ResourceName,
			Namespace: c.fs.Namespace,
			Labels:    c.podLabels(mdsConfig),
		},
		Spec: apps.DeploymentSpec{
			Selector: &metav1.LabelSelector{
				MatchLabels: podSpec.Labels,
			},
			Template: podSpec,
			Replicas: &replicas,
			Strategy: apps.DeploymentStrategy{
				Type: apps.RecreateDeploymentStrategyType,
			},
		},
	}
	k8sutil.AddRookVersionLabelToDeployment(d)
	c.fs.Spec.MetadataServer.Annotations.ApplyToObjectMeta(&d.ObjectMeta)
	opspec.AddCephVersionLabelToDeployment(c.clusterInfo.CephVersion, d)
	k8sutil.SetOwnerRef(&d.ObjectMeta, &c.ownerRef)
	return d
}

func (c *Cluster) makeChownInitContainer(mdsConfig *mdsConfig) v1.Container {
	return opspec.ChownCephDataDirsInitContainer(
		*mdsConfig.DataPathMap,
		c.clusterSpec.CephVersion.Image,
		opspec.DaemonVolumeMounts(mdsConfig.DataPathMap, mdsConfig.ResourceName),
		c.fs.Spec.MetadataServer.Resources,
		mon.PodSecurityContext(),
	)
}

func (c *Cluster) makeMdsDaemonContainer(mdsConfig *mdsConfig) v1.Container {
	args := append(
		opspec.DaemonFlags(c.clusterInfo, mdsConfig.DaemonID),
		"--foreground",
	)

	container := v1.Container{
		Name: "mds",
		Command: []string{
			"ceph-mds",
		},
		Args:         args,
		Image:        c.clusterSpec.CephVersion.Image,
		VolumeMounts: opspec.DaemonVolumeMounts(mdsConfig.DataPathMap, mdsConfig.ResourceName),
		Env: append(
			opspec.DaemonEnvVars(c.clusterSpec.CephVersion.Image),
		),
		Resources:       c.fs.Spec.MetadataServer.Resources,
		SecurityContext: mon.PodSecurityContext(),
	}

	return container
}

func (c *Cluster) podLabels(mdsConfig *mdsConfig) map[string]string {
	labels := opspec.PodLabels(AppName, c.fs.Namespace, "mds", mdsConfig.DaemonID)
	labels["rook_file_system"] = c.fs.Name
	return labels
}

func getMdsDeployments(context *clusterd.Context, namespace, fsName string) (*apps.DeploymentList, error) {
	fsLabelSelector := fmt.Sprintf("rook_file_system=%s", fsName)
	deps, err := k8sutil.GetDeployments(context.Clientset, namespace, fsLabelSelector)
	if err != nil {
		return nil, errors.Wrapf(err, "could not get deployments for filesystem %s (matching label selector %q)", fsName, fsLabelSelector)
	}
	return deps, nil
}

func deleteMdsDeployment(context *clusterd.Context, namespace string, deployment *apps.Deployment) error {
	// Delete the mds deployment
	logger.Infof("deleting mds deployment %s", deployment.Name)
	var gracePeriod int64
	propagation := metav1.DeletePropagationForeground
	options := &metav1.DeleteOptions{GracePeriodSeconds: &gracePeriod, PropagationPolicy: &propagation}
	if err := context.Clientset.AppsV1().Deployments(namespace).Delete(deployment.GetName(), options); err != nil {
		return errors.Wrapf(err, "failed to delete mds deployment %s", deployment.GetName())
	}
	return nil
}
