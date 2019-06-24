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

// Package nfs for NFS ganesha
package nfs

import (
	"fmt"
	"time"

	cephv1 "github.com/rook/rook/pkg/apis/ceph.rook.io/v1"
	"github.com/rook/rook/pkg/clusterd"
	opmon "github.com/rook/rook/pkg/operator/ceph/cluster/mon"
	opspec "github.com/rook/rook/pkg/operator/ceph/spec"
	"github.com/rook/rook/pkg/operator/k8sutil"
	batch "k8s.io/api/batch/v1"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	ganeshaRadosGraceCmd = "ganesha-rados-grace"
)

var updateDeploymentAndWait = k8sutil.UpdateDeploymentAndWait

// Create the ganesha server
func (c *CephNFSController) upCephNFS(n cephv1.CephNFS, oldActive int) error {
	if err := validateGanesha(c.context, n); err != nil {
		return err
	}

	logger.Infof("Starting cephNFS %s(%d-%d)", n.Name, oldActive,
		n.Spec.Server.Active-1)

	for i := oldActive; i < n.Spec.Server.Active; i++ {
		name := k8sutil.IndexToName(i)

		configName, err := c.generateConfig(n, name)
		if err != nil {
			return fmt.Errorf("failed to create config. %+v", err)
		}

		err = c.addRADOSConfigFile(n, name)
		if err != nil {
			return fmt.Errorf("failed to create RADOS config object. %+v", err)
		}

		// start the deployment
		deployment := c.makeDeployment(n, name, configName)
		_, err = c.context.Clientset.AppsV1().Deployments(n.Namespace).Create(deployment)
		if err != nil {
			if !errors.IsAlreadyExists(err) {
				return fmt.Errorf("failed to create ganesha deployment. %+v", err)
			}
			logger.Infof("ganesha deployment %s already exists. updating if needed", deployment.Name)
			if _, err := updateDeploymentAndWait(c.context, deployment, n.Namespace); err != nil {
				return fmt.Errorf("failed to update ganesha deployment %s. %+v", deployment.Name, err)
			}
		} else {
			logger.Infof("ganesha deployment %s started", deployment.Name)
		}

		// create a service
		err = c.createCephNFSService(n, name)
		if err != nil {
			return fmt.Errorf("failed to create ganesha service. %+v", err)
		}

		c.addServerToDatabase(n, name)
	}

	return nil
}

// Create empty config file for new ganesha server
func (c *CephNFSController) addRADOSConfigFile(n cephv1.CephNFS, name string) error {
	nodeID := getNFSNodeID(n, name)
	config := getGaneshaConfigObject(nodeID)
	err := c.context.Executor.ExecuteCommand(false, "", "rados", "--pool", n.Spec.RADOS.Pool, "--namespace", n.Spec.RADOS.Namespace, "stat", config)
	if err == nil {
		// If stat works then we assume it's present already
		return nil
	}
	// try to create it
	return c.context.Executor.ExecuteCommand(false, "", "rados", "--pool", n.Spec.RADOS.Pool, "--namespace", n.Spec.RADOS.Namespace, "create", config)
}

func (c *CephNFSController) addServerToDatabase(n cephv1.CephNFS, name string) {
	logger.Infof("Adding ganesha %s to grace db", name)

	if err := c.runGaneshaRadosGraceJob(n, name, "add", 10*time.Minute); err != nil {
		logger.Errorf("failed to add %s to grace db. %+v", name, err)
	}
}

func (c *CephNFSController) removeServerFromDatabase(n cephv1.CephNFS, name string) {
	logger.Infof("Removing ganesha %s from grace db", name)

	if err := c.runGaneshaRadosGraceJob(n, name, "remove", 10*time.Minute); err != nil {
		logger.Errorf("failed to remmove %s from grace db. %+v", name, err)
	}
}

func (c *CephNFSController) runGaneshaRadosGraceJob(n cephv1.CephNFS, name, action string, timeout time.Duration) error {
	nodeID := getNFSNodeID(n, name)
	args := []string{"--pool", n.Spec.RADOS.Pool, "--ns", n.Spec.RADOS.Namespace, action, nodeID}

	// FIX: After the operator is based on the nautilus image, we can execute the command directly instead of running a job
	//return c.context.Executor.ExecuteCommand(false, "", ganeshaRadosGraceCmd, args...)

	job := &batch.Job{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "rook-ceph-nfs-ganesha-rados-grace",
			Namespace: n.Namespace,
		},
		Spec: batch.JobSpec{
			Template: v1.PodTemplateSpec{
				Spec: v1.PodSpec{
					InitContainers: []v1.Container{
						{
							Name: opspec.ConfigInitContainerName,
							Args: []string{
								"ceph",
								"config-init",
							},
							Image: k8sutil.MakeRookImage(c.rookImage),
							Env: []v1.EnvVar{
								{Name: "ROOK_USERNAME", Value: "client.admin"},
								{Name: "ROOK_KEYRING",
									ValueFrom: &v1.EnvVarSource{
										SecretKeyRef: &v1.SecretKeySelector{
											LocalObjectReference: v1.LocalObjectReference{Name: "rook-ceph-mon"},
											Key:                  "admin-secret",
										}}},
								k8sutil.PodIPEnvVar(k8sutil.PrivateIPEnvVar),
								k8sutil.PodIPEnvVar(k8sutil.PublicIPEnvVar),
								opmon.EndpointEnvVar(),
								k8sutil.ConfigOverrideEnvVar(),
							},
							VolumeMounts: opspec.RookVolumeMounts(),
						},
					},
					Containers: []v1.Container{
						{
							Command:      []string{ganeshaRadosGraceCmd},
							Args:         args,
							Name:         ganeshaRadosGraceCmd,
							Image:        c.cephVersion.Image,
							VolumeMounts: opspec.RookVolumeMounts(),
						},
					},
					Volumes:       opspec.PodVolumes("", ""),
					RestartPolicy: v1.RestartPolicyOnFailure,
				},
			},
		},
	}
	k8sutil.SetOwnerRef(&job.ObjectMeta, &c.ownerRef)

	// run the job to detect the version
	if err := k8sutil.RunReplaceableJob(c.context.Clientset, job, false); err != nil {
		return fmt.Errorf("failed to start job %s. %+v", job.Name, err)
	}

	if err := k8sutil.WaitForJobCompletion(c.context.Clientset, job, timeout); err != nil {
		return fmt.Errorf("failed to complete job %s. %+v", job.Name, err)
	}

	if err := k8sutil.DeleteBatchJob(c.context.Clientset, n.Namespace, job.Name, false); err != nil {
		return fmt.Errorf("failed to delete job %s. %+v", job.Name, err)
	}

	logger.Infof("successfully completed job %s", job.Name)
	return nil
}

func (c *CephNFSController) generateConfig(n cephv1.CephNFS, name string) (string, error) {

	data := map[string]string{
		"config": getGaneshaConfig(n, name),
	}
	configMap := &v1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      fmt.Sprintf("%s-%s-%s", appName, n.Name, name),
			Namespace: n.Namespace,
			Labels:    getLabels(n, name),
		},
		Data: data,
	}
	if _, err := c.context.Clientset.CoreV1().ConfigMaps(n.Namespace).Create(configMap); err != nil {
		if errors.IsAlreadyExists(err) {
			if _, err := c.context.Clientset.CoreV1().ConfigMaps(n.Namespace).Update(configMap); err != nil {
				return "", fmt.Errorf("failed to update ganesha config. %+v", err)
			}
			return configMap.Name, nil
		}
		return "", fmt.Errorf("failed to create ganesha config. %+v", err)
	}
	return configMap.Name, nil
}

// Delete the ganesha server
func (c *CephNFSController) downCephNFS(n cephv1.CephNFS, newActive int) error {
	for i := n.Spec.Server.Active - 1; i >= newActive; i-- {
		name := k8sutil.IndexToName(i)

		// Remove from grace db
		c.removeServerFromDatabase(n, name)

		// Delete the mds deployment
		k8sutil.DeleteDeployment(c.context.Clientset, n.Namespace, instanceName(n, name))

		// Delete the ganesha service
		options := &metav1.DeleteOptions{}
		err := c.context.Clientset.CoreV1().Services(n.Namespace).Delete(instanceName(n, name), options)
		if err != nil && !errors.IsNotFound(err) {
			logger.Warningf("failed to delete ganesha service. %+v", err)
		}
	}

	return nil
}

func instanceName(n cephv1.CephNFS, name string) string {
	return fmt.Sprintf("%s-%s-%s", appName, n.Name, name)
}

func validateGanesha(context *clusterd.Context, n cephv1.CephNFS) error {
	// core properties
	if n.Name == "" {
		return fmt.Errorf("missing name")
	}
	if n.Namespace == "" {
		return fmt.Errorf("missing namespace")
	}

	// Client recovery properties
	if n.Spec.RADOS.Pool == "" {
		return fmt.Errorf("missing RADOS.pool")
	}
	if n.Spec.RADOS.Namespace == "" {
		return fmt.Errorf("missing RADOS.namespace")
	}

	// Ganesha server properties
	if n.Spec.Server.Active == 0 {
		return fmt.Errorf("at least one active server required")
	}

	return nil
}
