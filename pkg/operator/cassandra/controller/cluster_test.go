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

package controller

import (
	"fmt"
	"testing"

	cassandrav1alpha1 "github.com/rook/rook/pkg/apis/cassandra.rook.io/v1alpha1"
	"github.com/rook/rook/pkg/operator/cassandra/constants"
	"github.com/rook/rook/pkg/operator/cassandra/controller/util"
	casstest "github.com/rook/rook/pkg/operator/cassandra/test"
	"github.com/stretchr/testify/require"
	appsv1 "k8s.io/api/apps/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
)

func TestCreateRack(t *testing.T) {
	simpleCluster := casstest.NewSimpleCluster(3)

	tests := []struct {
		name        string
		kubeObjects []runtime.Object
		rack        cassandrav1alpha1.RackSpec
		cluster     *cassandrav1alpha1.Cluster
		expectedErr bool
	}{
		{
			name:        "new rack",
			kubeObjects: nil,
			rack:        simpleCluster.Spec.Datacenter.Racks[0],
			cluster:     simpleCluster,
			expectedErr: false,
		},
		{
			name: "sts already exists",
			kubeObjects: []runtime.Object{
				util.StatefulSetForRack(simpleCluster.Spec.Datacenter.Racks[0], simpleCluster, ""),
			},
			rack:        simpleCluster.Spec.Datacenter.Racks[0],
			cluster:     simpleCluster,
			expectedErr: false,
		},
		{
			name: "sts exists with different owner",
			kubeObjects: []runtime.Object{
				&appsv1.StatefulSet{
					ObjectMeta: metav1.ObjectMeta{
						Name:            util.StatefulSetNameForRack(simpleCluster.Spec.Datacenter.Racks[0], simpleCluster),
						Namespace:       simpleCluster.Namespace,
						OwnerReferences: nil,
					},
					Spec: appsv1.StatefulSetSpec{},
				},
			},
			rack:        simpleCluster.Spec.Datacenter.Racks[0],
			cluster:     simpleCluster,
			expectedErr: true,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			cc := newFakeClusterController(test.kubeObjects, nil)

			if err := cc.createRack(test.rack, test.cluster); err == nil {
				if test.expectedErr {
					t.Errorf("Expected an error, got none.")
				} else {

					var sts *appsv1.StatefulSet
					sts, err = cc.kubeClient.AppsV1().StatefulSets(test.cluster.Namespace).
						Get(util.StatefulSetNameForRack(test.rack, test.cluster), metav1.GetOptions{})
					if err != nil {
						t.Errorf("Couldn't retrieve expected StatefulSet: %s", err.Error())
					} else {
						t.Logf("Got StatefulSet as expected: %s", sts.Name)
					}
				}
			} else {
				if test.expectedErr {
					t.Logf("Got an error as expected: %s", err.Error())
				} else {
					t.Errorf("Unexpected error: %s", err.Error())
				}
			}
		})
	}
}

func TestScaleUpRack(t *testing.T) {

	currMembers := int32(2)
	expMembers := int32(3)
	c := casstest.NewSimpleCluster(expMembers)
	r := c.Spec.Datacenter.Racks[0]
	sts := util.StatefulSetForRack(r, c, "")
	*sts.Spec.Replicas = currMembers

	tests := []struct {
		name        string
		kubeObjects []runtime.Object
		rack        cassandrav1alpha1.RackSpec
		rackStatus  *cassandrav1alpha1.RackStatus
		cluster     *cassandrav1alpha1.Cluster
		expectedErr bool
	}{
		{
			name:        "normal",
			kubeObjects: []runtime.Object{sts},
			rack:        r,
			rackStatus:  &cassandrav1alpha1.RackStatus{Members: currMembers, ReadyMembers: currMembers},
			cluster:     c,
			expectedErr: false,
		},
		{
			name:        "statefulset missing",
			kubeObjects: []runtime.Object{},
			rack:        r,
			rackStatus:  &cassandrav1alpha1.RackStatus{Members: currMembers, ReadyMembers: currMembers},
			cluster:     c,
			expectedErr: true,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {

			cc := newFakeClusterController(test.kubeObjects, nil)

			test.cluster.Status = cassandrav1alpha1.ClusterStatus{
				Racks: map[string]*cassandrav1alpha1.RackStatus{
					"test-rack": test.rackStatus,
				},
			}
			err := cc.scaleUpRack(test.rack, test.cluster)

			if err == nil {
				if test.expectedErr {
					t.Errorf("Expected an error, got none.")
				} else {
					sts, err := cc.kubeClient.AppsV1().StatefulSets(test.cluster.Namespace).
						Get(util.StatefulSetNameForRack(test.rack, test.cluster), metav1.GetOptions{})
					if err != nil {
						t.Errorf("Couldn't retrieve expected StatefulSet: %s", err.Error())
						return
					}
					expectedReplicas := test.rackStatus.Members + 1
					actualReplicas := *sts.Spec.Replicas
					if actualReplicas != expectedReplicas {
						t.Errorf("Error, expected %d replicas, got %d.", expectedReplicas, actualReplicas)
						return
					}
					t.Logf("Rack scaled to %d members as expected", actualReplicas)
				}
			} else {
				if test.expectedErr {
					t.Logf("Got an error as expected: %s", err.Error())
				}
			}

		})
	}
}

func TestScaleDownRack(t *testing.T) {

	desired := int32(2)
	actual := int32(3)

	c := casstest.NewSimpleCluster(desired)
	r := c.Spec.Datacenter.Racks[0]
	c.Status = cassandrav1alpha1.ClusterStatus{
		Racks: map[string]*cassandrav1alpha1.RackStatus{
			r.Name: {
				Members:      actual,
				ReadyMembers: actual,
			},
		},
	}
	sts := util.StatefulSetForRack(r, c, "")
	memberServices := casstest.MemberServicesForCluster(c)

	// Find the member to decommission
	memberName := fmt.Sprintf("%s-%d", util.StatefulSetNameForRack(r, c), actual-1)

	t.Run("scale down requested and started", func(t *testing.T) {

		kubeObjects := append(memberServices, sts)
		rookObjects := []runtime.Object{c}
		cc := newFakeClusterController(kubeObjects, rookObjects)

		err := cc.scaleDownRack(r, c)
		require.NoErrorf(t, err, "Unexpected error while scaling down: %v", err)

		// Check that MemberService has the decommissioned label
		svc, err := cc.serviceLister.Services(c.Namespace).Get(memberName)
		require.NoErrorf(t, err, "Unexpected error while getting MemberService: %v", err)

		val, ok := svc.Labels[constants.DecommissionLabel]
		require.True(t, ok, "Service didn't have the decommissioned label as expected")
		require.Truef(t, val == constants.LabelValueFalse, "Decommissioned Label had unexpected value: %s", val)

	})

	t.Run("scale down resumed", func(t *testing.T) {

		sts.Spec.Replicas = &actual

		kubeObjects := append(memberServices, sts)
		rookObjects := []runtime.Object{c}

		cc := newFakeClusterController(kubeObjects, rookObjects)

		svc, err := cc.serviceLister.Services(c.Namespace).Get(memberName)
		require.NoErrorf(t, err, "Unexpected error while getting MemberService: %v", err)

		// Mark as decommissioned
		svc.Labels[constants.DecommissionLabel] = constants.LabelValueTrue
		_, err = cc.kubeClient.CoreV1().Services(svc.Namespace).Update(svc)
		require.Nilf(t, err, "Unexpected error while updating MemberService: %v", err)

		// Resume decommission
		err = cc.scaleDownRack(r, c)
		require.NoErrorf(t, err, "Unexpected error while resuming scale down: %v", err)

		// Check that StatefulSet is scaled
		updatedSts, err := cc.kubeClient.AppsV1().StatefulSets(sts.Namespace).Get(sts.Name, metav1.GetOptions{})
		require.NoErrorf(t, err, "Unexpected error while getting statefulset: %v", err)
		require.Truef(t, *updatedSts.Spec.Replicas == *sts.Spec.Replicas-1, "Statefulset has incorrect number of replicas. Expected: %d, got %d.", *sts.Spec.Replicas-1, *updatedSts.Spec.Replicas)

	})

}
