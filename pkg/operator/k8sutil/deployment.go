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

	"github.com/banzaicloud/k8s-objectmatcher/patch"

	"github.com/rook/rook/pkg/clusterd"
	"github.com/rook/rook/pkg/util"
	apps "k8s.io/api/apps/v1"
	v1 "k8s.io/api/apps/v1"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
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

// GetDeploymentSpecImage returns the image name from the spec
func GetDeploymentSpecImage(clientset kubernetes.Interface, d apps.Deployment, container string, initContainer bool) (string, error) {
	image, err := GetSpecContainerImage(d.Spec.Template.Spec, container, initContainer)
	if err != nil {
		return "", err
	}

	return image, nil
}

// UpdateDeploymentAndWait updates a deployment and waits until it is running to return. It will
// error if the deployment does not exist to be updated or if it takes too long.
// This method has a generic callback function that each backend can rely on
// It serves two purposes:
//   1. verify that a resource can be stopped
//   2. verify that we can continue the update procedure
// Basically, we go one resource by one and check if we can stop and then if the resource has been successfully updated
// we check if we can go ahead and move to the next one.
func UpdateDeploymentAndWait(context *clusterd.Context, modifiedDeployment *apps.Deployment, namespace string, verifyCallback func(action string) error) (*v1.Deployment, error) {
	currentDeployment, err := context.Clientset.AppsV1().Deployments(namespace).Get(modifiedDeployment.Name, metav1.GetOptions{})
	if err != nil {
		return nil, fmt.Errorf("failed to get deployment %s. %+v", modifiedDeployment.Name, err)
	}

	// Check whether the current deployement and newly generated one are identical
	patchResult, err := patch.DefaultPatchMaker.Calculate(currentDeployment, modifiedDeployment)
	if err != nil {
		return nil, fmt.Errorf("failed to calculate diff between current deployment %q and newly generated one. %v", currentDeployment.Name, err)
	}

	// If deployments are different, let's update!
	if !patchResult.IsEmpty() {
		// Let's verify the deployment can be stopped
		// retry for 5 times, every minute
		err = util.Retry(5, 60*time.Second, func() error {
			return verifyCallback("stop")
		})
		if err != nil {
			return nil, fmt.Errorf("failed to check if deployment %q can be updated. %v", modifiedDeployment.Name, err)
		}

		// Set hash annotation to the newly generated deployment
		err := patch.DefaultAnnotator.SetLastAppliedAnnotation(modifiedDeployment)
		if err != nil {
			return nil, fmt.Errorf("failed to set hash annotation on deployment %q. %v", modifiedDeployment.Name, err)
		}

		logger.Infof("updating deployment %q", modifiedDeployment.Name)
		if _, err := context.Clientset.AppsV1().Deployments(namespace).Update(modifiedDeployment); err != nil {
			return nil, fmt.Errorf("failed to update deployment %q. %v", modifiedDeployment.Name, err)
		}

		// wait for the deployment to be restarted
		sleepTime := 2
		attempts := 30
		if currentDeployment.Spec.ProgressDeadlineSeconds != nil {
			// make the attempts double the progress deadline since the pod is both stopping and starting
			attempts = 2 * (int(*currentDeployment.Spec.ProgressDeadlineSeconds) / sleepTime)
		}
		for i := 0; i < attempts; i++ {
			// check for the status of the deployment
			d, err := context.Clientset.AppsV1().Deployments(namespace).Get(modifiedDeployment.Name, metav1.GetOptions{})
			if err != nil {
				return nil, fmt.Errorf("failed to get deployment %q. %v", modifiedDeployment.Name, err)
			}
			if d.Status.ObservedGeneration != currentDeployment.Status.ObservedGeneration && d.Status.UpdatedReplicas > 0 && d.Status.ReadyReplicas > 0 {
				logger.Infof("finished waiting for updated deployment %q", d.Name)

				// Now we check if we can go to the next daemon
				err = verifyCallback("continue")
				if err != nil {
					return nil, fmt.Errorf("failed to check if deployment %q can continue: %v", modifiedDeployment.Name, err)
				}

				return d, nil
			}

			// If ProgressDeadlineExceeded is reached let's fail earlier
			// This can happen if one of the deployment cannot be scheduled on a node and stays in "pending" state
			for _, condition := range d.Status.Conditions {
				if condition.Type == v1.DeploymentProgressing && condition.Reason == "ProgressDeadlineExceeded" {
					return nil, fmt.Errorf("gave up waiting for deployment %q to update because %q", modifiedDeployment.Name, condition.Reason)
				}
			}

			logger.Debugf("deployment %q status=%+v", d.Name, d.Status)
			time.Sleep(time.Duration(sleepTime) * time.Second)
		}
		return nil, fmt.Errorf("gave up waiting for deployment %q to update", modifiedDeployment.Name)
	}

	logger.Infof("deployment %q did not change, nothing to update", currentDeployment.Name)
	return nil, nil
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

func AddRookVersionLabelToObjectMeta(meta *metav1.ObjectMeta) {
	if meta.Labels == nil {
		meta.Labels = map[string]string{}
	}
	addRookVersionLabel(meta.Labels)
}

func AddLabelToDeployment(key, value string, d *v1.Deployment) {
	if d == nil {
		return
	}
	if d.Labels == nil {
		d.Labels = map[string]string{}
	}
	addLabel(key, value, d.Labels)
}

func AddLabelToPod(key, value string, p *corev1.PodTemplateSpec) {
	if p == nil {
		return
	}
	if p.Labels == nil {
		p.Labels = map[string]string{}
	}
	addLabel(key, value, p.Labels)
}

func AddLabelToJob(key, value string, b *batchv1.Job) {
	if b == nil {
		return
	}
	if b.Labels == nil {
		b.Labels = map[string]string{}
	}
	addLabel(key, value, b.Labels)
}

func addLabel(key, value string, labels map[string]string) {
	labels[key] = value
}

func CreateDeployment(clientset kubernetes.Interface, name, namespace string, dep *apps.Deployment) error {
	_, err := clientset.AppsV1().Deployments(namespace).Create(dep)
	if err != nil {
		if k8serrors.IsAlreadyExists(err) {
			_, err = clientset.AppsV1().Deployments(namespace).Update(dep)
		}
		if err != nil {
			return fmt.Errorf("failed to start %s deployment: %+v\n%+v", name, err, dep)
		}
	}
	return err
}
