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
	"context"
	"fmt"

	"github.com/pkg/errors"
	cephv1 "github.com/rook/rook/pkg/apis/ceph.rook.io/v1"
	"github.com/rook/rook/pkg/clusterd"
	"github.com/rook/rook/pkg/operator/ceph/config"
	"github.com/rook/rook/pkg/operator/ceph/controller"
	"github.com/rook/rook/pkg/operator/k8sutil"
	apps "k8s.io/api/apps/v1"
	v1 "k8s.io/api/core/v1"
	kerrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	podIPEnvVar = "ROOK_POD_IP"
	// MDS cache memory limit should be set to 50-60% of RAM reserved for the MDS container
	// MDS uses approximately 125% of the value of mds_cache_memory_limit in RAM.
	// Eventually we will tune this automatically: http://tracker.ceph.com/issues/36663
	mdsCacheMemoryLimitFactor = 0.5
)

func (c *Cluster) makeDeployment(mdsConfig *mdsConfig, namespace string) (*apps.Deployment, error) {

	mdsContainer := c.makeMdsDaemonContainer(mdsConfig)
	mdsContainer = config.ConfigureLivenessProbe(cephv1.KeyMds, mdsContainer, c.clusterSpec.HealthCheck)

	podSpec := v1.PodTemplateSpec{
		ObjectMeta: metav1.ObjectMeta{
			Name:      mdsConfig.ResourceName,
			Namespace: namespace,
			Labels:    c.podLabels(mdsConfig, true),
		},
		Spec: v1.PodSpec{
			InitContainers: []v1.Container{
				c.makeChownInitContainer(mdsConfig),
			},
			Containers: []v1.Container{
				mdsContainer,
			},
			RestartPolicy:     v1.RestartPolicyAlways,
			Volumes:           controller.DaemonVolumes(mdsConfig.DataPathMap, mdsConfig.ResourceName),
			HostNetwork:       c.clusterSpec.Network.IsHost(),
			PriorityClassName: c.fs.Spec.MetadataServer.PriorityClassName,
		},
	}

	// Replace default unreachable node toleration
	k8sutil.AddUnreachableNodeToleration(&podSpec.Spec)

	// If the log collector is enabled we add the side-car container
	if c.clusterSpec.LogCollector.Enabled {
		shareProcessNamespace := true
		podSpec.Spec.ShareProcessNamespace = &shareProcessNamespace
		podSpec.Spec.Containers = append(podSpec.Spec.Containers, *controller.LogCollectorContainer(fmt.Sprintf("ceph-mds.%s", mdsConfig.DaemonID), c.clusterInfo.Namespace, *c.clusterSpec))
	}

	c.fs.Spec.MetadataServer.Annotations.ApplyToObjectMeta(&podSpec.ObjectMeta)
	c.fs.Spec.MetadataServer.Labels.ApplyToObjectMeta(&podSpec.ObjectMeta)
	c.fs.Spec.MetadataServer.Placement.ApplyToPodSpec(&podSpec.Spec)

	replicas := int32(1)
	d := &apps.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      mdsConfig.ResourceName,
			Namespace: c.fs.Namespace,
			Labels:    c.podLabels(mdsConfig, true),
		},
		Spec: apps.DeploymentSpec{
			Selector: &metav1.LabelSelector{
				MatchLabels: c.podLabels(mdsConfig, false),
			},
			Template: podSpec,
			Replicas: &replicas,
			Strategy: apps.DeploymentStrategy{
				Type: apps.RecreateDeploymentStrategyType,
			},
		},
	}

	if c.clusterSpec.Network.IsHost() {
		d.Spec.Template.Spec.DNSPolicy = v1.DNSClusterFirstWithHostNet
	} else if c.clusterSpec.Network.NetworkSpec.IsMultus() {
		if err := k8sutil.ApplyMultus(c.clusterSpec.Network.NetworkSpec, &podSpec.ObjectMeta); err != nil {
			return nil, err
		}
	}

	k8sutil.AddRookVersionLabelToDeployment(d)
	c.fs.Spec.MetadataServer.Annotations.ApplyToObjectMeta(&d.ObjectMeta)
	c.fs.Spec.MetadataServer.Labels.ApplyToObjectMeta(&d.ObjectMeta)
	controller.AddCephVersionLabelToDeployment(c.clusterInfo.CephVersion, d)

	return d, nil
}

func (c *Cluster) makeChownInitContainer(mdsConfig *mdsConfig) v1.Container {
	return controller.ChownCephDataDirsInitContainer(
		*mdsConfig.DataPathMap,
		c.clusterSpec.CephVersion.Image,
		controller.DaemonVolumeMounts(mdsConfig.DataPathMap, mdsConfig.ResourceName),
		c.fs.Spec.MetadataServer.Resources,
		controller.PodSecurityContext(),
	)
}

func (c *Cluster) makeMdsDaemonContainer(mdsConfig *mdsConfig) v1.Container {
	args := append(
		controller.DaemonFlags(c.clusterInfo, c.clusterSpec, mdsConfig.DaemonID),
		"--foreground",
	)

	if !c.clusterSpec.Network.IsHost() {
		args = append(args,
			config.NewFlag("public-addr", controller.ContainerEnvVarReference(podIPEnvVar)))
	}

	container := v1.Container{
		Name: "mds",
		Command: []string{
			"ceph-mds",
		},
		Args:            args,
		Image:           c.clusterSpec.CephVersion.Image,
		VolumeMounts:    controller.DaemonVolumeMounts(mdsConfig.DataPathMap, mdsConfig.ResourceName),
		Env:             append(controller.DaemonEnvVars(c.clusterSpec.CephVersion.Image), k8sutil.PodIPEnvVar(podIPEnvVar)),
		Resources:       c.fs.Spec.MetadataServer.Resources,
		SecurityContext: controller.PodSecurityContext(),
		LivenessProbe:   controller.GenerateLivenessProbeExecDaemon(config.MdsType, mdsConfig.DaemonID),
		WorkingDir:      config.VarLogCephDir,
	}

	return container
}

func (c *Cluster) podLabels(mdsConfig *mdsConfig, includeNewLabels bool) map[string]string {
	labels := controller.CephDaemonAppLabels(AppName, c.fs.Namespace, "mds", mdsConfig.DaemonID, includeNewLabels)
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

func deleteMdsDeployment(clusterdContext *clusterd.Context, namespace string, deployment *apps.Deployment) error {
	ctx := context.TODO()
	// Delete the mds deployment
	logger.Infof("deleting mds deployment %s", deployment.Name)
	var gracePeriod int64
	propagation := metav1.DeletePropagationForeground
	options := &metav1.DeleteOptions{GracePeriodSeconds: &gracePeriod, PropagationPolicy: &propagation}
	if err := clusterdContext.Clientset.AppsV1().Deployments(namespace).Delete(ctx, deployment.GetName(), *options); err != nil {
		return errors.Wrapf(err, "failed to delete mds deployment %s", deployment.GetName())
	}
	return nil
}

func scaleMdsDeployment(clusterdContext *clusterd.Context, namespace string, deployment *apps.Deployment, replicas int32) error {
	ctx := context.TODO()
	// scale mds deployment
	logger.Infof("scaling mds deployment %s to %d replicas", deployment.Name, replicas)
	d, err := clusterdContext.Clientset.AppsV1().Deployments(namespace).Get(ctx, deployment.GetName(), metav1.GetOptions{})
	if err != nil {
		if replicas != 0 && kerrors.IsNotFound(err) {
			return errors.Wrapf(err, "failed to scale mds deployment %q to %d", deployment.GetName(), replicas)
		}
	}
	// replicas already met requirement
	if *d.Spec.Replicas == replicas {
		return nil
	}
	*d.Spec.Replicas = replicas
	if _, err := clusterdContext.Clientset.AppsV1().Deployments(namespace).Update(ctx, d, metav1.UpdateOptions{}); err != nil {
		return errors.Wrapf(err, "failed to scale mds deployment %s to %d replicas", deployment.GetName(), replicas)
	}
	return nil
}
