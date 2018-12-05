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
	cassandrav1alpha1 "github.com/rook/rook/pkg/apis/cassandra.rook.io/v1alpha1"
	"github.com/rook/rook/pkg/operator/cassandra/controller/util"
	"github.com/rook/rook/pkg/operator/cassandra/test"
	appsv1 "k8s.io/api/apps/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"testing"
)

func TestCreateRack(t *testing.T) {

	simpleCluster := test.NewSimpleCluster(3)

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
			err := cc.createRack(test.rack, test.cluster)

			if err == nil {
				if test.expectedErr {
					t.Errorf("Expected an error, got none.")
				} else {

					sts, err := cc.kubeClient.AppsV1().StatefulSets(test.cluster.Namespace).
						Get(util.StatefulSetNameForRack(test.rack, test.cluster), metav1.GetOptions{})
					if err != nil {
						t.Errorf("Couldn't retrieve expected StatefulSet: %s", err.Error())
					} else {
						t.Logf("Got StatefulSet as expected: %s", sts.Name)
					}
				}
			}
			if err != nil {
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
	c := test.NewSimpleCluster(expMembers)
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
