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

package multus

import (
	"bytes"
	"context"
	"fmt"
	"html/template"
	"time"

	batch "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	k8sErrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/pkg/errors"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/apimachinery/pkg/util/yaml"
	"k8s.io/client-go/kubernetes"
)

type JobParameters struct {
	ControllerName      string
	ControllerNamespace string
	ControllerImage     string
	NodeName            string
	ControllerIP        string
	MultusInterface     string
	MigratedInterface   string
}

func (params *JobParameters) SetControllerParams(pod *corev1.Pod) error {
	multusNetworkName, found := pod.ObjectMeta.Annotations[multusAnnotation]
	if !found {
		return errors.New("failed get find multus annotation")
	}
	multusConf, err := getMultusConfs(pod)
	if err != nil {
		return errors.Wrap(err, "failed to get multus configuration")
	}
	multusIfaceName, err := findMultusInterfaceName(multusConf, multusNetworkName, pod.ObjectMeta.Namespace)
	if err != nil {
		return errors.Wrap(err, "failed to get multus interface name")
	}

	params.ControllerName = pod.ObjectMeta.Name
	params.ControllerNamespace = pod.ObjectMeta.Namespace
	params.ControllerIP = pod.Status.PodIP
	params.ControllerImage = pod.Spec.Containers[0].Image
	params.NodeName = pod.Spec.NodeName
	params.MultusInterface = multusIfaceName

	return nil
}

func (params *JobParameters) SetMigratedInterfaceName(pod *corev1.Pod) error {
	iface, found := pod.ObjectMeta.Annotations[migrationAnnotation]
	if !found {
		return errors.New("failed to get multus annotation")
	}
	params.MigratedInterface = iface
	return nil
}

func templateToJob(name, templateData string, p JobParameters) (*batch.Job, error) {
	var job batch.Job
	t, err := loadTemplate(name, templateData, p)
	if err != nil {
		return nil, errors.Wrap(err, "failed to load job template")
	}

	err = yaml.Unmarshal([]byte(t), &job)
	if err != nil {
		return nil, errors.Wrap(err, "failed to unmarshal job template")
	}
	return &job, nil
}

func loadTemplate(name, templateData string, p JobParameters) ([]byte, error) {
	var writer bytes.Buffer
	t := template.New(name)
	t, err := t.Parse(templateData)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to parse template %v", name)
	}
	err = t.Execute(&writer, p)
	return writer.Bytes(), err
}

func runReplaceableJob(ctx context.Context, clientset kubernetes.Interface, job *batch.Job) error {
	// check if the job was already created and what its status is
	existingJob, err := clientset.BatchV1().Jobs(job.Namespace).Get(ctx, job.Name, metav1.GetOptions{})
	if err != nil && !k8sErrors.IsNotFound(err) {
		return errors.Wrap(err, "failed to check for existing job")
	} else if err == nil {
		// delete the job that already exists from a previous run
		err = clientset.BatchV1().Jobs(existingJob.Namespace).Delete(ctx, existingJob.Name, metav1.DeleteOptions{})
		if err != nil {
			return fmt.Errorf("failed to remove existing job %s. %v", job.Name, err)
		}
		// Wait for delete to complete before continuing
		err = wait.Poll(time.Second, 20*time.Second, func() (bool, error) {
			_, err := clientset.BatchV1().Jobs(job.Namespace).Get(ctx, job.Name, metav1.GetOptions{})
			if err != nil && !k8sErrors.IsNotFound(err) {
				return false, err
			} else if err == nil {
				// Job resource hasn't been deleted yet.
				return false, nil
			}
			return true, nil
		})
		if err != nil {
			return errors.Wrap(err, "failed to wait for job deletion to complete")
		}
	}

	_, err = clientset.BatchV1().Jobs(job.Namespace).Create(ctx, job, metav1.CreateOptions{})
	return errors.Wrap(err, "failed to create job")
}

func WaitForJobCompletion(ctx context.Context, clientset kubernetes.Interface, job *batch.Job, timeout time.Duration) error {
	return wait.Poll(5*time.Second, timeout, func() (bool, error) {
		job, err := clientset.BatchV1().Jobs(job.Namespace).Get(ctx, job.Name, metav1.GetOptions{})
		if err != nil {
			return false, fmt.Errorf("failed to detect job %s. %+v", job.Name, err)
		}

		// if the job is still running, allow it to continue to completion
		if job.Status.Active > 0 {
			return false, nil
		}
		if job.Status.Failed > 0 {
			return false, fmt.Errorf("job %s failed", job.Name)
		}
		if job.Status.Succeeded > 0 {
			return true, nil
		}
		return false, nil
	})
}
