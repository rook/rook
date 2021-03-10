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
	"context"
	"fmt"
	"time"

	"github.com/banzaicloud/k8s-objectmatcher/patch"

	"github.com/rook/rook/pkg/clusterd"
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
	ctx := context.TODO()
	d, err := clientset.AppsV1().Deployments(namespace).Get(ctx, name, metav1.GetOptions{})
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
func UpdateDeploymentAndWait(clusterContext *clusterd.Context, modifiedDeployment *apps.Deployment, namespace string, verifyCallback func(action string) error) error {
	ctx := context.TODO()
	currentDeployment, err := clusterContext.Clientset.AppsV1().Deployments(namespace).Get(ctx, modifiedDeployment.Name, metav1.GetOptions{})
	if err != nil {
		return fmt.Errorf("failed to get deployment %s. %+v", modifiedDeployment.Name, err)
	}

	// Check whether the current deployment and newly generated one are identical
	patchChanged := false
	patchResult, err := patch.DefaultPatchMaker.Calculate(currentDeployment, modifiedDeployment)
	if err != nil {
		logger.Warningf("failed to calculate diff between current deployment %q and newly generated one. Assuming it changed. %v", currentDeployment.Name, err)
		patchChanged = true
	} else if !patchResult.IsEmpty() {
		patchChanged = true
	}

	if !patchChanged {
		logger.Infof("deployment %q did not change, nothing to update", currentDeployment.Name)
		return nil
	}

	// If deployments are different, let's update!
	logger.Infof("updating deployment %q after verifying it is safe to stop", modifiedDeployment.Name)

	// Let's verify the deployment can be stopped
	if err := verifyCallback("stop"); err != nil {
		return fmt.Errorf("failed to check if deployment %q can be updated. %v", modifiedDeployment.Name, err)
	}

	// Set hash annotation to the newly generated deployment
	if err := patch.DefaultAnnotator.SetLastAppliedAnnotation(modifiedDeployment); err != nil {
		return fmt.Errorf("failed to set hash annotation on deployment %q. %v", modifiedDeployment.Name, err)
	}

	if _, err := clusterContext.Clientset.AppsV1().Deployments(namespace).Update(ctx, modifiedDeployment, metav1.UpdateOptions{}); err != nil {
		return fmt.Errorf("failed to update deployment %q. %v", modifiedDeployment.Name, err)
	}

	if err := WaitForDeploymentToStart(clusterContext, currentDeployment); err != nil {
		return err
	}

	// Now we check if we can go to the next daemon
	if err := verifyCallback("continue"); err != nil {
		return fmt.Errorf("failed to check if deployment %q can continue: %v", modifiedDeployment.Name, err)
	}
	return nil
}

func WaitForDeploymentToStart(clusterdContext *clusterd.Context, deployment *v1.Deployment) error {
	// wait for the deployment to be restarted up to 300s
	sleepTime := 3
	attempts := 100
	for i := 0; i < attempts; i++ {
		// check for the status of the deployment
		d, err := clusterdContext.Clientset.AppsV1().Deployments(deployment.Namespace).Get(context.TODO(), deployment.Name, metav1.GetOptions{})
		if err != nil {
			return fmt.Errorf("failed to get deployment %q. %v", deployment.Name, err)
		}
		if d.Status.ObservedGeneration != deployment.Status.ObservedGeneration && d.Status.UpdatedReplicas > 0 && d.Status.ReadyReplicas > 0 {
			logger.Infof("finished waiting for updated deployment %q", d.Name)
			return nil
		}

		// If ProgressDeadlineExceeded is reached let's fail earlier
		// This can happen if one of the deployment cannot be scheduled on a node and stays in "pending" state
		for _, condition := range d.Status.Conditions {
			if condition.Type == v1.DeploymentProgressing && condition.Reason == "ProgressDeadlineExceeded" {
				return fmt.Errorf("gave up waiting for deployment %q to update because %q", deployment.Name, condition.Reason)
			}
		}

		logger.Debugf("deployment %q status=%+v", d.Name, d.Status)
		time.Sleep(time.Duration(sleepTime) * time.Second)
	}
	return fmt.Errorf("gave up waiting for deployment %q to update", deployment.Name)
}

// GetDeployments returns a list of deployment names labels matching a given selector
// example of a label selector might be "app=rook-ceph-mon, mon!=b"
// more: https://kubernetes.io/docs/concepts/overview/working-with-objects/labels/
func GetDeployments(clientset kubernetes.Interface, namespace, labelSelector string) (*apps.DeploymentList, error) {
	listOptions := metav1.ListOptions{LabelSelector: labelSelector}
	ctx := context.TODO()
	deployments, err := clientset.AppsV1().Deployments(namespace).List(ctx, listOptions)
	if err != nil {
		return nil, fmt.Errorf("failed to list deployments with labelSelector %s: %v", labelSelector, err)
	}
	return deployments, nil
}

// DeleteDeployment makes a best effort at deleting a deployment and its pods, then waits for them to be deleted
func DeleteDeployment(clientset kubernetes.Interface, namespace, name string) error {
	logger.Debugf("removing %s deployment if it exists", name)
	ctx := context.TODO()
	deleteAction := func(options *metav1.DeleteOptions) error {
		return clientset.AppsV1().Deployments(namespace).Delete(ctx, name, *options)
	}
	getAction := func() error {
		_, err := clientset.AppsV1().Deployments(namespace).Get(ctx, name, metav1.GetOptions{})
		return err
	}
	return deleteResourceAndWait(namespace, name, "deployment", deleteAction, getAction)
}

// WaitForDeploymentImage waits for all deployments with the given labels are running.
// WARNING:This is currently only useful for testing!
func WaitForDeploymentImage(clientset kubernetes.Interface, namespace, label, container string, initContainer bool, desiredImage string) error {
	ctx := context.TODO()
	sleepTime := 3
	attempts := 120
	for i := 0; i < attempts; i++ {
		deployments, err := clientset.AppsV1().Deployments(namespace).List(ctx, metav1.ListOptions{LabelSelector: label})
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
	ctx := context.TODO()
	_, err := clientset.AppsV1().Deployments(namespace).Create(ctx, dep, metav1.CreateOptions{})
	if err != nil {
		if k8serrors.IsAlreadyExists(err) {
			_, err = clientset.AppsV1().Deployments(namespace).Update(ctx, dep, metav1.UpdateOptions{})
		}
		if err != nil {
			return fmt.Errorf("failed to start %s deployment: %+v\n%+v", name, err, dep)
		}
	}
	return err
}
