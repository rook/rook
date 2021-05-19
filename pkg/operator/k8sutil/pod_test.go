/*
Copyright 2016 The Rook Authors. All rights reserved.

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
	"os"
	"testing"

	cephv1 "github.com/rook/rook/pkg/apis/ceph.rook.io/v1"
	"github.com/rook/rook/pkg/operator/test"
	"github.com/stretchr/testify/assert"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestGetContainerInPod(t *testing.T) {
	expectedName := "mycontainer"
	imageName := "myimage"

	// no container fails
	_, err := GetMatchingContainer([]v1.Container{}, expectedName)
	assert.NotNil(t, err)

	// one container will allow any name
	container, err := GetMatchingContainer([]v1.Container{{Name: "foo", Image: imageName}}, expectedName)
	assert.Nil(t, err)
	assert.Equal(t, imageName, container.Image)

	// multiple container fails if we don't find the correct name
	container, err = GetMatchingContainer(
		[]v1.Container{{Name: "foo", Image: imageName}, {Name: "bar", Image: imageName}},
		expectedName)
	assert.NotNil(t, err)

	// multiple container succeeds if we find the correct name
	container, err = GetMatchingContainer(
		[]v1.Container{{Name: "foo", Image: imageName}, {Name: expectedName, Image: imageName}},
		expectedName)
	assert.Nil(t, err)
	assert.Equal(t, imageName, container.Image)
}

func TestGetPodPhaseMap(t *testing.T) {
	// empty pod list should result in empty pod phase map
	pods := &v1.PodList{Items: []v1.Pod{}}
	podPhaseMap := GetPodPhaseMap(pods)
	assert.Equal(t, 0, len(podPhaseMap))

	// 2 running pods, 1 failed pod
	pods = &v1.PodList{
		Items: []v1.Pod{
			{ObjectMeta: metav1.ObjectMeta{Name: "pod1"}, Status: v1.PodStatus{Phase: v1.PodRunning}},
			{ObjectMeta: metav1.ObjectMeta{Name: "pod2"}, Status: v1.PodStatus{Phase: v1.PodRunning}},
			{ObjectMeta: metav1.ObjectMeta{Name: "pod3"}, Status: v1.PodStatus{Phase: v1.PodFailed}},
		},
	}
	podPhaseMap = GetPodPhaseMap(pods)

	// map should have 2 entries, 1 list of running pods and 1 list of failed pods
	assert.Equal(t, 2, len(podPhaseMap))

	// list of running pods should have 2 entries
	assert.Equal(t, 2, len(podPhaseMap[v1.PodRunning]))

	// list of failed pods should have 1 entry
	assert.Equal(t, 1, len(podPhaseMap[v1.PodFailed]))
}

func newToleration(defaultSeconds int64, tolerationKey string) v1.Toleration {
	return v1.Toleration{Key: tolerationKey,
		Operator:          "Exists",
		Effect:            "NoExecute",
		TolerationSeconds: &defaultSeconds}
}

func TestAddUnreachableNodeToleration(t *testing.T) {
	podSpec := v1.PodSpec{}

	// -------------------------------------------------------------------------
	// Test one toleration of 5 seconds
	expectedURToleration := newToleration(5, "node.kubernetes.io/unreachable")

	// Change the UR toleration in the pod using env var and the tested function
	os.Setenv("ROOK_UNREACHABLE_NODE_TOLERATION_SECONDS", "5")
	AddUnreachableNodeToleration(&podSpec)

	assert.Equal(t, 1, len(podSpec.Tolerations))
	assert.Equal(t, expectedURToleration, podSpec.Tolerations[0])

	//--------------------------------------------------------------------------
	// Test adding one additional toleration, replaces the previous one,
	// keeping only the last.
	expectedURToleration = newToleration(6, "node.kubernetes.io/unreachable")

	// Change the UR toleration in the pod using env var and the tested function
	os.Setenv("ROOK_UNREACHABLE_NODE_TOLERATION_SECONDS", "6")
	AddUnreachableNodeToleration(&podSpec)

	assert.Equal(t, 1, len(podSpec.Tolerations))
	assert.Equal(t, expectedURToleration, podSpec.Tolerations[0])

	//--------------------------------------------------------------------------
	// Changing the toleration at the beginning of the list
	urTol := newToleration(10, "node.kubernetes.io/unreachable")
	otherTol := newToleration(20, "node.kubernetes.io/network-unavailable")

	podSpec.Tolerations = nil
	podSpec.Tolerations = append(podSpec.Tolerations, urTol, otherTol)

	expectedURToleration = newToleration(7, "node.kubernetes.io/unreachable")

	// Change the Unreachable node toleration
	os.Setenv("ROOK_UNREACHABLE_NODE_TOLERATION_SECONDS", "7")
	AddUnreachableNodeToleration(&podSpec)

	assert.Equal(t, 2, len(podSpec.Tolerations))
	assert.Equal(t, expectedURToleration, podSpec.Tolerations[0])

	//--------------------------------------------------------------------------
	// Changing the toleration at the middle of the list
	podSpec.Tolerations = nil
	podSpec.Tolerations = append(podSpec.Tolerations, otherTol, urTol, otherTol)

	expectedURToleration = newToleration(8, "node.kubernetes.io/unreachable")

	// Change the Unreachable node toleration
	os.Setenv("ROOK_UNREACHABLE_NODE_TOLERATION_SECONDS", "8")
	AddUnreachableNodeToleration(&podSpec)

	assert.Equal(t, 3, len(podSpec.Tolerations))
	assert.Equal(t, expectedURToleration, podSpec.Tolerations[1])

	//--------------------------------------------------------------------------
	// Changing the toleration at the end of the list
	podSpec.Tolerations = nil
	podSpec.Tolerations = append(podSpec.Tolerations, otherTol, urTol)

	expectedURToleration = newToleration(9, "node.kubernetes.io/unreachable")

	// Change the Unreachable node toleration
	os.Setenv("ROOK_UNREACHABLE_NODE_TOLERATION_SECONDS", "9")
	AddUnreachableNodeToleration(&podSpec)

	assert.Equal(t, 2, len(podSpec.Tolerations))
	assert.Equal(t, expectedURToleration, podSpec.Tolerations[1])

	// Environment var with wrong value format results in using default value
	podSpec.Tolerations = nil

	// The default value used for the Unreachable Node Toleration is 5 seconds
	expectedURToleration = newToleration(5, "node.kubernetes.io/unreachable")

	// Change the Unreachable node toleration using wrong format
	os.Setenv("ROOK_UNREACHABLE_NODE_TOLERATION_SECONDS", "9s")
	AddUnreachableNodeToleration(&podSpec)

	assert.Equal(t, 1, len(podSpec.Tolerations))
	assert.Equal(t, expectedURToleration, podSpec.Tolerations[0])

}

func testPodSpecPlacement(t *testing.T, requiredDuringScheduling bool, req, pref int, placement *cephv1.Placement) {
	spec := v1.PodSpec{
		InitContainers: []v1.Container{},
		Containers:     []v1.Container{},
		RestartPolicy:  v1.RestartPolicyAlways,
	}

	placement.ApplyToPodSpec(&spec)
	SetNodeAntiAffinityForPod(&spec, requiredDuringScheduling, v1.LabelHostname, map[string]string{"app": "mon"}, nil)

	// should have a required anti-affinity and no preferred anti-affinity
	assert.Equal(t,
		req,
		len(spec.Affinity.PodAntiAffinity.RequiredDuringSchedulingIgnoredDuringExecution))
}

func makePlacement() cephv1.Placement {
	return cephv1.Placement{
		PodAntiAffinity: &v1.PodAntiAffinity{
			RequiredDuringSchedulingIgnoredDuringExecution: []v1.PodAffinityTerm{
				{
					TopologyKey: v1.LabelZoneFailureDomain,
				},
			},
			PreferredDuringSchedulingIgnoredDuringExecution: []v1.WeightedPodAffinityTerm{
				{
					PodAffinityTerm: v1.PodAffinityTerm{
						TopologyKey: v1.LabelZoneFailureDomain,
					},
				},
			},
		},
	}
}

func TestPodSpecPlacement(t *testing.T) {
	// no placement settings in the crd
	p := cephv1.Placement{}
	testPodSpecPlacement(t, true, 1, 0, &p)
	testPodSpecPlacement(t, false, 0, 1, &p)
	testPodSpecPlacement(t, false, 0, 0, &p)

	// crd has other preferred and required anti-affinity setting
	p = makePlacement()
	testPodSpecPlacement(t, true, 2, 1, &p)
	p = makePlacement()
	testPodSpecPlacement(t, false, 1, 2, &p)
}

func TestIsMonScheduled(t *testing.T) {
	ctx := context.TODO()
	clientset := test.New(t, 1)
	pod := v1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "mon-pod",
			Namespace: "ns",
			Labels: map[string]string{
				"app":            "rook-ceph-mon",
				"ceph_daemon_id": "a",
			},
		},
	}

	// no pods running
	isScheduled, err := IsPodScheduled(clientset, "ns", "a")
	assert.Error(t, err)
	assert.False(t, isScheduled)

	selector := fmt.Sprintf("%s=%s,%s=%s", AppAttr, "rook-ceph-mon", "ceph_daemon_id", "a")

	// unscheduled pod
	_, err = clientset.CoreV1().Pods("ns").Create(ctx, &pod, metav1.CreateOptions{})
	assert.NoError(t, err)
	isScheduled, err = IsPodScheduled(clientset, "ns", selector)
	assert.NoError(t, err)
	assert.False(t, isScheduled)

	// scheduled pod
	pod.Spec.NodeName = "node0"
	_, err = clientset.CoreV1().Pods("ns").Update(ctx, &pod, metav1.UpdateOptions{})
	assert.NoError(t, err)
	isScheduled, err = IsPodScheduled(clientset, "ns", selector)
	assert.NoError(t, err)
	assert.True(t, isScheduled)

	// no pods found
	assert.NoError(t, err)
	isScheduled, err = IsPodScheduled(clientset, "ns", "b")
	assert.Error(t, err)
	assert.False(t, isScheduled)
}
