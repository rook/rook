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
	"regexp"
	"testing"

	"github.com/stretchr/testify/assert"
	v1 "k8s.io/api/core/v1"
)

// A ContainersTester is a helper exposing methods for testing required Rook specifications common
// for all Rook containers.
type ContainersTester struct {
	t          *testing.T
	containers []v1.Container
}

// ResourceLimitExpectations allows a test to define expectations for resource limits on containers.
// If any field is left as an empty string, that field will not be tested.
type ResourceLimitExpectations struct {
	CPUResourceLimit      string
	CPUResourceRequest    string
	MemoryResourceLimit   string
	MemoryResourceRequest string
}

// Containers creates a ContainersTester from a parent PodSpecTester. Because ContainersTester is
// intended to test the full list of containers (both init and run containers) in a PodSpec, this
// method is the only way of creating a ContainersTester.
func (ps *PodSpecTester) Containers() *ContainersTester {
	return NewContainersSpecTester(ps.t, allContainers(ps.spec))
}

// NewContainersSpecTester creates a new tester for the given container spec.
func NewContainersSpecTester(t *testing.T, cc []v1.Container) *ContainersTester {
	return &ContainersTester{
		t:          t,
		containers: cc,
	}
}

// AssertArgReferencesMatchEnvVars asserts that for each container under test, any references to
// Kubernetes environment variables (e.g., $(POD_NAME)), have an environment variable set to source
// the value.
func (ct *ContainersTester) AssertArgReferencesMatchEnvVars() {
	for _, c := range ct.containers {
		localcontainer := c
		assert.Subset(ct.t, varNames(&localcontainer), argEnvReferences(&localcontainer),
			"container: "+c.Name,
			"references to env vars in args do not match env vars",
			"args:", c.Args, "envs:", c.Env)
	}
}

// AssertResourceSpec asserts that the container under test's resource limits/requests match the
// given (in string format) resource limits/requests.
func (ct *ContainersTester) AssertResourceSpec(expectations ResourceLimitExpectations) {
	for _, c := range ct.containers {
		if expectations.CPUResourceLimit != "" {
			assert.Equal(ct.t, expectations.CPUResourceLimit, c.Resources.Limits.Cpu().String())
		}
		if expectations.CPUResourceRequest != "" {
			assert.Equal(ct.t, expectations.CPUResourceRequest, c.Resources.Requests.Cpu().String())
		}
		if expectations.MemoryResourceLimit != "" {
			assert.Equal(ct.t, expectations.MemoryResourceLimit, c.Resources.Limits.Memory().String())
		}
		if expectations.MemoryResourceRequest != "" {
			assert.Equal(ct.t, expectations.MemoryResourceRequest, c.Resources.Requests.Memory().String())
		}
	}
}

// RunFullSuite runs all assertion tests for the Containers under test.
func (ct *ContainersTester) RunFullSuite(resourceExpectations ResourceLimitExpectations) {
	ct.AssertArgReferencesMatchEnvVars()
	ct.AssertResourceSpec(resourceExpectations)
}

func argEnvReferences(c *v1.Container) []string {
	argRefSet := map[string]bool{}
	for _, a := range c.Args {
		argRefRegex, e := regexp.Compile(`\$\(([a-zA-Z][a-zA-Z0-9_]*)\)`)
		if e != nil {
			panic("could not compile argument reference regexp")
		}
		matches := argRefRegex.FindAllStringSubmatch(a, -1)
		for _, m := range matches {
			argRefSet[m[1]] = true
		}
	}
	refs := []string{}
	for r := range argRefSet {
		refs = append(refs, r)
	}
	return refs
}

func varNames(c *v1.Container) []string {
	vars := []string{}
	for _, v := range c.Env {
		vars = append(vars, v.Name)
	}
	return vars
}
