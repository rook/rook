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

// Package k8sutil for Kubernetes helpers.
package k8sutil

import (
	"fmt"
	"time"

	batch "k8s.io/api/batch/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/kubernetes"
)

func RunReplaceableJob(clientset kubernetes.Interface, job *batch.Job) error {
	// check if the job was already created and what its status is
	existingJob, err := clientset.Batch().Jobs(job.Namespace).Get(job.Name, metav1.GetOptions{})
	if err != nil && !errors.IsNotFound(err) {
		logger.Warningf("failed to detect job %s. %+v", job.Name, err)
	} else if err == nil {
		// if the job is still running, allow it to continue to completion
		if existingJob.Status.Active > 0 {
			logger.Infof("Found previous job %s. Status=%+v", job.Name, existingJob.Status)
			return nil
		}

		// delete the job that already exists from a previous run
		logger.Infof("Removing previous job %s to start a new one", job.Name)
		err := DeleteBatchJob(clientset, job.Namespace, existingJob.Name, true)
		if err != nil {
			logger.Warningf("failed to remove job %s. %+v", job.Name, err)
		}
	}

	_, err = clientset.Batch().Jobs(job.Namespace).Create(job)
	return err
}

// WaitForJobCompletion waits for a job to reach the completed state.
// Assumes that only one pod needs to complete.
func WaitForJobCompletion(clientset kubernetes.Interface, job *batch.Job, timeout time.Duration) error {
	logger.Infof("waiting for job %s to complete...", job.Name)
	return wait.Poll(5*time.Second, timeout, func() (bool, error) {
		job, err := clientset.Batch().Jobs(job.Namespace).Get(job.Name, metav1.GetOptions{})
		if err != nil {
			return false, fmt.Errorf("failed to detect job %s. %+v", job.Name, err)
		}

		// if the job is still running, allow it to continue to completion
		if job.Status.Active > 0 {
			logger.Debugf("job is still running. Status=%+v", job.Status)
			return false, nil
		}
		if job.Status.Failed > 0 {
			return false, fmt.Errorf("job %s failed.", job.Name)
		}
		if job.Status.Succeeded > 0 {
			return true, nil
		}
		logger.Debugf("job is still initializing")
		return false, nil
	})
}

func DeleteBatchJob(clientset kubernetes.Interface, namespace, name string, wait bool) error {
	propagation := metav1.DeletePropagationForeground
	gracePeriod := int64(0)
	options := &metav1.DeleteOptions{GracePeriodSeconds: &gracePeriod, PropagationPolicy: &propagation}

	if err := clientset.Batch().Jobs(namespace).Delete(name, options); err != nil {
		return fmt.Errorf("failed to remove previous provisioning job for node %s. %+v", name, err)
	}

	if !wait {
		return nil
	}

	retries := 20
	sleepInterval := 2 * time.Second
	for i := 0; i < retries; i++ {
		_, err := clientset.Batch().Jobs(namespace).Get(name, metav1.GetOptions{})
		if err != nil && errors.IsNotFound(err) {
			logger.Infof("batch job %s deleted", name)
			return nil
		}

		logger.Infof("batch job %s still exists", name)
		time.Sleep(sleepInterval)
	}

	logger.Warningf("gave up waiting for batch job %s to be deleted", name)
	return nil
}
