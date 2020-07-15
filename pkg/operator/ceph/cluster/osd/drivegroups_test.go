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

package osd

import (
	"fmt"
	"testing"

	cephv1 "github.com/rook/rook/pkg/apis/ceph.rook.io/v1"
	rookv1 "github.com/rook/rook/pkg/apis/rook.io/v1"
	"github.com/stretchr/testify/assert"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestSanitizeDriveGroups(t *testing.T) {
	// this should look the same in input and output
	dgSanitized := cephv1.DriveGroup{
		Name: "sanitized",
		Spec: cephv1.DriveGroupSpec{
			"placement": map[string]interface{}{
				"host_pattern": "*",
			},
			"data_devices": map[string]interface{}{
				"all": "true",
			},
		},
		Placement: rookv1.Placement{},
	}
	// this should not make it to output
	dgEmptySpec := cephv1.DriveGroup{
		Name:      "empty-spec",
		Spec:      cephv1.DriveGroupSpec{},
		Placement: rookv1.Placement{},
	}
	// when sanitized, this should look the same as dgSanitized except the name
	dgHostPattern := cephv1.DriveGroup{
		Name: "host-pattern",
		Spec: cephv1.DriveGroupSpec{
			"host_pattern": "*",
			"data_devices": map[string]interface{}{
				"all": "true",
			},
		},
		Placement: rookv1.Placement{},
	}
	// when sanitized this should look the same as dgSanitized except the name
	dgPlacement := cephv1.DriveGroup{
		Name: "placement-host-pattern",
		Spec: cephv1.DriveGroupSpec{
			"placement": map[string]interface{}{
				"nodes": []interface{}{"node1", "node2"},
			},
			"data_devices": map[string]interface{}{
				"all": "true",
			},
		},
		Placement: rookv1.Placement{},
	}
	// when sanitized, the service_id should be replaced by 'different-service-id'
	dgServiceID := cephv1.DriveGroup{
		Name: "different-service-id",
		Spec: cephv1.DriveGroupSpec{
			"data_devices": map[string]interface{}{
				"all": "true",
			},
			"service_id": "custom_id",
		},
		Placement: rookv1.Placement{},
	}

	// assert that the drive groups are "functionally" equal and that the actual has the expected name
	// The expected drive group's name information will not be used to inform "functional" equality
	assertDGsFunctionallyEqual := func(expectedName string, expected, actual cephv1.DriveGroup) {
		t.Helper()
		newExpected := *expected.DeepCopy()
		newExpected.Name = expectedName
		newExpected.Spec["service_id"] = expectedName
		// also assert that the actual service_id given is the expected name
		assert.Equal(t, expectedName, actual.Spec["service_id"])
		assert.Equal(t, newExpected, actual)
	}

	// The first drive group already has placement.host_pattern set to the sanitized value, so both
	// input and output should be the same
	dgs := cephv1.DriveGroupsSpec{dgSanitized}
	out := SanitizeDriveGroups(dgs)
	fmt.Println(out)
	assert.Len(t, out, 1)
	assertDGsFunctionallyEqual(dgSanitized.Name, dgSanitized, out[0])

	// An empty spec should not make it into the output
	dgs = cephv1.DriveGroupsSpec{dgEmptySpec}
	out = SanitizeDriveGroups(dgs)
	assert.Empty(t, out)

	// Groups with deprecated top-level host_pattern or top-level placement defined should have
	// those replaced with a glob-anything host pattern
	dgs = cephv1.DriveGroupsSpec{dgHostPattern, dgPlacement, dgEmptySpec}
	out = SanitizeDriveGroups(dgs)
	assert.Len(t, out, 2)
	assertDGsFunctionallyEqual(dgHostPattern.Name, dgSanitized, out[0])
	assertDGsFunctionallyEqual(dgPlacement.Name, dgSanitized, out[1])

	// A group with a preexisting service_id should have it replaced
	dgs = cephv1.DriveGroupsSpec{dgServiceID}
	out = SanitizeDriveGroups(dgs)
	assert.Len(t, out, 1)
	assert.Equal(t, dgServiceID.Name, out[0].Spec["service_id"])
	assertDGsFunctionallyEqual(dgServiceID.Name, dgSanitized, out[0])
}

func TestDriveGroupsWithPlacementMatchingNode(t *testing.T) {
	//
	// Make a bunch of drive groups to test
	//

	// dg1 is a basic drive group with no placement
	dg1 := cephv1.DriveGroup{
		Name: "dg1",
		Spec: cephv1.DriveGroupSpec{
			"data_devices": map[string]interface{}{
				"all": "true",
			},
		},
		Placement: rookv1.Placement{},
	}

	// dg2 has affinity for label role=storage
	dg2 := *dg1.DeepCopy()
	dg2.Name = "dg2"
	dg2.Placement.NodeAffinity = &v1.NodeAffinity{
		RequiredDuringSchedulingIgnoredDuringExecution: &v1.NodeSelector{
			NodeSelectorTerms: []v1.NodeSelectorTerm{
				{
					MatchExpressions: []v1.NodeSelectorRequirement{
						{Key: "role", Operator: v1.NodeSelectorOpIn, Values: []string{"storage"}},
					},
				},
			},
		},
	}

	// dg3 tolerates taint role=storage
	dg3 := *dg1.DeepCopy()
	dg3.Name = "dg3"
	dg3.Placement.Tolerations = []v1.Toleration{
		{Key: "role", Operator: v1.TolerationOpEqual, Value: "storage"},
	}

	// dg4 has affinity for label role=storage and tolerates taint role=storage
	dg4 := *dg2.DeepCopy()
	dg4.Name = "dg4"
	dg4.Placement.Tolerations = []v1.Toleration{
		{Key: "role", Operator: v1.TolerationOpEqual, Value: "storage"},
	}

	dgs := cephv1.DriveGroupsSpec{dg1, dg2, dg3, dg4}

	assertGroupsApplyToNode := func(n v1.Node, groups ...string) {
		t.Helper()
		gs, err := DriveGroupsWithPlacementMatchingNode(dgs, &n)
		assert.NoError(t, err)
		actualGroups := []string{}
		for _, g := range gs {
			actualGroups = append(actualGroups, g.Name)
		}
		assert.ElementsMatch(t, groups, actualGroups)
	}

	//
	// Make a bunch of nodes with different stuff going on
	//

	// node1 is a basic node that is ready without labels or taints
	node1 := v1.Node{
		ObjectMeta: metav1.ObjectMeta{
			Name:   "node1",
			Labels: map[string]string{},
		},
		Spec: v1.NodeSpec{},
		Status: v1.NodeStatus{
			Conditions: []v1.NodeCondition{
				{Type: v1.NodeReady, Status: v1.ConditionTrue},
			},
		},
	}
	assertGroupsApplyToNode(node1, "dg1", "dg3")

	// node2 is not ready; no nodes should apply
	node2 := *node1.DeepCopy()
	node2.ObjectMeta.Name = "node2"
	node2.Status.Conditions[0].Status = v1.ConditionFalse
	assertGroupsApplyToNode(node2)

	// node3 has a label role=storage
	node3 := *node1.DeepCopy()
	node3.ObjectMeta.Name = "node3"
	node3.ObjectMeta.Labels["role"] = "storage"
	assertGroupsApplyToNode(node3, "dg1", "dg2", "dg3", "dg4")

	// node4 has taint role=storage:NoExecute but no label role=storage
	node4 := *node1.DeepCopy()
	node4.ObjectMeta.Name = "node4"
	node4.Spec.Taints = []v1.Taint{
		{Key: "role", Value: "storage", Effect: v1.TaintEffectNoExecute},
	}
	assertGroupsApplyToNode(node4, "dg3")

	// node5 has both taint and label from node3 and node4
	node5 := *node4.DeepCopy()
	node5.ObjectMeta.Name = "node5"
	node5.ObjectMeta.Labels["role"] = "storage"
	assertGroupsApplyToNode(node5, "dg3", "dg4")
}
