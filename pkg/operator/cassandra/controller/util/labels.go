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
	cassandrav1alpha1 "github.com/rook/rook/pkg/apis/cassandra.rook.io/v1alpha1"
	"github.com/rook/rook/pkg/operator/cassandra/constants"
	appsv1 "k8s.io/api/apps/v1"
	"k8s.io/apimachinery/pkg/labels"
)

// ClusterLabels returns a map of label keys and values
// for the given Cluster.
func ClusterLabels(c *cassandrav1alpha1.Cluster) map[string]string {
	labels := recommendedLabels()
	labels[constants.ClusterNameLabel] = c.Name
	return labels
}

// DatacenterLabels returns a map of label keys and values
// for the given Datacenter.
func DatacenterLabels(c *cassandrav1alpha1.Cluster) map[string]string {
	recLabels := recommendedLabels()
	dcLabels := ClusterLabels(c)
	dcLabels[constants.DatacenterNameLabel] = c.Spec.Datacenter.Name

	return mergeLabels(dcLabels, recLabels)
}

// RackLabels returns a map of label keys and values
// for the given Rack.
func RackLabels(r cassandrav1alpha1.RackSpec, c *cassandrav1alpha1.Cluster) map[string]string {
	recLabels := recommendedLabels()
	rackLabels := DatacenterLabels(c)
	rackLabels[constants.RackNameLabel] = r.Name

	return mergeLabels(rackLabels, recLabels)
}

// StatefulSetPodLabel returns a map of labels to uniquely
// identify a StatefulSet Pod with the given name
func StatefulSetPodLabel(name string) map[string]string {
	return map[string]string{
		appsv1.StatefulSetPodNameLabel: name,
	}
}

// RackSelector returns a LabelSelector for the given rack.
func RackSelector(r cassandrav1alpha1.RackSpec, c *cassandrav1alpha1.Cluster) labels.Selector {

	rackLabelsSet := labels.Set(RackLabels(r, c))
	sel := labels.SelectorFromSet(rackLabelsSet)

	return sel
}

func recommendedLabels() map[string]string {

	return map[string]string{
		"app": constants.AppName,

		"app.kubernetes.io/name":       constants.AppName,
		"app.kubernetes.io/managed-by": constants.OperatorAppName,
	}
}

func mergeLabels(l1, l2 map[string]string) map[string]string {

	res := make(map[string]string)
	for k, v := range l1 {
		res[k] = v
	}
	for k, v := range l2 {
		res[k] = v
	}
	return res
}
