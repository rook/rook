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
package k8sutil

import (
	"k8s.io/api/core/v1"
)

// Placement encapsulates the various kubernetes options that control where
// pods are scheduled and executed.
type Placement struct {
	NodeAffinity *v1.NodeAffinity `json:"nodeAffinity,omitempty"`
	Tolerations  []v1.Toleration  `json:"tolerations,omitemtpy"`
}

func (p Placement) ApplyToPodSpec(t *v1.PodSpec) {
	if p.NodeAffinity != nil {
		if t.Affinity == nil {
			t.Affinity = &v1.Affinity{}
		}
		t.Affinity.NodeAffinity = p.NodeAffinity
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
	if with.Tolerations != nil {
		ret.Tolerations = with.Tolerations
	}
	return ret
}
