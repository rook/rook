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
	"encoding/json"
	cassandrav1alpha1 "github.com/rook/rook/pkg/apis/cassandra.rook.io/v1alpha1"
	"github.com/rook/rook/pkg/client/clientset/versioned"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/strategicpatch"
	"k8s.io/client-go/kubernetes"
)

// PatchService patches the old Service so that it matches the
// new Service.
func PatchService(old, new *corev1.Service, kubeClient kubernetes.Interface) error {

	oldJSON, err := json.Marshal(old)
	if err != nil {
		return err
	}

	newJSON, err := json.Marshal(new)
	if err != nil {
		return err
	}

	patchBytes, err := strategicpatch.CreateTwoWayMergePatch(oldJSON, newJSON, corev1.Service{})
	if err != nil {
		return err
	}

	_, err = kubeClient.CoreV1().Services(old.Namespace).Patch(old.Name, types.StrategicMergePatchType, patchBytes)
	return err
}

// PatchStatefulSet patches the old StatefulSet so that it matches the
// new StatefulSet.
func PatchStatefulSet(old, new *appsv1.StatefulSet, kubeClient kubernetes.Interface) error {

	oldJSON, err := json.Marshal(old)
	if err != nil {
		return err
	}

	newJSON, err := json.Marshal(new)
	if err != nil {
		return err
	}

	patchBytes, err := strategicpatch.CreateTwoWayMergePatch(oldJSON, newJSON, appsv1.StatefulSet{})
	if err != nil {
		return err
	}

	_, err = kubeClient.AppsV1().StatefulSets(old.Namespace).Patch(old.Name, types.StrategicMergePatchType, patchBytes)
	return err
}

// PatchCluster patches the old Cluster so that it matches the new Cluster.
func PatchClusterStatus(c *cassandrav1alpha1.Cluster, rookClient versioned.Interface) error {

	// JSON Patch RFC 6902
	patch := []struct {
		Op    string                          `json:"op"`
		Path  string                          `json:"path"`
		Value cassandrav1alpha1.ClusterStatus `json:"value"`
	}{
		{
			Op:    "add",
			Path:  "/status",
			Value: c.Status,
		},
	}

	patchBytes, err := json.Marshal(patch)
	if err != nil {
		return err
	}
	_, err = rookClient.CassandraV1alpha1().Clusters(c.Namespace).Patch(c.Name, types.JSONPatchType, patchBytes)
	return err

}
