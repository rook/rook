/*
Copyright 2024 The Rook Authors. All rights reserved.

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
package controller

import (
	"context"
	"strings"

	"github.com/pkg/errors"
	cephv1 "github.com/rook/rook/pkg/apis/ceph.rook.io/v1"
	"github.com/rook/rook/pkg/operator/k8sutil"
	batch "k8s.io/api/batch/v1"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	k8sClient "sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	volumeName                  = "cleanup-volume"
	dataDirHostPath             = "ROOK_DATA_DIR_HOST_PATH"
	CleanupAppName              = "resource-cleanup"
	RESOURCE_CLEANUP_ANNOTATION = "rook.io/force-deletion"

	// CephFSSubVolumeGroup env resources
	CephFSSubVolumeGroupNameEnv = "SUB_VOLUME_GROUP_NAME"
	CephFSNameEnv               = "FILESYSTEM_NAME"
	CSICephFSRadosNamesaceEnv   = "CSI_CEPHFS_RADOS_NAMESPACE"
	CephFSMetaDataPoolNameEnv   = "METADATA_POOL_NAME"

	// cephblockpoolradosnamespace env resources
	CephBlockPoolNameEnv           = "BLOCKPOOL_NAME"
	CephBlockPoolRadosNamespaceEnv = "RADOS_NAMESPACE"
)

// ResourceCleanup defines an rook ceph resource to be cleaned up
type ResourceCleanup struct {
	resource  k8sClient.Object
	cluster   *cephv1.CephCluster
	rookImage string
	// config defines the attributes of the custom resource to passed in as environment variables in the clean up job
	config map[string]string
}

func NewResourceCleanup(obj k8sClient.Object, cluster *cephv1.CephCluster, rookImage string, config map[string]string) *ResourceCleanup {
	return &ResourceCleanup{
		resource:  obj,
		rookImage: rookImage,
		cluster:   cluster,
		config:    config,
	}
}

// Start a new job to perform clean up of the ceph resources. It returns true if the cleanup job has succeeded
func (c *ResourceCleanup) StartJob(ctx context.Context, clientset kubernetes.Interface, jobName string) error {
	podSpec := c.jobTemplateSpec()
	job := &batch.Job{
		ObjectMeta: metav1.ObjectMeta{
			Name:            jobName,
			Namespace:       c.resource.GetNamespace(),
			OwnerReferences: c.resource.GetOwnerReferences(),
		},
		Spec: batch.JobSpec{
			Template: podSpec,
		},
	}

	if err := k8sutil.RunReplaceableJob(ctx, clientset, job, false); err != nil {
		return errors.Wrapf(err, "failed to run clean up job for %q resource named %q in namespace %q",
			c.resource.GetObjectKind().GroupVersionKind().Kind, c.resource.GetName(), c.resource.GetNamespace())
	}

	return nil
}

func (c *ResourceCleanup) jobContainer() v1.Container {
	volumeMounts := []v1.VolumeMount{}
	envVars := []v1.EnvVar{}
	if c.cluster.Spec.DataDirHostPath != "" {
		hostPathVolumeMount := v1.VolumeMount{Name: volumeName, MountPath: c.cluster.Spec.DataDirHostPath}
		volumeMounts = append(volumeMounts, hostPathVolumeMount)
		envVars = append(envVars, []v1.EnvVar{
			{Name: dataDirHostPath, Value: c.cluster.Spec.DataDirHostPath},
			{Name: "ROOK_LOG_LEVEL", Value: "DEBUG"},
			{Name: k8sutil.PodNamespaceEnvVar, Value: c.resource.GetNamespace()},
		}...)
	}
	// append all the resource attributes as env variables.
	for k, v := range c.config {
		envVars = append(envVars, v1.EnvVar{Name: k, Value: v})
	}
	securityContext := PrivilegedContext(true)
	return v1.Container{
		Name:            "resource-cleanup",
		Image:           c.rookImage,
		SecurityContext: securityContext,
		VolumeMounts:    volumeMounts,
		Env:             envVars,
		Args:            []string{"ceph", "clean", c.resource.GetObjectKind().GroupVersionKind().Kind},
		Resources:       cephv1.GetCleanupResources(c.cluster.Spec.Resources),
	}
}

func (c *ResourceCleanup) jobTemplateSpec() v1.PodTemplateSpec {
	volumes := []v1.Volume{}
	hostPathVolume := v1.Volume{Name: volumeName, VolumeSource: v1.VolumeSource{HostPath: &v1.HostPathVolumeSource{Path: c.cluster.Spec.DataDirHostPath}}}
	volumes = append(volumes, hostPathVolume)

	podSpec := v1.PodTemplateSpec{
		ObjectMeta: metav1.ObjectMeta{
			Name: CleanupAppName,
		},
		Spec: v1.PodSpec{
			Containers: []v1.Container{
				c.jobContainer(),
			},
			Volumes:            volumes,
			RestartPolicy:      v1.RestartPolicyOnFailure,
			PriorityClassName:  cephv1.GetCleanupPriorityClassName(c.cluster.Spec.PriorityClassNames),
			ServiceAccountName: k8sutil.DefaultServiceAccount,
		},
	}

	return podSpec
}

// ForceDeleteRequested returns true if `rook.io/force-deletion:true` annotation is available on the resource
func ForceDeleteRequested(annotations map[string]string) bool {
	if value, found := annotations[RESOURCE_CLEANUP_ANNOTATION]; found {
		if strings.EqualFold(value, "true") {
			return true
		}
	}
	return false
}
