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
	"context"
	"testing"
	"time"

	"github.com/rook/rook/pkg/clusterd"
	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/event"
)

func TestIsHotPlugCM(t *testing.T) {
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

func TestNodeTopologyLabelsChanged(t *testing.T) {
	tests := []struct {
		name    string
		oldobj  corev1.Node
		newobj  corev1.Node
		changed bool
	}{
		{"if no labels", corev1.Node{}, corev1.Node{}, false},
		{
			"if a topology label is added",
			corev1.Node{ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{"kubernetes.io/hostname": "node1"}}},
			corev1.Node{ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{"kubernetes.io/hostname": "node1", "topology.rook.io/rack": "rack1"}}},
			true,
		},
		{
			"if a topology label value is changed",
			corev1.Node{ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{"topology.rook.io/rack": "rack1"}}},
			corev1.Node{ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{"topology.rook.io/rack": "rack2"}}},
			true,
		},
		{
			"if a topology label is removed",
			corev1.Node{ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{"kubernetes.io/hostname": "node1", "topology.rook.io/rack": "rack1"}}},
			corev1.Node{ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{"kubernetes.io/hostname": "node1"}}},
			true,
		},
		{
			"if a kubernetes topology label is changed",
			corev1.Node{ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{"topology.kubernetes.io/zone": "zone1"}}},
			corev1.Node{ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{"topology.kubernetes.io/zone": "zone2"}}},
			true,
		},
		{
			"if a non-topology label is changed",
			corev1.Node{ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{"topology.rook.io/rack": "rack1", "example.com/foo": "bar"}}},
			corev1.Node{ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{"topology.rook.io/rack": "rack1", "example.com/foo": "baz"}}},
			false,
		},
		{
			"if an annotation is changed",
			corev1.Node{ObjectMeta: metav1.ObjectMeta{Annotations: map[string]string{"foo": "bar"}}},
			corev1.Node{ObjectMeta: metav1.ObjectMeta{Annotations: map[string]string{"foo": "baz"}}},
			false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			//#nosec G601 -- since nothing is modifying the tests slice
			assert.Equal(t, tt.changed, nodeTopologyLabelsChanged(&tt.oldobj, &tt.newobj))
		})
	}
}

func TestPredicateForNodeWatcherUpdate(t *testing.T) {
	ns := "rook-ceph"
	opns := "operator"
	nodeName := "node-already-osd-host"

	// Simulate a node that onK8sNode() would short-circuit to false for (for
	// example because it is already an OSD host), so the only way a label change
	// triggers a reconcile is the topology label check in UpdateFunc.
	nodesCheckedForReconcile.Insert(nodeName)
	defer nodesCheckedForReconcile.Delete(nodeName)

	client := getFakeClient(fakeCluster(ns))
	p := predicateForNodeWatcher[*corev1.Node](context.TODO(), client, &clusterd.Context{}, opns)

	baseNode := func() *corev1.Node {
		return &corev1.Node{
			ObjectMeta: metav1.ObjectMeta{
				Name:   nodeName,
				Labels: map[string]string{"kubernetes.io/hostname": nodeName},
			},
		}
	}

	t.Run("topology label change triggers reconcile even when onK8sNode returns false", func(t *testing.T) {
		objOld := baseNode()
		objNew := baseNode()
		objNew.Labels["topology.rook.io/rack"] = "rack1"

		reconcile := p.Update(event.TypedUpdateEvent[*corev1.Node]{ObjectOld: objOld, ObjectNew: objNew})
		assert.True(t, reconcile)
	})

	t.Run("non-topology label change does not trigger reconcile", func(t *testing.T) {
		objOld := baseNode()
		objNew := baseNode()
		objNew.Labels["example.com/foo"] = "bar"

		reconcile := p.Update(event.TypedUpdateEvent[*corev1.Node]{ObjectOld: objOld, ObjectNew: objNew})
		assert.False(t, reconcile)
	})

	t.Run("annotation change does not trigger reconcile", func(t *testing.T) {
		objOld := baseNode()
		objNew := baseNode()
		objNew.Annotations = map[string]string{"foo": "bar"}

		reconcile := p.Update(event.TypedUpdateEvent[*corev1.Node]{ObjectOld: objOld, ObjectNew: objNew})
		assert.False(t, reconcile)
	})

	t.Run("resourceVersion-only change does not trigger reconcile", func(t *testing.T) {
		objOld := baseNode()
		objNew := baseNode()
		objOld.ResourceVersion = "1"
		objNew.ResourceVersion = "2"

		reconcile := p.Update(event.TypedUpdateEvent[*corev1.Node]{ObjectOld: objOld, ObjectNew: objNew})
		assert.False(t, reconcile)
	})

	t.Run("non-label spec change defers to onK8sNode", func(t *testing.T) {
		objOld := baseNode()
		objNew := baseNode()
		objNew.Spec.PodCIDR = "10.0.0.0/24"

		// onK8sNode short-circuits to false because the node is already in
		// nodesCheckedForReconcile, so a pure spec change is not forced to true.
		reconcile := p.Update(event.TypedUpdateEvent[*corev1.Node]{ObjectOld: objOld, ObjectNew: objNew})
		assert.False(t, reconcile)
	})
}
