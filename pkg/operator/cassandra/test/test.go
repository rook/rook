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

package test

import (
	cassandrav1alpha1 "github.com/rook/rook/pkg/apis/cassandra.rook.io/v1alpha1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func NewSimpleCluster(members int32) *cassandrav1alpha1.Cluster {
	return &cassandrav1alpha1.Cluster{
		TypeMeta: metav1.TypeMeta{
			APIVersion: cassandrav1alpha1.APIVersion,
			Kind:       "Cluster",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-cluster",
			Namespace: "test-ns",
		},
		Spec: cassandrav1alpha1.ClusterSpec{
			Version: "3.1.11",
			Mode:    cassandrav1alpha1.ClusterModeCassandra,
			Datacenter: cassandrav1alpha1.DatacenterSpec{
				Name: "test-dc",
				Racks: []cassandrav1alpha1.RackSpec{
					{
						Name:    "test-rack",
						Members: members,
					},
				},
			},
		},
	}
}
