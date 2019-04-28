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
	daemonType config.DaemonType, daemonID string,
) {
	keyringSecretName := fmt.Sprintf("rook-ceph-%s-%s-keyring", daemonType, daemonID)
	if daemonType == config.MonType {
		keyringSecretName = "rook-ceph-mons-keyring" // mons share a keyring
	}
	requiredVols := []string{"rook-ceph-config", keyringSecretName}
	if daemonType != config.RbdMirrorType {
		requiredVols = append(requiredVols, "ceph-daemon-data")
	}
	vols := []string{}

	for _, v := range ps.spec.Volumes {
		vols = append(vols, v.Name)
		switch v.Name {
		case "ceph-daemon-data":
			switch daemonType {
			case config.MonType, config.OsdType:
				// mons and osds MUST be host path
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

// AssertVolumesAndMountsMatch asserts that all of the volume mounts in the pod spec under test's
// containers have a volume which sources them. It also asserts that each volume is used at least
// once by any of the mounts in containers.
func (ps *PodSpecTester) AssertVolumesAndMountsMatch() {
	pod := optest.VolumesAndMountsTestDefinition{
		VolumesSpec:     &optest.VolumesSpec{Moniker: "volumes", Volumes: ps.spec.Volumes},
		MountsSpecItems: []*optest.MountsSpec{},
	}
	for _, c := range allContainers(ps.spec) {
		pod.MountsSpecItems = append(
			pod.MountsSpecItems,
			&optest.MountsSpec{Moniker: "mounts of container " + c.Name, Mounts: c.VolumeMounts},
		)
	}
	pod.TestMountsMatchVolumes(ps.t)
}

// AssertRestartPolicyAlways asserts that the pod spec is set to always restart on failure.
func (ps *PodSpecTester) AssertRestartPolicyAlways() {
	assert.Equal(ps.t, v1.RestartPolicyAlways, ps.spec.RestartPolicy)
}

// RunFullSuite runs all assertion tests for the PodSpec under test and its sub-resources.
func (ps *PodSpecTester) RunFullSuite(
	daemonType config.DaemonType,
	resourceName, cephImage, cpuResourceLimit, cpuResourceRequest, memoryResourceLimit, memoryResourceRequest string,
) {
	ps.AssertVolumesAndMountsMatch()
	ps.AssertVolumesMeetCephRequirements(daemonType, resourceName)
	ps.AssertRestartPolicyAlways()
	ps.Containers().RunFullSuite(cephImage, cpuResourceLimit, cpuResourceRequest, memoryResourceLimit, memoryResourceRequest)
}

func allContainers(p *v1.PodSpec) []v1.Container {
	return append(p.InitContainers, p.Containers...)
}
