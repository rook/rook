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
package v1

import (
	v1 "k8s.io/api/core/v1"
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
		t.Affinity.NodeAffinity = p.mergeNodeAffinity(t.Affinity.NodeAffinity)
	}
	if p.PodAffinity != nil {
		t.Affinity.PodAffinity = p.PodAffinity.DeepCopy()
	}
	if p.PodAntiAffinity != nil {
		t.Affinity.PodAntiAffinity = p.PodAntiAffinity.DeepCopy()
	}
	if p.Tolerations != nil {
		t.Tolerations = p.mergeTolerations(t.Tolerations)
	}
	if p.TopologySpreadConstraints != nil {
		t.TopologySpreadConstraints = p.TopologySpreadConstraints
	}
}

func (p Placement) mergeNodeAffinity(nodeAffinity *v1.NodeAffinity) *v1.NodeAffinity {
	// no node affinity is specified yet, so return the placement's nodeAffinity
	result := p.NodeAffinity.DeepCopy()
	if nodeAffinity == nil {
		return result
	}

	// merge the preferred node affinity that was already specified, and the placement's nodeAffinity
	result.PreferredDuringSchedulingIgnoredDuringExecution = append(
		nodeAffinity.PreferredDuringSchedulingIgnoredDuringExecution,
		p.NodeAffinity.PreferredDuringSchedulingIgnoredDuringExecution...)

	// nothing to merge if no affinity was passed in
	if nodeAffinity.RequiredDuringSchedulingIgnoredDuringExecution == nil {
		return result
	}
	// take the desired affinity if there was none on the placement
	if p.NodeAffinity.RequiredDuringSchedulingIgnoredDuringExecution == nil {
		result.RequiredDuringSchedulingIgnoredDuringExecution = nodeAffinity.RequiredDuringSchedulingIgnoredDuringExecution
		return result
	}
	// take the desired affinity node selectors without the need to merge
	if len(nodeAffinity.RequiredDuringSchedulingIgnoredDuringExecution.NodeSelectorTerms) == 0 {
		return result
	}
	// take the placement affinity node selectors without the need to merge
	if len(p.NodeAffinity.RequiredDuringSchedulingIgnoredDuringExecution.NodeSelectorTerms) == 0 {
		// take the placement from the first option since the second isn't specified
		result.RequiredDuringSchedulingIgnoredDuringExecution.NodeSelectorTerms =
			nodeAffinity.RequiredDuringSchedulingIgnoredDuringExecution.NodeSelectorTerms
		return result
	}

	// merge the match expressions together since they are defined in both placements
	// this will only work if we want an "and" between all the expressions, more complex conditions won't work with this merge
	var nodeTerm v1.NodeSelectorTerm
	nodeTerm.MatchExpressions = nodeAffinity.RequiredDuringSchedulingIgnoredDuringExecution.NodeSelectorTerms[0].MatchExpressions
	// only append unique match expressions and match fields
	for _, matchExpression := range p.NodeAffinity.RequiredDuringSchedulingIgnoredDuringExecution.NodeSelectorTerms[0].MatchExpressions {
		found := false
		for _, match := range nodeAffinity.RequiredDuringSchedulingIgnoredDuringExecution.NodeSelectorTerms[0].MatchExpressions {
			if match.Key == matchExpression.Key && match.Operator == matchExpression.Operator {
				found = true
				break
			}
		}
		if !found {
			nodeTerm.MatchExpressions = append(nodeTerm.MatchExpressions, matchExpression)
		}
	}

	nodeTerm.MatchFields = nodeAffinity.RequiredDuringSchedulingIgnoredDuringExecution.NodeSelectorTerms[0].MatchFields
	for _, matchField := range p.NodeAffinity.RequiredDuringSchedulingIgnoredDuringExecution.NodeSelectorTerms[0].MatchFields {
		found := false
		for _, match := range nodeAffinity.RequiredDuringSchedulingIgnoredDuringExecution.NodeSelectorTerms[0].MatchFields {
			if match.Key == matchField.Key && match.Operator == matchField.Operator {
				found = true
				break
			}
		}
		if !found {
			nodeTerm.MatchFields = append(nodeTerm.MatchFields, matchField)
		}
	}

	result.RequiredDuringSchedulingIgnoredDuringExecution.NodeSelectorTerms[0] = nodeTerm

	return result
}

func (p Placement) mergeTolerations(tolerations []v1.Toleration) []v1.Toleration {
	// no toleration is specified yet, return placement's toleration
	if tolerations == nil {
		return p.Tolerations
	}

	// only append the tolerations that are not already in the pod spec
	// to avoid duplicates values
	for _, toleration := range p.Tolerations {
		found := false
		for _, t := range tolerations {
			if toleration.Key == t.Key && toleration.Operator == t.Operator && toleration.Value == t.Value {
				found = true
				break
			}
		}
		if !found {
			tolerations = append(tolerations, toleration)
		}
	}

	return tolerations
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
		ret.Tolerations = ret.mergeTolerations(with.Tolerations)
	}
	if with.TopologySpreadConstraints != nil {
		ret.TopologySpreadConstraints = with.TopologySpreadConstraints
	}
	return ret
}

// GetMgrPlacement returns the placement for the MGR service
func GetMgrPlacement(p PlacementSpec) Placement {
	return p.All().Merge(p[KeyMgr])
}

// GetMonPlacement returns the placement for the MON service
func GetMonPlacement(p PlacementSpec) Placement {
	return p.All().Merge(p[KeyMon])
}

// GetArbiterPlacement returns the placement for the arbiter MON service
func GetArbiterPlacement(p PlacementSpec) Placement {
	// If the mon is the arbiter in a stretch cluster and its placement is specified, return it
	// without merging with the "all" placement so it can be handled separately from all other daemons
	return p[KeyMonArbiter]
}

// GetOSDPlacement returns the placement for the OSD service
func GetOSDPlacement(p PlacementSpec) Placement {
	return p.All().Merge(p[KeyOSD])
}
