/*
Copyright 2019 The Rook Authors. All rights reserved.

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

// A PodTemplateSpecTester is a helper exposing methods for testing required Ceph specifications
// common for all Ceph PodTemplateSpecs.
type PodTemplateSpecTester struct {
	t        *testing.T
	template *v1.PodTemplateSpec
}

// NewPodTemplateSpecTester creates a new tester to test the given PodTemplateSpec
func NewPodTemplateSpecTester(t *testing.T, template *v1.PodTemplateSpec) *PodTemplateSpecTester {
	return &PodTemplateSpecTester{t: t, template: template}
}

// AssertLabelsContainCephRequirements asserts that the PodTemplateSpec under test contains labels
// which all Ceph pods should have.
func (pt *PodTemplateSpecTester) AssertLabelsContainCephRequirements(
	daemonType, daemonID, appName, namespace, parentName, resourceKind, appBinaryName string,
) {
	AssertLabelsContainCephRequirements(pt.t, pt.template.ObjectMeta.Labels,
		daemonType, daemonID, appName, namespace, parentName, resourceKind, appBinaryName)
}

// RunFullSuite runs all assertion tests for the PodTemplateSpec under test and its sub-resources.
func (pt *PodTemplateSpecTester) RunFullSuite(
	daemonType, daemonID, appName, namespace, cephImage,
	cpuResourceLimit, cpuResourceRequest,
	memoryResourceLimit, memoryResourceRequest string,
	priorityClassName, parentName, resourceKind, appBinaryName string,
) {
	pt.AssertLabelsContainCephRequirements(daemonType, daemonID, appName, namespace, parentName, resourceKind, appBinaryName)
	pt.Spec().RunFullSuite(daemonType, daemonID, cephImage, cpuResourceLimit, cpuResourceRequest, memoryResourceLimit, memoryResourceRequest, priorityClassName)
}
