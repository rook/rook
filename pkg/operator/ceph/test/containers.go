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
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"k8s.io/api/core/v1"
)

var requiredEnvVars = []string{
	"POD_NAME", "POD_NAMESPACE", "NODE_NAME",
	"ROOK_CEPH_MON_HOST", "ROOK_CEPH_MON_INITIAL_MEMBERS",
}

// A ContainersTester is a helper exposing methods for testing required Ceph specifications common
// for all Ceph containers.
type ContainersTester struct {
	t          *testing.T
	containers []v1.Container
}

// Containers creates a ContainersTester from a parent PodSpecTester. Because ContainersTester is
// intended to test the full list of containers (both init and run containers) in a PodSpec, this
// method is the only way of creating a ContainersTester.
func (ps *PodSpecTester) Containers() *ContainersTester {
	return &ContainersTester{
		t:          ps.t,
		containers: allContainers(ps.spec),
	}
}

// AssertArgsContainCephRequirements asserts that all Ceph containers under test have the flags
// required for all Ceph containers.
func (ct *ContainersTester) AssertArgsContainCephRequirements() {
	for _, c := range ct.containers {
		if !isCephCommand(c.Command) {
			continue // don't consider containers that aren't Ceph commands
		}
		requiredFlags := []string{
			"--log-to-stderr=true",
			"--err-to-stderr=true",
			"--mon-cluster-log-to-stderr=true",
			"--log-stderr-prefix=debug ",
			"--mon-host=$(ROOK_CEPH_MON_HOST)",
			"--mon-initial-members=$(ROOK_CEPH_MON_INITIAL_MEMBERS)",
		}
		assert.Subset(ct.t, c.Args, requiredFlags, "required Ceph flags are not in container"+c.Name)
		fsidPresent := false
		for _, a := range c.Args {
			if strings.HasPrefix(a, "--fsid=") {
				fsidPresent = true
				break
			}
		}
		assert.True(ct.t, fsidPresent, "--fsid=XXXXXXXX is not present in container args:", c.Args)
	}
}

// AssertEnvVarsContainCephRequirements asserts that all Ceph containers under test have the
// environment variables required for all Ceph containers.
func (ct *ContainersTester) AssertEnvVarsContainCephRequirements() {
	for _, c := range ct.containers {
		if !isCephCommand(c.Command) {
			continue // don't consider containers that aren't Ceph commands
		}
		assert.Subset(ct.t, varNames(&c), requiredEnvVars)
		for _, e := range c.Env {
			// For the required env vars, make sure they are sourced as expected
			switch e.Name {
			case "POD_NAME":
				assert.Equal(ct.t, "metadata.name", e.ValueFrom.FieldRef.FieldPath,
					"POD_NAME env var does not have the appropriate source:", e)
			case "POD_NAMESPACE":
				assert.Equal(ct.t, "metadata.namespace", e.ValueFrom.FieldRef.FieldPath,
					"POD_NAMESPACE env var does not have the appropriate source:", e)
			case "NODE_NAME":
				assert.Equal(ct.t, "spec.nodeName", e.ValueFrom.FieldRef.FieldPath,
					"NODE_NAME env var does not have the appropriate source:", e)
			case "ROOK_CEPH_MON_HOST":
				assert.Equal(ct.t, "rook-ceph-config", e.ValueFrom.SecretKeyRef.LocalObjectReference.Name,
					"ROOK_CEPH_MON_HOST env var does not have appropriate source:", e)
				assert.Equal(ct.t, "mon_host", e.ValueFrom.SecretKeyRef.Key,
					"ROOK_CEPH_MON_HOST env var does not have appropriate source:", e)
			case "ROOK_CEPH_MON_INITIAL_MEMBERS":
				assert.Equal(ct.t, "rook-ceph-config", e.ValueFrom.SecretKeyRef.LocalObjectReference.Name,
					"ROOK_CEPH_MON_INITIAL_MEMBERS env var does not have appropriate source:", e)
				assert.Equal(ct.t, "mon_initial_members", e.ValueFrom.SecretKeyRef.Key,
					"ROOK_CEPH_MON_INITIAL_MEMBERS env var does not have appropriate source:", e)
			}
		}
	}
}

// AssertArgReferencesMatchEnvVars asserts that for each container under test, any references to
// Kubernetes environment variables (e.g., $(POD_NAME)), have an environment variable set to source
// the value.
func (ct *ContainersTester) AssertArgReferencesMatchEnvVars() {
	for _, c := range ct.containers {
		assert.Subset(ct.t, varNames(&c), argEnvReferences(&c),
			"container: "+c.Name,
			"references to env vars in args do not match env vars",
			"args:", c.Args, "envs:", c.Env)
	}
	// also make sure there are no extraneous env vars
	// the only allowed extraneous vars are the required vars
	assert.ElementsMatch(ct.t, ct.allNonrequiredVarNames(), ct.allNonrequiredArgEnvReferences())
}

// AssertCephImagesMatch asserts that for all Ceph containers under test, the Ceph image used is the
// expected image.
func (ct *ContainersTester) AssertCephImagesMatch(image string) {
	for _, c := range ct.containers {
		if !isCephCommand(c.Command) {
			continue // don't consider containers that aren't Ceph commands
		}
		assert.Equal(ct.t, image, c.Image, "Ceph image for container "+c.Name+"does not match expected")
	}
}

// AssertResourceSpec asserts that the container under test's resource limits/requests match the
// given (in string format) resource limits/requests.
func (ct *ContainersTester) AssertResourceSpec(cpuResourceLimit, memoryResourceRequest string) {
	for _, c := range ct.containers {
		assert.Equal(ct.t, cpuResourceLimit, c.Resources.Limits.Cpu().String())
		assert.Equal(ct.t, memoryResourceRequest, c.Resources.Requests.Memory().String())
	}
}

// RunFullSuite runs all assertion tests for the Containers under test.
func (ct *ContainersTester) RunFullSuite(cephImage, cpuResourceLimit, memoryResourceRequest string) {
	ct.AssertEnvVarsContainCephRequirements()
	ct.AssertArgReferencesMatchEnvVars()
	ct.AssertArgsContainCephRequirements()
	ct.AssertCephImagesMatch(cephImage)
	ct.AssertResourceSpec(cpuResourceLimit, memoryResourceRequest)
}

func isCephCommand(command []string) bool {
	// assume a ceph command is identified by the existence of the word "ceph" somewhere in the
	// first command word.
	// Are Ceph commands: ["ceph-mon", ...], ["ceph-mgr", ...], ["ceph", "config", ...]
	// Are not: ["cp", "/etc/ceph/...], ...
	return strings.Contains(command[0], "ceph")
}

func argEnvReferences(c *v1.Container) []string {
	argRefSet := map[string]bool{}
	for _, a := range c.Args {
		argRefRegex, e := regexp.Compile("\\$\\(([a-zA-Z][a-zA-Z0-9_]*)\\)")
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

func (ct *ContainersTester) allNonrequiredArgEnvReferences() []string {
	allSet := map[string]bool{}
	for _, c := range ct.containers {
		for _, r := range argEnvReferences(&c) {
			allSet[r] = true
		}
	}
	for _, req := range requiredEnvVars {
		allSet[req] = false // required env vars NOT required
	}
	all := []string{}
	for r, req := range allSet {
		if req {
			all = append(all, r)
		}
	}
	return all
}

func (ct *ContainersTester) allNonrequiredVarNames() []string {
	allSet := map[string]bool{}
	for _, c := range ct.containers {
		for _, v := range varNames(&c) {
			allSet[v] = true
		}
	}
	for _, req := range requiredEnvVars {
		allSet[req] = false // required env vars NOT required
	}
	all := []string{}
	for v, req := range allSet {
		if req {
			all = append(all, v)
		}
	}
	return all
}
