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
package v1alpha2

import (
	"encoding/json"
	"testing"

	"github.com/ghodss/yaml"
	"github.com/stretchr/testify/assert"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestPlacement_spec(t *testing.T) {
	specYaml := []byte(`
nodeAffinity:
  requiredDuringSchedulingIgnoredDuringExecution:
    nodeSelectorTerms:
    - matchExpressions:
      - key: foo
        operator: In
        values:
          - bar
tolerations:
  - key: foo
    operator: Exists`)

	// convert the raw spec yaml into JSON
	rawJSON, err := yaml.YAMLToJSON(specYaml)
	assert.Nil(t, err)

	// unmarshal the JSON into a strongly typed placement spec object
	var placement Placement
	err = json.Unmarshal(rawJSON, &placement)
	assert.Nil(t, err)

	// the unmarshalled placement spec should equal the expected spec below
	expected := Placement{
		NodeAffinity: &v1.NodeAffinity{
			RequiredDuringSchedulingIgnoredDuringExecution: &v1.NodeSelector{
				NodeSelectorTerms: []v1.NodeSelectorTerm{
					{
						MatchExpressions: []v1.NodeSelectorRequirement{
							{
								Key:      "foo",
								Operator: v1.NodeSelectorOpIn,
								Values:   []string{"bar"},
							},
						},
					},
				},
			},
		},
		Tolerations: []v1.Toleration{
			{
				Key:      "foo",
				Operator: v1.TolerationOpExists,
			},
		},
	}
	assert.Equal(t, expected, placement)
}

func TestPlacementApplyToPodSpec(t *testing.T) {
	to := placementTestGetTolerations("foo", "bar")
	na := placementTestGenerateNodeAffinity()
	antiaffinity := placementAntiAffinity("v1")
	expected := &v1.PodSpec{Affinity: &v1.Affinity{NodeAffinity: na, PodAntiAffinity: antiaffinity}, Tolerations: to}

	var p Placement
	var ps *v1.PodSpec

	p = Placement{NodeAffinity: na, Tolerations: to, PodAntiAffinity: antiaffinity}
	ps = &v1.PodSpec{}
	p.ApplyToPodSpec(ps)
	assert.Equal(t, expected, ps)
	assert.Equal(t, 1, len(ps.Affinity.PodAntiAffinity.PreferredDuringSchedulingIgnoredDuringExecution))

	// Appending some other antiaffinity to the pod spec should not alter the original placement antiaffinity
	otherAntiAffinity := placementAntiAffinity("v2")
	ps.Affinity.PodAntiAffinity.PreferredDuringSchedulingIgnoredDuringExecution = append(
		ps.Affinity.PodAntiAffinity.PreferredDuringSchedulingIgnoredDuringExecution,
		otherAntiAffinity.PreferredDuringSchedulingIgnoredDuringExecution...)
	assert.Equal(t, 1, len(antiaffinity.PreferredDuringSchedulingIgnoredDuringExecution))

	// partial update
	p = Placement{NodeAffinity: na, PodAntiAffinity: antiaffinity}
	ps = &v1.PodSpec{Tolerations: to}
	p.ApplyToPodSpec(ps)
	assert.Equal(t, expected, ps)

	// overridden attributes
	p = Placement{NodeAffinity: na, Tolerations: to, PodAntiAffinity: antiaffinity}
	ps = &v1.PodSpec{Tolerations: placementTestGetTolerations("bar", "baz")}
	p.ApplyToPodSpec(ps)
	assert.Equal(t, expected, ps)

	p = Placement{NodeAffinity: na, PodAntiAffinity: antiaffinity}
	nap := placementTestGenerateNodeAffinity()
	nap.PreferredDuringSchedulingIgnoredDuringExecution[0].Weight = 5
	ps = &v1.PodSpec{Affinity: &v1.Affinity{NodeAffinity: nap}, Tolerations: to}
	p.ApplyToPodSpec(ps)
	assert.Equal(t, expected, ps)
}

func TestPlacementMerge(t *testing.T) {
	to := placementTestGetTolerations("foo", "bar")
	na := placementTestGenerateNodeAffinity()

	var original, with, expected, merged Placement

	original = Placement{}
	with = Placement{Tolerations: to}
	expected = Placement{Tolerations: to}
	merged = original.Merge(with)
	assert.Equal(t, expected, merged)

	original = Placement{NodeAffinity: na}
	with = Placement{Tolerations: to}
	expected = Placement{NodeAffinity: na, Tolerations: to}
	merged = original.Merge(with)
	assert.Equal(t, expected, merged)

	original = Placement{Tolerations: placementTestGetTolerations("bar", "baz")}
	with = Placement{NodeAffinity: na, Tolerations: to}
	expected = Placement{NodeAffinity: na, Tolerations: to}
	merged = original.Merge(with)
	assert.Equal(t, expected, merged)
}

func placementTestGetTolerations(key, value string) []v1.Toleration {
	var ts int64 = 10
	return []v1.Toleration{
		{
			Key:               key,
			Operator:          v1.TolerationOpExists,
			Value:             value,
			Effect:            v1.TaintEffectNoSchedule,
			TolerationSeconds: &ts,
		},
	}
}

func placementAntiAffinity(value string) *v1.PodAntiAffinity {
	return &v1.PodAntiAffinity{
		PreferredDuringSchedulingIgnoredDuringExecution: []v1.WeightedPodAffinityTerm{
			{
				Weight: 50,
				PodAffinityTerm: v1.PodAffinityTerm{
					LabelSelector: &metav1.LabelSelector{
						MatchLabels: map[string]string{
							"app": value,
						},
					},
					TopologyKey: v1.LabelHostname,
				},
			},
		},
	}
}

func placementTestGenerateNodeAffinity() *v1.NodeAffinity {
	return &v1.NodeAffinity{
		RequiredDuringSchedulingIgnoredDuringExecution: &v1.NodeSelector{
			NodeSelectorTerms: []v1.NodeSelectorTerm{
				{
					MatchExpressions: []v1.NodeSelectorRequirement{
						{
							Key:      "foo",
							Operator: v1.NodeSelectorOpExists,
							Values:   []string{"bar"},
						},
					},
				},
			},
		},
		PreferredDuringSchedulingIgnoredDuringExecution: []v1.PreferredSchedulingTerm{
			{
				Weight: 10,
				Preference: v1.NodeSelectorTerm{
					MatchExpressions: []v1.NodeSelectorRequirement{
						{
							Key:      "foo",
							Operator: v1.NodeSelectorOpExists,
							Values:   []string{"bar"},
						},
					},
				},
			},
		},
	}
}
