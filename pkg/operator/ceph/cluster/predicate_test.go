/*
Copyright 2020 The Rook Authors. All rights reserved.

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

package cluster

import (
	"testing"
	"time"

	cephv1 "github.com/rook/rook/pkg/apis/ceph.rook.io/v1"
	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestIsHotPlugCM(t *testing.T) {
	blockPool := &cephv1.CephBlockPool{}

	assert.False(t, isHotPlugCM(blockPool))

	cm := &corev1.ConfigMap{}
	assert.False(t, isHotPlugCM(cm))

	cm.Labels = map[string]string{
		"foo": "bar",
	}
	assert.False(t, isHotPlugCM(cm))

	cm.Labels["app"] = "rook-discover"
	assert.True(t, isHotPlugCM(cm))
}

func TestCompareNodes(t *testing.T) {
	tests := []struct {
		name      string
		oldobj    corev1.Node
		newobj    corev1.Node
		reconcile bool
	}{
		{"if no changes", corev1.Node{}, corev1.Node{}, false},
		{"if only Resourceversion got changed", corev1.Node{ObjectMeta: metav1.ObjectMeta{ResourceVersion: "123"}}, corev1.Node{ObjectMeta: metav1.ObjectMeta{ResourceVersion: "145"}}, false},
		{"if only heartbeat got changed", corev1.Node{Status: corev1.NodeStatus{Conditions: []corev1.NodeCondition{{LastHeartbeatTime: metav1.NewTime(time.Now())}}}}, corev1.Node{Status: corev1.NodeStatus{Conditions: []corev1.NodeCondition{{LastHeartbeatTime: metav1.NewTime(<-time.After(5))}}}}, false},
		{"if both Resourceversion and heartbeat change", corev1.Node{ObjectMeta: metav1.ObjectMeta{ResourceVersion: "123"}, Status: corev1.NodeStatus{Conditions: []corev1.NodeCondition{{LastHeartbeatTime: metav1.NewTime(time.Now())}}}}, corev1.Node{ObjectMeta: metav1.ObjectMeta{ResourceVersion: "145"}, Status: corev1.NodeStatus{Conditions: []corev1.NodeCondition{{LastHeartbeatTime: metav1.NewTime(<-time.After(5))}}}}, false},
		{"if only a field changes", corev1.Node{Spec: corev1.NodeSpec{PodCIDR: "123"}}, corev1.Node{Spec: corev1.NodeSpec{PodCIDR: "145"}}, true},
		{"if a field and the Resourceversion change", corev1.Node{Spec: corev1.NodeSpec{PodCIDR: "123"}, ObjectMeta: metav1.ObjectMeta{ResourceVersion: "123"}}, corev1.Node{Spec: corev1.NodeSpec{PodCIDR: "145"}, ObjectMeta: metav1.ObjectMeta{ResourceVersion: "145"}}, true},
		{"if a field and the hertbeat change", corev1.Node{Spec: corev1.NodeSpec{PodCIDR: "123"}, Status: corev1.NodeStatus{Conditions: []corev1.NodeCondition{{LastHeartbeatTime: metav1.NewTime(time.Now())}}}}, corev1.Node{Spec: corev1.NodeSpec{PodCIDR: "145"}, Status: corev1.NodeStatus{Conditions: []corev1.NodeCondition{{LastHeartbeatTime: metav1.NewTime(<-time.After(5))}}}}, true},
		{"if a field, Resourceversion and the hertbeat change", corev1.Node{ObjectMeta: metav1.ObjectMeta{ResourceVersion: "123"}, Spec: corev1.NodeSpec{PodCIDR: "123"}, Status: corev1.NodeStatus{Conditions: []corev1.NodeCondition{{LastHeartbeatTime: metav1.NewTime(time.Now())}}}}, corev1.Node{ObjectMeta: metav1.ObjectMeta{ResourceVersion: "145"}, Spec: corev1.NodeSpec{PodCIDR: "145"}, Status: corev1.NodeStatus{Conditions: []corev1.NodeCondition{{LastHeartbeatTime: metav1.NewTime(<-time.After(5))}}}}, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			//#nosec G601 -- since nothing is modifying the tests slic
			assert.Equal(t, tt.reconcile, shouldReconcileChangedNode(&tt.oldobj, &tt.newobj))
		})
	}
}
