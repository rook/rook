/*
Copyright 2019 The Rook Authors. All rights reserved.

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

package newosd

import (
	"fmt"

	cephv1 "github.com/rook/rook/pkg/apis/ceph.rook.io/v1"
	"github.com/rook/rook/pkg/operator/ceph/config"
	opspec "github.com/rook/rook/pkg/operator/ceph/spec"
	"github.com/rook/rook/pkg/operator/k8sutil/cmdreporter"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type cephVolumeJobConfiguration struct {
	parentController *Controller
	hostname         string
	jobName          string
	appName          string
	cephVolumeArgs   []string
}

func (j *cephVolumeJobConfiguration) cmdReporter() (*cmdreporter.CmdReporter, error) {
	c := j.parentController
	cephCluster := c.cephCluster.spec
	commonLabels := opspec.AppLabels(j.appName, c.namespace)

	// Use stdbuf to capture the python output buffer such that we can write to the pod log as the
	// logging happens instead of using the default buffering that will log everything after
	// ceph-volume exits
	cmd := []string{"stdbuf"}
	args := append([]string{"-oL", "ceph-volume"}, j.cephVolumeArgs...)
	cmdReporter, err := cmdreporter.New(
		c.context.Clientset, &c.ownerRef,
		j.appName, j.jobName, c.namespace,
		cmd, args,
		c.rookImage, c.cephCluster.spec.CephVersion.Image)
	if err != nil {
		return nil, fmt.Errorf("failed to create ceph-volume job for node %s. %+v", j.hostname, err)
	}

	dataPathMap := config.NewDatalessDaemonDataPathMap(c.namespace, cephCluster.DataDirHostPath)

	// update job spec for ceph-volume needs
	job := cmdReporter.Job()
	applyLabelsToObjectMeta(commonLabels, &job.ObjectMeta)
	applyLabelsToObjectMeta(commonLabels, &job.Spec.Template.ObjectMeta)
	opspec.AddCephVersionLabelToJob(c.cephCluster.info.CephVersion, job)
	annotations := cephv1.GetOSDAnnotations(cephCluster.Annotations)
	annotations.ApplyToObjectMeta(&job.ObjectMeta)
	annotations.ApplyToObjectMeta(&job.Spec.Template.ObjectMeta)

	// update pod spec for ceph-volume needs
	podSpec := &job.Spec.Template.Spec
	podSpec.ServiceAccountName = serviceAccountName
	deviceVols, _ := deviceVolsAndMounts()
	podSpec.Volumes = append(podSpec.Volumes,
		append(
			deviceVols,
			opspec.StoredLogVolume(dataPathMap.ContainerLogDir),
		)...,
	)
	podSpec.HostNetwork = cephCluster.Network.HostNetwork
	// TODO: HostIPC for encrypted devices
	if cephCluster.Network.HostNetwork {
		podSpec.DNSPolicy = v1.DNSClusterFirstWithHostNet
	}
	placement := cephv1.GetOSDPlacement(cephCluster.Placement)
	placement.ApplyToPodSpec(podSpec)

	// update container spec for ceph-volume needs
	if len(podSpec.Containers) < 1 {
		return nil, fmt.Errorf("CmdReporter did not return a runner container in its batch job: %+v", job)
	}
	container := &podSpec.Containers[0]
	_, deviceVolMounts := deviceVolsAndMounts()
	container.VolumeMounts = append(container.VolumeMounts,
		append(
			deviceVolMounts,
			opspec.StoredLogVolumeMount(),
		)...,
	)
	container.SecurityContext = &v1.SecurityContext{
		// ceph-volume containers mount /dev, so privilege is required
		Privileged:             newBool(true),
		RunAsUser:              newInt64(0),
		RunAsNonRoot:           newBool(false),
		ReadOnlyRootFilesystem: newBool(false),
	}
	container.Resources = cephv1.GetOSDResources(j.parentController.cephCluster.spec.Resources)

	return cmdReporter, nil
}

func applyLabelsToObjectMeta(labels map[string]string, m *metav1.ObjectMeta) {
	for k, v := range labels {
		m.Labels[k] = v
	}
}

func deviceVolsAndMounts() ([]v1.Volume, []v1.VolumeMount) {
	vols := []v1.Volume{
		// c-v needs access to /dev to find and prepare devices
		{Name: "dev", VolumeSource: v1.VolumeSource{
			HostPath: &v1.HostPathVolumeSource{Path: "/dev"}}},
		// mount /run/udev so ceph-volume (via `lvs`) can access the udev database
		{Name: "run-udev", VolumeSource: v1.VolumeSource{
			HostPath: &v1.HostPathVolumeSource{Path: "/run/udev"}}},
	}
	mounts := []v1.VolumeMount{
		{Name: "dev", MountPath: "/dev"},
		{Name: "run-udev", MountPath: "/run/udev"},
	}
	return vols, mounts
}
