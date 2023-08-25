/*
Copyright 2021 The Rook Authors. All rights reserved.

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

package mirror

import (
	"fmt"

	cephv1 "github.com/rook/rook/pkg/apis/ceph.rook.io/v1"
	"github.com/rook/rook/pkg/operator/ceph/config"
	"github.com/rook/rook/pkg/operator/ceph/controller"
	"github.com/rook/rook/pkg/operator/k8sutil"
	apps "k8s.io/api/apps/v1"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func (r *ReconcileFilesystemMirror) makeDeployment(daemonConfig *daemonConfig, fsMirror *cephv1.CephFilesystemMirror) (*apps.Deployment, error) {
	podSpec := v1.PodTemplateSpec{
		ObjectMeta: metav1.ObjectMeta{
			Name:      daemonConfig.ResourceName,
			Namespace: fsMirror.Namespace,
			Labels:    controller.CephDaemonAppLabels(AppName, fsMirror.Namespace, config.FilesystemMirrorType, userID, fsMirror.Name, "cephfilesystemmirrors.ceph.rook.io", true),
		},
		Spec: v1.PodSpec{
			InitContainers: []v1.Container{
				r.makeChownInitContainer(daemonConfig, fsMirror),
			},
			Containers: []v1.Container{
				r.makeFsMirroringDaemonContainer(daemonConfig, fsMirror),
			},
			RestartPolicy:     v1.RestartPolicyAlways,
			Volumes:           controller.DaemonVolumes(daemonConfig.DataPathMap, daemonConfig.ResourceName, r.cephClusterSpec.DataDirHostPath),
			HostNetwork:       r.cephClusterSpec.Network.IsHost(),
			PriorityClassName: fsMirror.Spec.PriorityClassName,
		},
	}

	// If the log collector is enabled we add the side-car container
	if r.cephClusterSpec.LogCollector.Enabled {
		shareProcessNamespace := true
		podSpec.Spec.ShareProcessNamespace = &shareProcessNamespace
		podSpec.Spec.Containers = append(podSpec.Spec.Containers, *controller.LogCollectorContainer(fmt.Sprintf("ceph-%s", user), r.clusterInfo.Namespace, *r.cephClusterSpec))
	}

	// Replace default unreachable node toleration
	k8sutil.AddUnreachableNodeToleration(&podSpec.Spec)
	fsMirror.Spec.Annotations.ApplyToObjectMeta(&podSpec.ObjectMeta)
	fsMirror.Spec.Labels.ApplyToObjectMeta(&podSpec.ObjectMeta)

	if r.cephClusterSpec.Network.IsHost() {
		podSpec.Spec.DNSPolicy = v1.DNSClusterFirstWithHostNet
	} else if r.cephClusterSpec.Network.IsMultus() {
		if err := k8sutil.ApplyMultus(r.cephClusterSpec.Network, &podSpec.ObjectMeta); err != nil {
			return nil, err
		}
	}
	fsMirror.Spec.Placement.ApplyToPodSpec(&podSpec.Spec)

	replicas := int32(1)
	d := &apps.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:        daemonConfig.ResourceName,
			Namespace:   fsMirror.Namespace,
			Annotations: fsMirror.Spec.Annotations,
			Labels:      controller.CephDaemonAppLabels(AppName, fsMirror.Namespace, config.FilesystemMirrorType, userID, fsMirror.Name, "cephfilesystemmirrors.ceph.rook.io", true)},
		Spec: apps.DeploymentSpec{
			Selector: &metav1.LabelSelector{
				MatchLabels: podSpec.Labels,
			},
			Template: podSpec,
			Replicas: &replicas,
		},
	}
	k8sutil.AddRookVersionLabelToDeployment(d)
	controller.AddCephVersionLabelToDeployment(r.clusterInfo.CephVersion, d)
	fsMirror.Spec.Annotations.ApplyToObjectMeta(&d.ObjectMeta)
	fsMirror.Spec.Labels.ApplyToObjectMeta(&d.ObjectMeta)

	return d, nil
}

func (r *ReconcileFilesystemMirror) makeChownInitContainer(daemonConfig *daemonConfig, fsMirror *cephv1.CephFilesystemMirror) v1.Container {
	return controller.ChownCephDataDirsInitContainer(
		*daemonConfig.DataPathMap,
		r.cephClusterSpec.CephVersion.Image,
		controller.GetContainerImagePullPolicy(r.cephClusterSpec.CephVersion.ImagePullPolicy),
		controller.DaemonVolumeMounts(daemonConfig.DataPathMap, daemonConfig.ResourceName, r.cephClusterSpec.DataDirHostPath),
		fsMirror.Spec.Resources,
		controller.PodSecurityContext(),
		"",
	)
}

func (r *ReconcileFilesystemMirror) makeFsMirroringDaemonContainer(daemonConfig *daemonConfig, fsMirror *cephv1.CephFilesystemMirror) v1.Container {
	container := v1.Container{
		Name: "fs-mirror",
		Command: []string{
			"cephfs-mirror",
		},
		Args: append(
			controller.DaemonFlags(r.clusterInfo, r.cephClusterSpec, userID),
			"--foreground",
			"--name="+user,
		),
		Image:           r.cephClusterSpec.CephVersion.Image,
		ImagePullPolicy: controller.GetContainerImagePullPolicy(r.cephClusterSpec.CephVersion.ImagePullPolicy),
		VolumeMounts:    controller.DaemonVolumeMounts(daemonConfig.DataPathMap, daemonConfig.ResourceName, r.cephClusterSpec.DataDirHostPath),
		Env:             controller.DaemonEnvVars(r.cephClusterSpec),
		Resources:       fsMirror.Spec.Resources,
		SecurityContext: controller.PodSecurityContext(),
		// TODO:
		// LivenessProbe:   controller.GenerateLivenessProbeExecDaemon(config.fsMirrorType, daemonConfig.DaemonID),
	}

	return container
}
