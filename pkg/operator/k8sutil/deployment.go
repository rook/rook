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
	"github.com/pkg/errors"
	"github.com/rook/rook/pkg/clusterd"
	"github.com/rook/rook/pkg/util"
	appsv1 "k8s.io/api/apps/v1"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
)

var (
	waitForDeploymentPeriod  = 2 * time.Second
	waitForDeploymentTimeout = 60 * time.Second
)

// GetDeploymentImage returns the version of the image running in the pod spec for the desired container
func GetDeploymentImage(ctx context.Context, clientset kubernetes.Interface, namespace, name, container string) (string, error) {
	d, err := clientset.AppsV1().Deployments(namespace).Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		return "", fmt.Errorf("failed to find deployment %s. %v", name, err)
	}
	return GetDeploymentSpecImage(clientset, *d, container, false)
}

// GetDeploymentSpecImage returns the image name from the spec
func GetDeploymentSpecImage(clientset kubernetes.Interface, d appsv1.Deployment, container string, initContainer bool) (string, error) {
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
func UpdateDeploymentAndWait(ctx context.Context, clusterContext *clusterd.Context, modifiedDeployment *appsv1.Deployment, namespace string, verifyCallback func(action string) error) error {
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

	if err := WaitForDeploymentToStart(ctx, clusterContext, currentDeployment); err != nil {
		return err
	}

	// Now we check if we can go to the next daemon
	if err := verifyCallback("continue"); err != nil {
		return fmt.Errorf("failed to check if deployment %q can continue: %v", modifiedDeployment.Name, err)
	}
	return nil
}

func WaitForDeploymentToStart(ctx context.Context, clusterdContext *clusterd.Context, deployment *appsv1.Deployment) error {
	// wait for the deployment to be restarted up to 300s
	sleepTime := 3
	attempts := 100
	for i := 0; i < attempts; i++ {
		// check for the status of the deployment
		d, err := clusterdContext.Clientset.AppsV1().Deployments(deployment.Namespace).Get(ctx, deployment.Name, metav1.GetOptions{})
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
			if condition.Type == appsv1.DeploymentProgressing && condition.Reason == "ProgressDeadlineExceeded" {
				return fmt.Errorf("gave up waiting for deployment %q to update because %q", deployment.Name, condition.Reason)
			}
		}

		logger.Debugf("deployment %q status=%+v", d.Name, d.Status)
		time.Sleep(time.Duration(sleepTime) * time.Second)
	}
	return fmt.Errorf("gave up waiting for deployment %q to update", deployment.Name)
}

// DeploymentNames returns a list of the names of deployments in the deployment list
func DeploymentNames(deployments *appsv1.DeploymentList) (names []string) {
	for _, d := range deployments.Items {
		names = append(names, d.Name)
	}
	return names
}

// DeploymentsUpdated is a mapping from deployment name to the observed generation of the old
// deployment which was updated.
type DeploymentsUpdated map[string]int64

// Names returns the names of the deployments which were updated.
func (d *DeploymentsUpdated) Names() (names []string) {
	for name := range *d {
		names = append(names, name)
	}
	return names
}

type Failure struct {
	ResourceName string
	Error        error
}

type Failures []Failure

func (failures *Failures) CollatedErrors() error {
	var err error
	for _, f := range *failures {
		err = fmt.Errorf("%v. %v", f.Error, err)
	}
	return err
}

// UpdateMultipleDeployments updates multiple deployments and returns DeploymentsUpdated map of
// deployment names which were updated successfully and the observed generation of the deployments
// before they were updated. If deployments are already up to date, they are not reported in the
// DeploymentsUpdated map.
// The DeploymentsUpdated map can be used with the WaitForDeploymentsToUpdate function.
// Also returns a list of failures. Each failure returned includes the name of the deployment which
// could not be updated and the error experienced when attempting to update the deployment.
func UpdateMultipleDeployments(
	ctx context.Context,
	clientset kubernetes.Interface,
	deployments []*appsv1.Deployment,
) (DeploymentsUpdated, Failures, *int32) {
	deploymentsUpdated := DeploymentsUpdated{}
	failures := Failures{}
	var maxProgressDeadlineSeconds *int32

	for _, dep := range deployments {
		oldDep, newDep, err := updateDeployment(ctx, clientset, dep)
		if err != nil {
			failures = append(failures, Failure{
				ResourceName: dep.Name,
				Error:        errors.Wrapf(err, "failed to update deployment %q", dep.Name),
			})
			continue
		}

		if newDep == nil {
			// deployment was not updated
			continue
		}

		deploymentsUpdated[newDep.Name] = oldDep.Status.ObservedGeneration

		maxProgressDeadlineSeconds = maxInt32Ptr(maxProgressDeadlineSeconds, oldDep.Spec.ProgressDeadlineSeconds)
		maxProgressDeadlineSeconds = maxInt32Ptr(maxProgressDeadlineSeconds, newDep.Spec.ProgressDeadlineSeconds)
	}

	return deploymentsUpdated, failures, maxProgressDeadlineSeconds
}

// WaitForDeploymentsToUpdate waits for all deployments to update. It returns a list of failures.
// Each failure includes the name of the deployment which was not updated within the timeout and an
// error indicating the reason why.
func WaitForDeploymentsToUpdate(
	deploymentsUpdated DeploymentsUpdated,
	progressDeadlineSeconds *int32,
	listFunc func() (*appsv1.DeploymentList, error),
) Failures {
	// do not modify the input!
	waitingOn := DeploymentsUpdated{}
	for k, v := range deploymentsUpdated {
		waitingOn[k] = v
	}
	failures := Failures{}

	waitFunc := func() (done bool, err error) {
		deployments, err := listFunc()
		if err != nil {
			return false, errors.Wrap(err, "failed to list deployments")
		}
		if len(deployments.Items) < len(waitingOn) {
			// This could be because the listFunc is written incorrectly
			logger.Warningf(
				"number of deployments listed (%d) is less than number of deployments we are waiting on to be updated (%d). listed: %+v. waiting on: %+v. ",
				len(deployments.Items), len(waitingOn), DeploymentNames(deployments), waitingOn.Names())
		}

		for i, dep := range deployments.Items { // deployment loop
			oldGeneration, ok := waitingOn[dep.Name]
			if !ok {
				// we are not waiting on this deployment to finish
				continue
			}

			// If ProgressDeadlineExceeded is reached, fail earlier. This can happen if a deployment
			// cannot be scheduled on a node and stays in "pending" state.
			// Index deployment to prevent implicit memory aliasing
			if err := progressDeadlineExceeded(&deployments.Items[i]); err != nil {
				failures = append(failures, Failure{
					ResourceName: dep.Name,
					Error:        errors.Wrapf(err, "progress deadline exceeded on deployment %q", dep.Name),
				})
				delete(waitingOn, dep.Name) // don't keep waiting on this deployment
				continue                    // deployment loop
			}

			// Index deployment to prevent implicit memory aliasing
			if deploymentIsDoneUpdating(&deployments.Items[i], oldGeneration) {
				delete(waitingOn, dep.Name) // done waiting on this deployment
			}
		}

		if len(waitingOn) == 0 {
			return true, nil
		}

		return false, nil
	}

	timeout := waitForDeploymentTimeout
	if progressDeadlineSeconds != nil {
		// make the timeout double the progress deadline since the pod is both stopping and starting
		timeout = 2 * time.Duration(*progressDeadlineSeconds) * time.Second
	}

	err := util.RetryWithTimeout(waitFunc, waitForDeploymentPeriod, timeout, "deployments to be updated")
	if err != nil {
		// the retry function doesn't return (true, error), so this must be a timeout error
		logger.Errorf("%v", err)
	}

	// process remaining items in the waitingOn list to mark which deployments timed out
	for depName := range waitingOn {
		failures = append(failures, Failure{
			ResourceName: depName,
			Error:        errors.Errorf("timed out waiting on deployment %q", depName),
		})
	}

	return failures
}

func UpdateMultipleDeploymentsAndWait(
	ctx context.Context,
	clientset kubernetes.Interface,
	deployments []*appsv1.Deployment,
	listFunc func() (*appsv1.DeploymentList, error),
) Failures {
	depsUpdated, updateFailures, maxProgressDeadline := UpdateMultipleDeployments(ctx, clientset, deployments)
	waitFailures := WaitForDeploymentsToUpdate(depsUpdated, maxProgressDeadline, listFunc)

	return append(updateFailures, waitFailures...)
}

func deploymentIsDoneUpdating(d *appsv1.Deployment, oldObservedGeneration int64) bool {
	return d.Status.ObservedGeneration != oldObservedGeneration && d.Status.UpdatedReplicas > 0 && d.Status.ReadyReplicas > 0
}

// return error if progress deadline exceeded condition on the deployment
func progressDeadlineExceeded(d *appsv1.Deployment) error {
	for _, condition := range d.Status.Conditions {
		if condition.Type == appsv1.DeploymentProgressing && condition.Reason == "ProgressDeadlineExceeded" {
			return fmt.Errorf("gave up waiting for deployment %q to update because %q", d.Name, condition.Reason)
		}
	}
	return nil
}

func updateDeployment(
	ctx context.Context,
	clientset kubernetes.Interface,
	deployment *appsv1.Deployment,
) (oldDeployment, newDeployment *appsv1.Deployment, err error) {
	namespace := deployment.Namespace

	oldDeployment, err = clientset.AppsV1().Deployments(namespace).Get(ctx, deployment.Name, metav1.GetOptions{})
	if err != nil {
		return nil, nil, errors.Wrapf(err, "failed to get existing deployment %q", deployment.Name)
	}

	// Check whether the current deployment and newly generated one are identical
	patchChanged := false
	patchResult, err := patch.DefaultPatchMaker.Calculate(oldDeployment, deployment)
	if err != nil {
		logger.Warningf("failed to calculate diff between current deployment %q and newly generated one. assuming it changed. %v", oldDeployment.Name, err)
		patchChanged = true
	} else if !patchResult.IsEmpty() {
		patchChanged = true
	}

	if patchChanged {
		// Set hash annotation to the newly generated deployment
		err := patch.DefaultAnnotator.SetLastAppliedAnnotation(deployment)
		if err != nil {
			return nil, nil, errors.Wrapf(err, "failed to set hash annotation on deployment %q", deployment.Name)
		}

		newDeployment, err := clientset.AppsV1().Deployments(namespace).Update(ctx, deployment, metav1.UpdateOptions{})
		if err != nil {
			return nil, nil, errors.Wrapf(err, "failed to update deployment %q", deployment.Name)
		}

		return oldDeployment, newDeployment, nil
	}

	logger.Debugf("deployment %q did not change. nothing to update", deployment.Name)
	return oldDeployment, nil, nil
}

// GetDeployments returns a list of deployment names labels matching a given selector
// example of a label selector might be "app=rook-ceph-mon, mon!=b"
// more: https://kubernetes.io/docs/concepts/overview/working-with-objects/labels/
func GetDeployments(ctx context.Context, clientset kubernetes.Interface, namespace, labelSelector string) (*appsv1.DeploymentList, error) {
	listOptions := metav1.ListOptions{LabelSelector: labelSelector}
	deployments, err := clientset.AppsV1().Deployments(namespace).List(ctx, listOptions)
	if err != nil {
		return nil, fmt.Errorf("failed to list deployments with labelSelector %s: %v", labelSelector, err)
	}
	return deployments, nil
}

// DeleteDeployment makes a best effort at deleting a deployment and its pods, then waits for them to be deleted
func DeleteDeployment(ctx context.Context, clientset kubernetes.Interface, namespace, name string) error {
	logger.Debugf("removing %s deployment if it exists", name)
	deleteAction := func(options *metav1.DeleteOptions) error {
		return clientset.AppsV1().Deployments(namespace).Delete(ctx, name, *options)
	}
	getAction := func() error {
		_, err := clientset.AppsV1().Deployments(namespace).Get(ctx, name, metav1.GetOptions{})
		return err
	}
	return deleteResourceAndWait(namespace, name, "deployment", deleteAction, getAction)
}

// GetDeploymentOwnerReference returns an OwnerReference to the deployment that is running the given pod name
func GetDeploymentOwnerReference(ctx context.Context, clientset kubernetes.Interface, podName, namespace string) (*metav1.OwnerReference, error) {
	var deploymentRef *metav1.OwnerReference
	pod, err := clientset.CoreV1().Pods(namespace).Get(ctx, podName, metav1.GetOptions{})
	if err != nil {
		return nil, errors.Wrapf(err, "could not find pod %q in namespace %q to find deployment owner reference", podName, namespace)
	}
	for _, podOwner := range pod.OwnerReferences {
		if podOwner.Kind == "ReplicaSet" {
			replicaset, err := clientset.AppsV1().ReplicaSets(namespace).Get(ctx, podOwner.Name, metav1.GetOptions{})
			if err != nil {
				return nil, errors.Wrapf(err, "could not find replicaset %q in namespace %q to find deployment owner reference", podOwner.Name, namespace)
			}
			for _, replicasetOwner := range replicaset.OwnerReferences {
				if replicasetOwner.Kind == "Deployment" {
					localreplicasetOwner := replicasetOwner
					deploymentRef = &localreplicasetOwner
				}
			}
		}
	}
	if deploymentRef == nil {
		return nil, errors.New("could not find owner reference for rook-ceph deployment")
	}
	return deploymentRef, nil
}

// WaitForDeploymentImage waits for all deployments with the given labels are running.
// WARNING:This is currently only useful for testing!
func WaitForDeploymentImage(ctx context.Context, clientset kubernetes.Interface, namespace, label, container string, initContainer bool, desiredImage string) error {
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
func AddRookVersionLabelToDeployment(d *appsv1.Deployment) {
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

func AddLabelToDeployment(key, value string, d *appsv1.Deployment) {
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

// CreateDeployment creates a deployment with a last applied hash annotation added
func CreateDeployment(ctx context.Context, clientset kubernetes.Interface, dep *appsv1.Deployment) (*appsv1.Deployment, error) {
	// Set hash annotation to the newly generated deployment
	err := patch.DefaultAnnotator.SetLastAppliedAnnotation(dep)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to set hash annotation on deployment %q", dep.Name)
	}

	return clientset.AppsV1().Deployments(dep.Namespace).Create(ctx, dep, metav1.CreateOptions{})
}

func CreateOrUpdateDeployment(ctx context.Context, clientset kubernetes.Interface, dep *appsv1.Deployment) (*appsv1.Deployment, error) {
	newDep, err := CreateDeployment(ctx, clientset, dep)
	if err != nil {
		if k8serrors.IsAlreadyExists(err) {
			// annotation was added in CreateDeployment to dep passed by reference
			newDep, err = clientset.AppsV1().Deployments(dep.Namespace).Update(ctx, dep, metav1.UpdateOptions{})
		}
		if err != nil {
			return nil, errors.Wrapf(err, "failed to create or update deployment %q: %+v", dep.Name, dep)
		}
	}
	return newDep, nil
}

func maxInt32Ptr(a, b *int32) *int32 {
	var ret int32
	if a == nil && b == nil {
		return nil
	}
	if a == nil && b != nil {
		ret = *b
		return &ret
	}
	if a != nil && b == nil {
		ret = *a
		return &ret
	}
	if *b > *a {
		ret = *b
		return &ret
	}
	ret = *a
	return &ret
}
