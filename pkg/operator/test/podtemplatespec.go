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

// A PodTemplateSpecTester is a helper exposing methods for testing required Rook specifications
// common for all Rook PodTemplateSpecs.
type PodTemplateSpecTester struct {
	t        *testing.T
	template *v1.PodTemplateSpec
}

// NewPodTemplateSpecTester creates a new tester to test the given PodTemplateSpec
func NewPodTemplateSpecTester(t *testing.T, template *v1.PodTemplateSpec) *PodTemplateSpecTester {
	return &PodTemplateSpecTester{t: t, template: template}
}

// AssertLabelsContainRookRequirements asserts that the PodTemplateSpec under test contains labels
// which all Rook pods should have.
func (pt *PodTemplateSpecTester) AssertLabelsContainRookRequirements(appName string) {
	AssertLabelsContainRookRequirements(pt.t, pt.template.ObjectMeta.Labels, appName)
}

// RunFullSuite runs all assertion tests for the PodTemplateSpec under test and its sub-resources.
func (pt *PodTemplateSpecTester) RunFullSuite(
	appName string,
	resourceExpectations ResourceLimitExpectations,
) {
	pt.AssertLabelsContainRookRequirements(appName)
	pt.Spec().RunFullSuite(resourceExpectations)
}
