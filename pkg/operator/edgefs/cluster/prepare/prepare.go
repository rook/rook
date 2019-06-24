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

// Package prepare for the Edgefs manager.
package prepare

import (
	"context"
	"fmt"

	"github.com/coreos/pkg/capnslog"
	rookalpha "github.com/rook/rook/pkg/apis/rook.io/v1alpha2"
	"github.com/rook/rook/pkg/clusterd"
	"github.com/rook/rook/pkg/operator/k8sutil"
	batch "k8s.io/api/batch/v1"
	"k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var logger = capnslog.NewPackageLogger("github.com/rook/rook", "edgefs-op-prepare")

const (
	appName                   = "host-prepare"
	defaultServiceAccountName = "rook-edgefs-cluster"
	etcVolumeName             = "etc-volume"
	jobNameFmt                = "%s-%s"
)

// Cluster is the edgefs prepare manager
type Cluster struct {
	Namespace      string
	Version        string
	serviceAccount string
	annotations    rookalpha.Annotations
	placement      rookalpha.Placement
	context        *clusterd.Context
	resources      v1.ResourceRequirements
	ownerRef       metav1.OwnerReference
	ctx            context.Context
}

// New creates an instance of the prepare
func New(
	context *clusterd.Context, namespace, version string,
	serviceAccount string,
	annotations rookalpha.Annotations,
	placement rookalpha.Placement,
	resources v1.ResourceRequirements,
	ownerRef metav1.OwnerReference,
) *Cluster {

	if serviceAccount == "" {
		// if the service account was not set, make a best effort with the example service account name since the default is unlikely to be sufficient.
		serviceAccount = defaultServiceAccountName
		logger.Infof("setting the prepare pods to use the service account name: %s", serviceAccount)
	}

	return &Cluster{
		context:        context,
		Namespace:      namespace,
		serviceAccount: serviceAccount,
		annotations:    annotations,
		placement:      placement,
		Version:        version,
		resources:      resources,
		ownerRef:       ownerRef,
	}
}

// Start the prepare instance
func (c *Cluster) Start(rookImage string, nodeName string) error {
	logger.Infof("start running prepare pods")

	// start the deployment
	job := c.makeJob(appName, c.Namespace, rookImage, nodeName)
	if _, err := c.context.Clientset.BatchV1().Jobs(c.Namespace).Create(job); err != nil {
		if !errors.IsAlreadyExists(err) {
			return fmt.Errorf("failed to create %s job. %+v", appName, err)
		}
		logger.Infof("%s daemonset already exists", appName)
	} else {
		logger.Infof("%s job started", appName)
	}

	err := c.waitJob(job)
	if err != nil {
		logger.Warningf("Job %s waiting failed. %+v", job.ObjectMeta.Name, err)
	}

	err = c.deleteJob(job)
	if err != nil {
		logger.Warningf("Failed to delete job %s due. %+v", job.ObjectMeta.Name, err)
	}

	return nil
}

func (c *Cluster) makeJob(name, clusterName, rookImage string, nodeName string) *batch.Job {

	volumes := []v1.Volume{
		{
			Name: etcVolumeName,
			VolumeSource: v1.VolumeSource{
				HostPath: &v1.HostPathVolumeSource{
					Path: "/etc",
				},
			},
		},
	}

	gracePeriod := int64(0)
	podSpec := v1.PodTemplateSpec{
		ObjectMeta: metav1.ObjectMeta{
			Name:   name,
			Labels: c.getDaemonLabels(clusterName),
		},
		Spec: v1.PodSpec{
			ServiceAccountName:            c.serviceAccount,
			Containers:                    []v1.Container{c.prepareContainer(name, rookImage)},
			RestartPolicy:                 v1.RestartPolicyOnFailure,
			HostIPC:                       true,
			HostNetwork:                   true,
			TerminationGracePeriodSeconds: &gracePeriod,
			NodeSelector: map[string]string{c.Namespace: "cluster",
				"kubernetes.io/hostname": nodeName},
			Volumes: volumes,
		},
	}
	podSpec.Spec.DNSPolicy = v1.DNSClusterFirstWithHostNet
	c.placement.ApplyToPodSpec(&podSpec.Spec)

	ds := &batch.Job{
		ObjectMeta: metav1.ObjectMeta{
			Name:      k8sutil.TruncateNodeName(fmt.Sprintf(jobNameFmt, name, "%s"), nodeName),
			Namespace: c.Namespace,
		},
		Spec: batch.JobSpec{Template: podSpec},
	}
	k8sutil.SetOwnerRef(&ds.ObjectMeta, &c.ownerRef)
	return ds
}

func (c *Cluster) waitJob(job *batch.Job) error {
	batchClient := c.context.Clientset.BatchV1()
	jobsClient := batchClient.Jobs(job.ObjectMeta.Namespace)
	watch, err := jobsClient.Watch(metav1.ListOptions{LabelSelector: "job-name=" + job.ObjectMeta.Name})

	k8sjob, err := jobsClient.Get(job.ObjectMeta.Name, metav1.GetOptions{})
	if err != nil {
		return fmt.Errorf("Failed to get job %s", job.ObjectMeta.Name)
	}

	if k8sjob == nil {
		return fmt.Errorf("Couldn't find job %s", job.ObjectMeta.Name)
	}

	events := watch.ResultChan()
	for {
		select {
		case event := <-events:
			if event.Object == nil {
				return fmt.Errorf("Result channel closed for Job %s", job.ObjectMeta.Name)
			}
			k8sJob, ok := event.Object.(*batch.Job)
			if !ok {
				return fmt.Errorf("Invalid Job event object: %T", event.Object)
			}
			conditions := k8sJob.Status.Conditions
			for _, condition := range conditions {
				if condition.Type == batch.JobComplete {
					logger.Infof("Job %s reported complete", job.ObjectMeta.Name)
					return nil
				} else if condition.Type == batch.JobFailed {
					return fmt.Errorf("Job %s failed", job.ObjectMeta.Name)
				}
			}
		}
	}
}

func (c *Cluster) deleteJob(job *batch.Job) error {
	batchClient := c.context.Clientset.BatchV1()
	jobsClient := batchClient.Jobs(job.ObjectMeta.Namespace)
	var deletePropagation metav1.DeletionPropagation
	deletePropagation = metav1.DeletePropagationForeground
	err := jobsClient.Delete(job.ObjectMeta.Name, &metav1.DeleteOptions{PropagationPolicy: &deletePropagation})
	if err != nil {
		return fmt.Errorf("Failed to delete job %s", job.ObjectMeta.Name)
	}
	logger.Infof("Deleted job %s", job.ObjectMeta.Name)

	return nil
}

func (c *Cluster) prepareContainer(name string, containerImage string) v1.Container {

	privileged := true
	runAsUser := int64(0)
	readOnlyRootFilesystem := false
	securityContext := &v1.SecurityContext{
		Privileged:             &privileged,
		RunAsUser:              &runAsUser,
		ReadOnlyRootFilesystem: &readOnlyRootFilesystem,
	}

	volumeMounts := []v1.VolumeMount{
		{
			Name:      etcVolumeName,
			MountPath: "/etc",
		},
	}

	return v1.Container{
		Name:            name,
		Image:           containerImage,
		ImagePullPolicy: v1.PullAlways,
		Args:            []string{"prepare"},
		VolumeMounts:    volumeMounts,
		Env: []v1.EnvVar{
			{
				Name:  "CCOW_LOG_LEVEL",
				Value: "5",
			},
			{
				Name: "HOST_HOSTNAME",
				ValueFrom: &v1.EnvVarSource{
					FieldRef: &v1.ObjectFieldSelector{
						FieldPath: "spec.nodeName",
					},
				},
			},
		},
		SecurityContext: securityContext,
		Resources:       c.resources,
	}
}

func (c *Cluster) getLabels() map[string]string {
	return map[string]string{
		k8sutil.AppAttr:     appName,
		k8sutil.ClusterAttr: c.Namespace,
	}
}

func (c *Cluster) getDaemonLabels(clusterName string) map[string]string {
	labels := c.getLabels()
	labels["instance"] = clusterName
	return labels
}
