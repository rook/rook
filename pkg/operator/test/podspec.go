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
	"testing"

	v1 "k8s.io/api/core/v1"
)

// A PodSpecTester is a helper exposing methods for testing required Ceph specifications common for
// all Ceph PodSpecs.
type PodSpecTester struct {
	t    *testing.T
	spec *v1.PodSpec
}

// Spec creates a PodSpecTester from a parent PodTemplateSpecTester.
func (pt *PodTemplateSpecTester) Spec() *PodSpecTester {
	return NewPodSpecTester(pt.t, &pt.template.Spec)
}

// NewPodSpecTester creates a new tester to test the given PodSpec.
func NewPodSpecTester(t *testing.T, spec *v1.PodSpec) *PodSpecTester {
	return &PodSpecTester{t: t, spec: spec}
}

// AssertVolumesAndMountsMatch asserts that all of the volume mounts in the pod spec under test's
// containers have a volume which sources them. It also asserts that each volume is used at least
// once by any of the mounts in containers.
func (ps *PodSpecTester) AssertVolumesAndMountsMatch() {
	pod := VolumesAndMountsTestDefinition{
		VolumesSpec:     &VolumesSpec{Moniker: "volumes", Volumes: ps.spec.Volumes},
		MountsSpecItems: []*MountsSpec{},
	}
	for _, c := range allContainers(ps.spec) {
		pod.MountsSpecItems = append(
			pod.MountsSpecItems,
			&MountsSpec{Moniker: "mounts of container " + c.Name, Mounts: c.VolumeMounts},
		)
	}
	pod.TestMountsMatchVolumes(ps.t)
}

// RunFullSuite runs all assertion tests for the PodSpec under test and its sub-resources.
func (ps *PodSpecTester) RunFullSuite(resourceExpectations ResourceLimitExpectations) {
	ps.AssertVolumesAndMountsMatch()
	ps.Containers().RunFullSuite(resourceExpectations)
}

func allContainers(p *v1.PodSpec) []v1.Container {
	return append(p.InitContainers, p.Containers...)
}
