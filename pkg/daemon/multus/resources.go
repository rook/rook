/*
Copyright 2023 The Rook Authors. All rights reserved.

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
	"context"
	"fmt"
	"time"

	core "k8s.io/api/core/v1"
	kerrors "k8s.io/apimachinery/pkg/api/errors"
	meta "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/wait"
)

type podNetworkInfo struct {
	// node the pod is running on
	nodeName string

	// multus public addr attached (if any)
	publicAddr string

	// multus cluster addr attached (if any)
	clusterAddr string
}

var manualCleanupInstructions = fmt.Sprintf(
	"manually delete owner configmap %q, and wait for all multus-validation-test resources to be deleted", ownerConfigMapName)

var previousTestSuggestion = "there could be a past test preventing this one from proceeding; " + manualCleanupInstructions

var unableToProvideAddressSuggestions = []string{
	"multus may be unable to provide addresses for pods",
	"check networking events on the pod and multus logs",
	"macvlan: NIC or switch hardware/software may block the association of some number of additional MAC addresses on an interface",
	"macvlan: interfaces and network switching must enable promiscuous mode to allow receiving packets for unknown (Multus) MACs",
	"macvlan/ipvlan: switch hardware/software may block an interface from receiving packets to an unknown (Multus) IP",
}

// create a validation test config object that stores the configuration of the running validation
// test. this object serves as the owner of all associated test objects. when this object is
// deleted, all validation test objects should also be deleted, effectively cleaning up all
// components of this test.
func (vt *ValidationTest) createOwningConfigMap(ctx context.Context) ([]meta.OwnerReference, error) {
	c := core.ConfigMap{
		ObjectMeta: meta.ObjectMeta{
			Name: ownerConfigMapName,
		},
	}

	configObject, err := vt.Clientset.CoreV1().ConfigMaps(vt.Namespace).Create(ctx, &c, meta.CreateOptions{})
	if err != nil {
		return nil, fmt.Errorf("failed to create validation test config object [%+v]: %w", c, err)
	}

	// for cleanup, we want to make sure all children are deleted
	BlockOwnerDeletion := true
	refToConfigObject := meta.OwnerReference{
		APIVersion:         "v1",
		Kind:               "ConfigMap",
		Name:               configObject.GetName(),
		UID:                configObject.GetUID(),
		BlockOwnerDeletion: &BlockOwnerDeletion,
	}
	return []meta.OwnerReference{refToConfigObject}, nil
}

func (vt *ValidationTest) startWebServer(ctx context.Context, owners []meta.OwnerReference) error {
	pod, err := vt.generateWebServerPod()
	if err != nil {
		return fmt.Errorf("failed to generate web server pod: %w", err)
	}
	pod.SetOwnerReferences(owners) // set owner refs so cleanup is easier

	configMap, err := vt.generateWebServerConfigMap()
	if err != nil {
		return fmt.Errorf("failed to generate web server config: %w", err)
	}
	configMap.SetOwnerReferences(owners) // set owner refs so cleanup is easier

	// create configmap before pod so pod doesn't crashloopbackoff on first creation
	_, err = vt.Clientset.CoreV1().ConfigMaps(vt.Namespace).Create(ctx, configMap, meta.CreateOptions{})
	if err != nil {
		return fmt.Errorf("failed to create web server config: %w", err)
	}

	_, err = vt.Clientset.CoreV1().Pods(vt.Namespace).Create(ctx, pod, meta.CreateOptions{})
	if err != nil {
		return fmt.Errorf("failed to create web server pod: %w", err)
	}

	return nil
}

func (vt *ValidationTest) getWebServerInfo(
	ctx context.Context,
	desiredPublicNet, desiredClusterNet *types.NamespacedName,
) (podNetworkInfo, []string, error) {
	podInfo := podNetworkInfo{}

	pod, err := vt.Clientset.CoreV1().Pods(vt.Namespace).Get(ctx, webServerPodName(), meta.GetOptions{})
	if err != nil {
		return podInfo, []string{}, fmt.Errorf("unexpected error when getting web server pod: %w", err)
	}

	var publicAddr, clusterAddr string
	publicAddr, clusterAddr, networkSuggestions, err := getNetworksFromPod(pod, desiredPublicNet, desiredClusterNet)
	if err != nil {
		return podInfo, networkSuggestions, fmt.Errorf("no web server network info: %w", err)
	}

	if !podIsReady(*pod) {
		return podInfo, []string{}, fmt.Errorf("web server pod is not ready yet")
	}

	podInfo.nodeName = pod.Spec.NodeName
	podInfo.publicAddr = publicAddr
	podInfo.clusterAddr = clusterAddr
	return podInfo, []string{}, nil // no suggestions if successful
}

func (vt *ValidationTest) startImagePullers(ctx context.Context, owners []meta.OwnerReference) error {
	ds, err := vt.generateImagePullDaemonSet()
	if err != nil {
		return fmt.Errorf("failed to generate image pull daemonset: %w", err)
	}
	ds.SetOwnerReferences(owners) // set owner so cleanup is easier

	_, err = vt.Clientset.AppsV1().DaemonSets(vt.Namespace).Create(ctx, ds, meta.CreateOptions{})
	if err != nil {
		return fmt.Errorf("failed to create image pull daemonset: %w", err)
	}

	return nil
}

func (vt *ValidationTest) deleteImagePullers(ctx context.Context) error {
	noGracePeriod := int64(0)
	delOpts := meta.DeleteOptions{
		GracePeriodSeconds: &noGracePeriod,
	}
	listOpts := meta.ListOptions{
		LabelSelector: imagePullAppLabel(),
	}
	err := vt.Clientset.AppsV1().DaemonSets(vt.Namespace).DeleteCollection(ctx, delOpts, listOpts)
	if err != nil {
		if kerrors.IsNotFound(err) {
			return nil // already deleted
		}
		return fmt.Errorf("failed to delete image pullers: %w", err)
	}

	return nil
}

func (vt *ValidationTest) startClients(
	ctx context.Context,
	owners []meta.OwnerReference,
	serverPublicAddr, serverClusterAddr string,
) error {
	for i := 0; i < vt.DaemonsPerNode; i++ {
		ds, err := vt.generateClientDaemonSet(i, serverPublicAddr, serverClusterAddr)
		if err != nil {
			return fmt.Errorf("failed to generate client daemonset for client #%d: %w", i, err)
		}
		ds.SetOwnerReferences(owners) // set owner refs so cleanup is easier

		_, err = vt.Clientset.AppsV1().DaemonSets(vt.Namespace).Create(ctx, ds, meta.CreateOptions{})
		if err != nil {
			return fmt.Errorf("failed to create client daemonset for client #%d: %w", i, err)
		}
	}

	return nil
}

func (vt *ValidationTest) getExpectedNumberOfDaemonSetPods(
	ctx context.Context,
	daemonsetSelectorLabel string,
	expectedNumDaemonsets int,
) (int, error) {
	var agreedNumberScheduled int32
	listOpts := meta.ListOptions{
		LabelSelector: daemonsetSelectorLabel,
	}
	dsets, err := vt.Clientset.AppsV1().DaemonSets(vt.Namespace).List(ctx, listOpts)
	if err != nil {
		return 0, fmt.Errorf("unexpected error listing daemonsets: %w", err)
	}
	if len(dsets.Items) != expectedNumDaemonsets {
		return 0, fmt.Errorf("got %d daemonsets when %d should exist", len(dsets.Items), expectedNumDaemonsets)
	}

	for _, d := range dsets.Items {
		numScheduled := d.Status.CurrentNumberScheduled
		if numScheduled == 0 {
			return 0, fmt.Errorf("a daemonset expects zero scheduled pods")
		}
		if agreedNumberScheduled == 0 {
			agreedNumberScheduled = numScheduled
		} else if numScheduled != agreedNumberScheduled {
			return 0, fmt.Errorf("daemonsets do not all agree on the number of expected pods")
		}
	}

	return int(agreedNumberScheduled) * expectedNumDaemonsets, nil
}

func (vt *ValidationTest) getNumRunningPods(
	ctx context.Context,
	podSelectorLabel string,
	expectedNumTotalPods int,
) (int, error) {
	listOpts := meta.ListOptions{
		LabelSelector: podSelectorLabel,
	}
	pods, err := vt.Clientset.CoreV1().Pods(vt.Namespace).List(ctx, listOpts)
	if err != nil {
		return 0, fmt.Errorf("failed to list pods: %w", err)
	}
	if len(pods.Items) != expectedNumTotalPods {
		return 0, fmt.Errorf("got %d pods when %d should exist", len(pods.Items), expectedNumTotalPods)
	}

	numRunning := 0
	for _, p := range pods.Items {
		if podIsRunning(p) {
			numRunning++
		}
	}

	return numRunning, nil
}

func (vt *ValidationTest) numClientsReady(ctx context.Context, expectedNumPods int) (int, error) {
	pods, err := vt.getClientPods(ctx, expectedNumPods)
	if err != nil {
		return 0, fmt.Errorf("unexpected error getting client pods: %w", err)
	}
	numReady := 0
	for _, p := range pods.Items {
		if podIsReady(p) {
			numReady++
		}
	}
	return numReady, nil
}

func (vt *ValidationTest) getClientPods(ctx context.Context, expectedNumPods int) (*core.PodList, error) {
	listOpts := meta.ListOptions{
		LabelSelector: clientAppLabel(),
	}
	pods, err := vt.Clientset.CoreV1().Pods(vt.Namespace).List(ctx, listOpts)
	if err != nil {
		return nil, fmt.Errorf("failed to list client pods: %w", err)
	}
	if len(pods.Items) != expectedNumPods {
		return nil, fmt.Errorf("the number of pods listed [%d] does not match the number expected [%d]", len(pods.Items), expectedNumPods)
	}
	return pods, err
}

func (vt *ValidationTest) cleanUpTestResources() (string, error) {
	// need a clean, non-canceled context in case the test is canceled by ctrl-c
	ctx := context.Background()

	// delete the config object in the foreground so we wait until all validation test resources are
	// gone before stopping, and do it now because there's no need to wait for just a test
	var gracePeriodZero int64 = 0
	deleteForeground := meta.DeletePropagationForeground
	delOpts := meta.DeleteOptions{
		PropagationPolicy:  &deleteForeground,
		GracePeriodSeconds: &gracePeriodZero,
	}
	err := vt.Clientset.CoreV1().ConfigMaps(vt.Namespace).Delete(ctx, ownerConfigMapName, delOpts)
	if err != nil {
		if !kerrors.IsNotFound(err) {
			return manualCleanupInstructions, fmt.Errorf("failed to clean up multus validation test resources: %w", err)
		}
		return "", nil
	}

	// clients take a long time to terminate, and the 0 grace period set on the configmap doesn't
	// propagate to dependents. make a best-effort attempt to delete client pods with 0 grace period
	listOpts := meta.ListOptions{
		LabelSelector: clientAppLabel(),
	}
	// ignore errors for a best-effort attempt, they will delete eventually
	_ = vt.Clientset.CoreV1().Pods(vt.Namespace).DeleteCollection(ctx, delOpts, listOpts)

	// wait for resources to be cleaned up
	ctx, cancel := context.WithTimeout(ctx, vt.ResourceTimeout)
	defer cancel()
	lastSuggestion := ""
	err = wait.PollUntilContextCancel(ctx, 2*time.Second, true, func(ctx context.Context) (done bool, err error) {
		_, getErr := vt.Clientset.CoreV1().ConfigMaps(vt.Namespace).Get(ctx, ownerConfigMapName, meta.GetOptions{})
		if getErr != nil {
			if kerrors.IsNotFound(getErr) {
				return true, nil
			}
			lastSuggestion = fmt.Sprintf("unexpected error when cleaning up multus validation test resources; attempting to continue: %v", err)
		}
		return false, nil
	})
	if err != nil {
		return lastSuggestion + "; " + manualCleanupInstructions,
			fmt.Errorf("failed waiting for multus validation test resources to be deleted: %w", err)
	}

	return "", nil
}
