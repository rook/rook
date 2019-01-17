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

package util

import (
	"fmt"
	"github.com/rook/rook/pkg/apis/cassandra.rook.io"
	cassandrav1alpha1 "github.com/rook/rook/pkg/apis/cassandra.rook.io/v1alpha1"
	"github.com/rook/rook/pkg/operator/cassandra/constants"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/selection"
	"k8s.io/client-go/kubernetes"
	appslisters "k8s.io/client-go/listers/apps/v1"
	corelisters "k8s.io/client-go/listers/core/v1"
	"strconv"
	"strings"
)

// GetPodsForCluster returns the existing Pods for
// the given cluster
func GetPodsForCluster(cluster *cassandrav1alpha1.Cluster, podLister corelisters.PodLister) ([]*corev1.Pod, error) {

	clusterRequirement, err := labels.NewRequirement(constants.ClusterNameLabel, selection.Equals, []string{cluster.Name})
	if err != nil {
		return nil, fmt.Errorf("error trying to create clusterRequirement: %s", err.Error())
	}
	clusterSelector := labels.NewSelector().Add(*clusterRequirement)
	return podLister.Pods(cluster.Namespace).List(clusterSelector)

}

// GetMemberServicesForRack returns the member services for the given rack.
func GerMemberServicesForRack(
	r cassandrav1alpha1.RackSpec,
	c *cassandrav1alpha1.Cluster,
	serviceLister corelisters.ServiceLister,
) ([]*corev1.Service, error) {

	sel := RackSelector(r, c)
	return serviceLister.Services(c.Namespace).List(sel)
}

// GetPodsForRack returns the created Pods for the given rack.
func GetPodsForRack(
	r cassandrav1alpha1.RackSpec,
	c *cassandrav1alpha1.Cluster,
	podLister corelisters.PodLister,
) ([]*corev1.Pod, error) {

	sel := RackSelector(r, c)
	return podLister.Pods(c.Namespace).List(sel)

}

// VerifyOwner checks if the owner Object is the controller
// of the obj Object and returns an error if it isn't.
func VerifyOwner(obj, owner metav1.Object) error {
	if !metav1.IsControlledBy(obj, owner) {
		ownerRef := metav1.GetControllerOf(obj)
		return fmt.Errorf(
			"'%s/%s' is foreign owned: "+
				"it is owned by '%v', not '%s/%s'.",
			obj.GetNamespace(), obj.GetName(),
			ownerRef,
			owner.GetNamespace(), owner.GetName(),
		)
	}
	return nil
}

// NewControllerRef returns an OwnerReference to
// the provided Cluster Object
func NewControllerRef(c *cassandrav1alpha1.Cluster) metav1.OwnerReference {
	return *metav1.NewControllerRef(c, schema.GroupVersionKind{
		Group:   cassandrarookio.GroupName,
		Version: "v1alpha1",
		Kind:    "Cluster",
	})
}

// RefFromString is a helper function that takes a string
// and outputs a reference to that string.
// Useful for initializing a string pointer from a literal.
func RefFromString(s string) *string {
	return &s
}

// RefFromInt is a helper function that takes a int
// and outputs a reference to that int.
// Useful for initializing an int pointer from a literal.
func RefFromInt(i int32) *int32 {
	return &i
}

// IndexFromName attempts to get the index from a name using the
// naming convention <name>-<index>.
func IndexFromName(n string) (int32, error) {

	// index := svc.Name[strings.LastIndex(svc.Name, "-") + 1 : len(svc.Name)]
	delimIndex := strings.LastIndex(n, "-")
	if delimIndex == -1 {
		return -1, fmt.Errorf("couldn't get index from name %s", n)
	}

	index, err := strconv.Atoi(n[delimIndex+1:])
	if err != nil {
		return -1, fmt.Errorf("couldn't get index from name %s", n)
	}

	return int32(index), nil
}

// isPodUnschedulable iterates a Pod's Status.Conditions to find out
// if it has been deemed unschedulable
func IsPodUnschedulable(pod *corev1.Pod) bool {
	for _, v := range pod.Status.Conditions {
		if v.Reason == corev1.PodReasonUnschedulable {
			return true
		}
	}
	return false
}

// RefFromInt32 is a helper function that takes a int32
// and outputs a reference to that int.
func RefFromInt32(i int32) *int32 {
	return &i
}

// RefFromInt64 is a helper function that takes a int64
// and outputs a reference to that int.
func RefFromInt64(i int64) *int64 {
	return &i
}

// Max returns the bigger of two given numbers
func Max(x, y int64) int64 {
	if x < y {
		return y
	}
	return x
}

// Min returns the smaller of two given numbers
func Min(x, y int64) int64 {
	if x < y {
		return x
	}
	return y
}

// ScaleStatefulSet attempts to scale a StatefulSet by the given amount
func ScaleStatefulSet(sts *appsv1.StatefulSet, amount int32, kubeClient kubernetes.Interface) error {
	updatedSts := sts.DeepCopy()
	updatedReplicas := *updatedSts.Spec.Replicas + amount
	if updatedReplicas < 0 {
		return fmt.Errorf("error, can't scale statefulset below 0 replicas")
	}
	updatedSts.Spec.Replicas = &updatedReplicas
	err := PatchStatefulSet(sts, updatedSts, kubeClient)
	return err
}

// IsRackConditionTrue checks a rack's status for the presence of a condition type
// and checks if it is true.
func IsRackConditionTrue(rackStatus *cassandrav1alpha1.RackStatus, condType cassandrav1alpha1.RackConditionType) bool {
	for _, cond := range rackStatus.Conditions {
		if cond.Type == cassandrav1alpha1.RackConditionTypeMemberLeaving && cond.Status == cassandrav1alpha1.ConditionTrue {
			return true
		}
	}
	return false
}

// StatefulSetStatusesStale checks if the StatefulSet Objects of a Cluster
// have been observed by the StatefulSet controller.
// If they haven't, their status might be stale, so it's better to wait
// and process them later.
func StatefulSetStatusesStale(c *cassandrav1alpha1.Cluster, statefulSetLister appslisters.StatefulSetLister) (bool, error) {
	// Before proceeding, ensure all the Statefulset Statuses are valid
	for _, r := range c.Spec.Datacenter.Racks {
		if _, ok := c.Status.Racks[r.Name]; !ok {
			continue
		}
		sts, err := statefulSetLister.StatefulSets(c.Namespace).Get(StatefulSetNameForRack(r, c))
		if err != nil {
			return true, fmt.Errorf("error getting statefulset: %s", err.Error())
		}
		if sts.Generation != sts.Status.ObservedGeneration {
			return true, nil
		}
	}
	return false, nil
}
