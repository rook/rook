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
package v1alpha2

import (
	"k8s.io/api/core/v1"
)

func (p PlacementSpec) All() Placement {
	return p[KeyAll]
}

// ApplyToPodSpec adds placement to a pod spec
func (p Placement) ApplyToPodSpec(t *v1.PodSpec) {
	if t.Affinity == nil {
		t.Affinity = &v1.Affinity{}
	}
	if p.NodeAffinity != nil {
		t.Affinity.NodeAffinity = p.NodeAffinity
	}
	if p.PodAffinity != nil {
		t.Affinity.PodAffinity = p.PodAffinity
	}
	if p.PodAntiAffinity != nil {
		t.Affinity.PodAntiAffinity = p.PodAntiAffinity
	}

	if p.Tolerations != nil {
		t.Tolerations = p.Tolerations
	}
}

// Merge returns a Placement which results from merging the attributes of the
// original Placement with the attributes of the supplied one. The supplied
// Placement's attributes will override the original ones if defined.
func (p Placement) Merge(with Placement) Placement {
	ret := p
	if with.NodeAffinity != nil {
		ret.NodeAffinity = with.NodeAffinity
	}
	if with.PodAffinity != nil {
		ret.PodAffinity = with.PodAffinity
	}
	if with.PodAntiAffinity != nil {
		ret.PodAntiAffinity = with.PodAntiAffinity
	}
	if with.Tolerations != nil {
		ret.Tolerations = with.Tolerations
	}
	return ret
}
