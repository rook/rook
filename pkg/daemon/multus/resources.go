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
	placement, err := vt.BestNodePlacementForServer()
	if err != nil {
		return fmt.Errorf("failed to place web server pod: %w", err)
	}

	// infer good placement for web server pod from the node type with the most OSDs
	pod, err := vt.generateWebServerPod(placement)
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
	for typeName, nodeType := range vt.NodeTypes {
		ds, err := vt.generateImagePullDaemonSet(typeName, nodeType.Placement)
		if err != nil {
			return fmt.Errorf("failed to generate image pull daemonset: %w", err)
		}
		ds.SetOwnerReferences(owners) // set owner so cleanup is easier

		_, err = vt.Clientset.AppsV1().DaemonSets(vt.Namespace).Create(ctx, ds, meta.CreateOptions{})
		if err != nil {
			return fmt.Errorf("failed to create image pull daemonset: %w", err)
		}
	}

	return nil
}

func (vt *ValidationTest) deleteDaemonsetsWithLabel(ctx context.Context, label string) error {
	noGracePeriod := int64(0)
	delOpts := meta.DeleteOptions{
		GracePeriodSeconds: &noGracePeriod,
	}
	listOpts := meta.ListOptions{
		LabelSelector: label,
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

func (vt *ValidationTest) startHostCheckers(
	ctx context.Context,
	owners []meta.OwnerReference,
	serverPublicAddr string,
) error {
	for typeName, nodeType := range vt.NodeTypes {
		ds, err := vt.generateHostCheckerDaemonSet(serverPublicAddr, typeName, nodeType.Placement)
		if err != nil {
			return fmt.Errorf("failed to generate host checker daemonset: %w", err)
		}
		ds.SetOwnerReferences(owners) // set owner so cleanup is easier

		_, err = vt.Clientset.AppsV1().DaemonSets(vt.Namespace).Create(ctx, ds, meta.CreateOptions{})
		if err != nil {
			return fmt.Errorf("failed to create host checker daemonset: %w", err)
		}
	}

	return nil
}

func (vt *ValidationTest) startClients(
	ctx context.Context,
	owners []meta.OwnerReference,
	serverPublicAddr, serverClusterAddr string,
	nodeType string,
) (int, error) {
	numDaemonsetsCreated := 0

	nodeConfig := vt.NodeTypes[nodeType]

	// start clients that simulate OSDs (connected to both public and cluster nets)
	osdsPerNode := nodeConfig.OSDsPerNode
	vt.Logger.Infof("starting %d %s validation clients for node type %q", osdsPerNode, ClientTypeOSD, nodeType)
	for i := 0; i < osdsPerNode; i++ {
		attachToClusterNet := true
		ds, err := vt.generateClientDaemonSet(true, attachToClusterNet, serverPublicAddr, serverClusterAddr, nodeType, ClientTypeOSD, i, nodeConfig.Placement)
		if err != nil {
			return numDaemonsetsCreated, fmt.Errorf("failed to generate client daemonset for node type %q, client type %q, client #%d: %w", nodeType, ClientTypeOSD, i, err)
		}
		ds.SetOwnerReferences(owners) // set owner refs so cleanup is easier

		_, err = vt.Clientset.AppsV1().DaemonSets(vt.Namespace).Create(ctx, ds, meta.CreateOptions{})
		if err != nil {
			return numDaemonsetsCreated, fmt.Errorf("failed to create client daemonset for node type %q, client type %q, client #%d: %w", nodeType, ClientTypeOSD, i, err)
		}
		numDaemonsetsCreated++
	}

	// start clients that simulate non-OSD daemons (connected only to public net)
	if serverPublicAddr == "" {
		return numDaemonsetsCreated, nil // no public net; thus, no public-net-only clients to run
	}
	otherPerNode := nodeConfig.OtherDaemonsPerNode
	vt.Logger.Infof("starting %d %s (non-OSD) validation clients for node type %q", otherPerNode, ClientTypeNonOSD, nodeType)
	for i := 0; i < otherPerNode; i++ {
		attachToClusterNet := false
		ds, err := vt.generateClientDaemonSet(true, attachToClusterNet, serverPublicAddr, serverClusterAddr, nodeType, ClientTypeNonOSD, i, nodeConfig.Placement)
		if err != nil {
			return numDaemonsetsCreated, fmt.Errorf("failed to generate client daemonset for node type %q, client type %q, client #%d: %w", nodeType, ClientTypeNonOSD, i, err)
		}
		ds.SetOwnerReferences(owners) // set owner refs so cleanup is easier

		_, err = vt.Clientset.AppsV1().DaemonSets(vt.Namespace).Create(ctx, ds, meta.CreateOptions{})
		if err != nil {
			return numDaemonsetsCreated, fmt.Errorf("failed to create client daemonset for node type %q, client type %q, client #%d: %w", nodeType, ClientTypeNonOSD, i, err)
		}
		numDaemonsetsCreated++
	}

	return numDaemonsetsCreated, nil
}

type perNodeTypeCount map[string]int

func (a *perNodeTypeCount) Increment(nodeType string) {
	current, ok := (*a)[nodeType]
	if !ok {
		current = 0
	}
	(*a)[nodeType] = current + 1
}

func (a *perNodeTypeCount) Total() int {
	t := 0
	for _, c := range *a {
		t += c
	}
	return t
}

func (a *perNodeTypeCount) Equal(b *perNodeTypeCount) bool {
	if len(*a) != len(*b) {
		return false
	}
	for nodeType, numA := range *a {
		numB, ok := (*b)[nodeType]
		if !ok {
			return false
		}
		if numA != numB {
			return false
		}
	}
	return true
}

func (vt *ValidationTest) getImagePullPodCountPerNodeType(
	ctx context.Context,
) (perNodeTypeCount, error) {
	emptyCount := perNodeTypeCount{}
	listOpts := meta.ListOptions{
		LabelSelector: imagePullAppLabel(),
	}
	dsets, err := vt.Clientset.AppsV1().DaemonSets(vt.Namespace).List(ctx, listOpts)
	if err != nil {
		return emptyCount, fmt.Errorf("unexpected error listing daemonsets: %w", err)
	}
	expectedNumDaemonsets := len(vt.NodeTypes)
	if len(dsets.Items) != expectedNumDaemonsets {
		return emptyCount, fmt.Errorf("got %d daemonsets when %d should exist", len(dsets.Items), expectedNumDaemonsets)
	}

	numsScheduled := perNodeTypeCount{}
	for i, d := range dsets.Items {
		nodeType := getNodeType(&dsets.Items[i].ObjectMeta)
		numScheduled := d.Status.CurrentNumberScheduled
		if numScheduled == 0 {
			return emptyCount, fmt.Errorf("image pull daemonset for node type %q expects zero scheduled pods", nodeType)
		}
		numsScheduled[nodeType] = int(numScheduled)
	}

	return numsScheduled, nil
}

func (vt *ValidationTest) ensureOneImagePullPodPerNode(ctx context.Context) error {
	listOpts := meta.ListOptions{
		LabelSelector: imagePullAppLabel(),
	}
	pods, err := vt.Clientset.CoreV1().Pods(vt.Namespace).List(ctx, listOpts)
	if err != nil {
		return fmt.Errorf("failed to list pods: %w", err)
	}

	nodesFound := map[string]string{}
	for _, p := range pods.Items {
		nodeName := p.Spec.NodeName
		nodeType := p.GetLabels()["nodeType"]
		if otherNodeType, ok := nodesFound[nodeName]; ok {
			return fmt.Errorf("node types must not overlap: node type %q has overlap with node type %q", nodeType, otherNodeType)
		}
		nodesFound[nodeName] = nodeType
	}

	return nil
}

func (vt *ValidationTest) getNumRunningPods(
	ctx context.Context,
	podSelectorLabel string,
) (int, error) {
	listOpts := meta.ListOptions{
		LabelSelector: podSelectorLabel,
	}
	pods, err := vt.Clientset.CoreV1().Pods(vt.Namespace).List(ctx, listOpts)
	if err != nil {
		return 0, fmt.Errorf("failed to list pods: %w", err)
	}

	numRunning := 0
	for _, p := range pods.Items {
		if podIsRunning(p) {
			numRunning++
		}
	}

	return numRunning, nil
}

func (vt *ValidationTest) numPodsReadyWithLabel(ctx context.Context, label string) (int, error) {
	pods, err := vt.getPodsWithLabel(ctx, label)
	if err != nil {
		return 0, fmt.Errorf("unexpected error getting pods with label %q: %w", label, err)
	}
	numReady := 0
	for _, p := range pods.Items {
		if podIsReady(p) {
			numReady++
		}
	}
	return numReady, nil
}

func (vt *ValidationTest) getPodsWithLabel(ctx context.Context, label string) (*core.PodList, error) {
	listOpts := meta.ListOptions{
		LabelSelector: label,
	}
	pods, err := vt.Clientset.CoreV1().Pods(vt.Namespace).List(ctx, listOpts)
	if err != nil {
		return nil, fmt.Errorf("failed to list pods with label %q: %w", label, err)
	}
	return pods, err
}

func (vt *ValidationTest) cleanUpTestResources() (string, error) {
	// need a clean, non-canceled context in case the test is canceled by ctrl-c
	ctx := context.Background()

	// delete the config object in the foreground so we wait until all validation test resources are
	// gone before stopping
	deleteForeground := meta.DeletePropagationForeground
	delOpts := meta.DeleteOptions{
		PropagationPolicy: &deleteForeground,
		// GracePeriodSeconds // leave at default; do not force delete, which can exhaust CNI IPAM addresses
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
