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

/*
The goal here is not to test every individual specification of the pod/container. Testing that the
generated pod spec has each piece set in the Rook code isn't a particularly effective use of unit
tests. Any time the Rook code changes the pod spec intentionally, the unit test changes in the
exact same way, which doesn't really help prevent against errors where devs are changing the wrong
spec values.

Instead, the unit tests should focus on testing things that are universal truths about
Ceph pod specifications that can help catch when pods ...
 - do not have the minimum requirements for running Ceph tools/daemons
 - have vestigial values set that are no longer needed
 - have references to absent resources (e.g., a volume mount without a volume source)

In this way, unit tests for pod specifications can be consistent and shared between all Ceph pods
created by the Rook operator. With this consistency between unit tests, there should be increased
consistency between the Ceph pods that Rook creates, ensuring a consistent user experience
interacting with pods.
*/

import (
	"fmt"
	"testing"

	"github.com/rook/rook/pkg/operator/ceph/config"
	optest "github.com/rook/rook/pkg/operator/test"
	"github.com/stretchr/testify/assert"
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

// AssertVolumesMeetCephRequirements asserts that all the required Ceph volumes exist in the pod
// spec under test, Volumes list.
func (ps *PodSpecTester) AssertVolumesMeetCephRequirements(
	daemonType, daemonID string,
) {
	// #nosec because of the word `Secret`
	keyringSecretName := fmt.Sprintf("rook-ceph-%s-%s-keyring", daemonType, daemonID)
	if daemonType == config.MonType {
		// #nosec because of the word `Secret`
		keyringSecretName = "rook-ceph-mons-keyring"
	}
	// CephFS mirror has no index so the daemon name is just "rook-ceph-fs-mirror"
	if daemonType == config.FilesystemMirrorType {
		keyringSecretName = fmt.Sprintf("rook-ceph-%s-keyring", daemonType)
	}
	requiredVols := []string{"rook-config-override", keyringSecretName}
	if daemonType != config.RbdMirrorType && daemonType != config.FilesystemMirrorType {
		requiredVols = append(requiredVols, "ceph-daemon-data")
	}
	vols := []string{}

	for _, v := range ps.spec.Volumes {
		vols = append(vols, v.Name)
		switch v.Name {
		case "ceph-daemon-data":
			switch daemonType {
			case config.MonType:
				// mons may be host path or pvc
				assert.True(ps.t,
					v.VolumeSource.HostPath != nil || v.VolumeSource.PersistentVolumeClaim != nil,
					string(daemonType)+" daemon should be host path or pvc:", v)
			case config.OsdType:
				// osds MUST be host path
				assert.NotNil(ps.t, v.VolumeSource.HostPath,
					string(daemonType)+" daemon should be host path:", v)
			case config.MgrType, config.MdsType, config.RgwType:
				// mgrs, mdses, and rgws MUST be host path
				assert.NotNil(ps.t, v.VolumeSource.EmptyDir,
					string(daemonType)+" daemon should be empty dir:", v)
			}
		case "rook-ceph-config":
			assert.Equal(ps.t, "rook-ceph-config", v.VolumeSource.ConfigMap.LocalObjectReference.Name,
				"Ceph config volume source is wrong path:", v)
		case keyringSecretName:
			assert.Equal(ps.t, keyringSecretName, v.VolumeSource.Secret.SecretName,
				"daemon keyring volume source is wrong path:", v)
		}
	}
	assert.Subset(ps.t, vols, requiredVols,
		"required volumes don't exist in pod spec's volume list:", ps.spec.Volumes)
}

// AssertRestartPolicyAlways asserts that the pod spec is set to always restart on failure.
func (ps *PodSpecTester) AssertRestartPolicyAlways() {
	assert.Equal(ps.t, v1.RestartPolicyAlways, ps.spec.RestartPolicy)
}

// AssertChownContainer ensures that the init container to chown the Ceph data dir is present for
// Ceph daemons.
func (ps *PodSpecTester) AssertChownContainer(daemonType string) {
	switch daemonType {
	case config.MonType, config.MgrType, config.OsdType, config.MdsType, config.RgwType, config.RbdMirrorType:
		assert.True(ps.t, containerExists("chown-container-data-dir", ps.spec))
	}
}

// AssertPriorityClassNameMatch asserts that the pod spec has priorityClassName set to be the same
func (ps *PodSpecTester) AssertPriorityClassNameMatch(name string) {
	assert.Equal(ps.t, name, ps.spec.PriorityClassName)
}

// RunFullSuite runs all assertion tests for the PodSpec under test and its sub-resources.
func (ps *PodSpecTester) RunFullSuite(
	daemonType, resourceName, cephImage,
	cpuResourceLimit, cpuResourceRequest, memoryResourceLimit, memoryResourceRequest string, priorityClassName string,
) {
	resourceExpectations := optest.ResourceLimitExpectations{
		CPUResourceLimit:      cpuResourceLimit,
		MemoryResourceLimit:   memoryResourceLimit,
		CPUResourceRequest:    cpuResourceRequest,
		MemoryResourceRequest: memoryResourceRequest,
	}
	ops := optest.NewPodSpecTester(ps.t, ps.spec)
	ops.RunFullSuite(resourceExpectations)

	ps.AssertVolumesMeetCephRequirements(daemonType, resourceName)
	ps.AssertRestartPolicyAlways()
	ps.AssertChownContainer(daemonType)
	ps.AssertPriorityClassNameMatch(priorityClassName)
	ps.Containers().RunFullSuite(cephImage, cpuResourceLimit, cpuResourceRequest, memoryResourceLimit, memoryResourceRequest)
}

func allContainers(p *v1.PodSpec) []v1.Container {
	return append(p.InitContainers, p.Containers...)
}

func containerExists(containerName string, p *v1.PodSpec) bool {
	for _, c := range p.InitContainers {
		if c.Name == containerName {
			return true
		}
	}
	return false
}
