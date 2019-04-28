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

package k8sutil

import (
	"fmt"
	"time"

	"github.com/rook/rook/pkg/clusterd"
	"k8s.io/api/apps/v1"
	apps "k8s.io/api/apps/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
)

// GetDeploymentImage returns the version of the image running in the pod spec for the desired container
func GetDeploymentImage(clientset kubernetes.Interface, namespace, name, container string) (string, error) {
	d, err := clientset.AppsV1().Deployments(namespace).Get(name, metav1.GetOptions{})
	if err != nil {
		return "", fmt.Errorf("failed to find deployment %s. %v", name, err)
	}
	return GetDeploymentSpecImage(clientset, *d, container, false)
}

func GetDeploymentSpecImage(clientset kubernetes.Interface, d apps.Deployment, container string, initContainer bool) (string, error) {
	image, err := GetSpecContainerImage(d.Spec.Template.Spec, container, initContainer)
	if err != nil {
		return "", err
	}

	return image, nil
}

// UpdateDeploymentAndWait updates a deployment and waits until it is running to return. It will
// error if the deployment does not exist to be updated or if it takes too long.
func UpdateDeploymentAndWait(context *clusterd.Context, deployment *apps.Deployment, namespace string) (*v1.Deployment, error) {
	original, err := context.Clientset.AppsV1().Deployments(namespace).Get(deployment.Name, metav1.GetOptions{})
	if err != nil {
		return nil, fmt.Errorf("failed to get deployment %s. %+v", deployment.Name, err)
	}

	logger.Infof("updating deployment %s", deployment.Name)
	if _, err := context.Clientset.AppsV1().Deployments(namespace).Update(deployment); err != nil {
		return nil, fmt.Errorf("failed to update deployment %s. %+v", deployment.Name, err)
	}

	// wait for the deployment to be restarted
	sleepTime := 2
	attempts := 30
	if original.Spec.ProgressDeadlineSeconds != nil {
		// make the attempts double the progress deadline since the pod is both stopping and starting
		attempts = 2 * (int(*original.Spec.ProgressDeadlineSeconds) / sleepTime)
	}
	for i := 0; i < attempts; i++ {
		// check for the status of the deployment
		d, err := context.Clientset.AppsV1().Deployments(namespace).Get(deployment.Name, metav1.GetOptions{})
		if err != nil {
			return nil, fmt.Errorf("failed to get deployment %s. %+v", deployment.Name, err)
		}
		if d.Status.ObservedGeneration != original.Status.ObservedGeneration && d.Status.UpdatedReplicas > 0 && d.Status.ReadyReplicas > 0 {
			logger.Infof("finished waiting for updated deployment %s", d.Name)
			return d, nil
		}

		logger.Debugf("deployment %s status=%+v", d.Name, d.Status)
		time.Sleep(time.Duration(sleepTime) * time.Second)
	}

	return nil, fmt.Errorf("gave up waiting for deployment %s to update", deployment.Name)
}

// GetDeployments returns a list of deployment names labels matching a given selector
// example of a label selector might be "app=rook-ceph-mon, mon!=b"
// more: https://kubernetes.io/docs/concepts/overview/working-with-objects/labels/
func GetDeployments(clientset kubernetes.Interface, namespace, labelSelector string) (*apps.DeploymentList, error) {
	listOptions := metav1.ListOptions{LabelSelector: labelSelector}
	deployments, err := clientset.AppsV1().Deployments(namespace).List(listOptions)
	if err != nil {
		return nil, fmt.Errorf("failed to list deployments with labelSelector %s: %v", labelSelector, err)
	}
	return deployments, nil
}

// DeleteDeployment makes a best effort at deleting a deployment and its pods, then waits for them to be deleted
func DeleteDeployment(clientset kubernetes.Interface, namespace, name string) error {
	logger.Debugf("removing %s deployment if it exists", name)
	deleteAction := func(options *metav1.DeleteOptions) error {
		return clientset.AppsV1().Deployments(namespace).Delete(name, options)
	}
	getAction := func() error {
		_, err := clientset.AppsV1().Deployments(namespace).Get(name, metav1.GetOptions{})
		return err
	}
	return deleteResourceAndWait(namespace, name, "deployment", deleteAction, getAction)
}

// WaitForDeploymentImage waits for all deployments with the given labels are running.
// WARNING:This is currently only useful for testing!
func WaitForDeploymentImage(clientset kubernetes.Interface, namespace, label, container string, initContainer bool, desiredImage string) error {

	sleepTime := 3
	attempts := 120
	for i := 0; i < attempts; i++ {
		deployments, err := clientset.AppsV1().Deployments(namespace).List(metav1.ListOptions{LabelSelector: label})
		if err != nil {
			return fmt.Errorf("failed to list deployments with label %s. %v", label, err)
		}

		matches := 0
		for _, d := range deployments.Items {
			image, err := GetDeploymentSpecImage(clientset, d, container, initContainer)
			if err != nil {
				logger.Infof("failed to get image for deployment %s. %+v", d.Name, err)
				continue
			}
			if image == desiredImage {
				matches++
			}
		}

		if matches == len(deployments.Items) && matches > 0 {
			logger.Infof("all %d %s deployments are on image %s", matches, label, desiredImage)
			return nil
		}

		if len(deployments.Items) == 0 {
			logger.Infof("waiting for at least one deployment to start to see the version")
		} else {
			logger.Infof("%d/%d %s deployments match image %s", matches, len(deployments.Items), label, desiredImage)
		}
		time.Sleep(time.Duration(sleepTime) * time.Second)
	}
	return fmt.Errorf("failed to wait for image %s in label %s", desiredImage, label)
}

// AddRookVersionLabelToDeployment adds or updates a label reporting the Rook version which last
// modified a deployment.
func AddRookVersionLabelToDeployment(d *v1.Deployment) {
	if d == nil {
		return
	}
	if d.Labels == nil {
		d.Labels = map[string]string{}
	}
	addRookVersionLabel(d.Labels)
}
