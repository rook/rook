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

package spec

import (
	"math"
	"testing"

	opconfig "github.com/rook/rook/pkg/operator/ceph/config"
	"github.com/rook/rook/pkg/operator/k8sutil"
	"github.com/rook/rook/pkg/operator/test"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
)

func TestPodVolumes(t *testing.T) {
	dataPathMap := opconfig.NewDatalessDaemonDataPathMap("rook-ceph", "/var/lib/rook")

	if err := test.VolumeIsEmptyDir(k8sutil.DataDirVolume, PodVolumes(dataPathMap, "", false)); err != nil {
		t.Errorf("PodVolumes(\"\") - data dir source is not EmptyDir: %s", err.Error())
	}
	if err := test.VolumeIsHostPath(k8sutil.DataDirVolume, "/dev/sdb", PodVolumes(dataPathMap, "/dev/sdb", false)); err != nil {
		t.Errorf("PodVolumes(\"/dev/sdb\") - data dir source is not HostPath: %s", err.Error())
	}
}

func TestMountsMatchVolumes(t *testing.T) {

	dataPathMap := opconfig.NewDatalessDaemonDataPathMap("rook-ceph", "/var/lib/rook")

	volsMountsTestDef := test.VolumesAndMountsTestDefinition{
		VolumesSpec: &test.VolumesSpec{
			Moniker: "PodVolumes(\"/dev/sdc\")", Volumes: PodVolumes(dataPathMap, "/dev/sdc", false)},
		MountsSpecItems: []*test.MountsSpec{
			{Moniker: "CephVolumeMounts(true)", Mounts: CephVolumeMounts(dataPathMap, false)},
			{Moniker: "RookVolumeMounts(true)", Mounts: RookVolumeMounts(dataPathMap, false)}},
	}
	volsMountsTestDef.TestMountsMatchVolumes(t)

	volsMountsTestDef = test.VolumesAndMountsTestDefinition{
		VolumesSpec: &test.VolumesSpec{
			Moniker: "PodVolumes(\"/dev/sdc\")", Volumes: PodVolumes(dataPathMap, "/dev/sdc", true)},
		MountsSpecItems: []*test.MountsSpec{
			{Moniker: "CephVolumeMounts(false)", Mounts: CephVolumeMounts(dataPathMap, true)},
			{Moniker: "RookVolumeMounts(false)", Mounts: RookVolumeMounts(dataPathMap, true)}},
	}
	volsMountsTestDef.TestMountsMatchVolumes(t)
}

func TestCheckPodMemory(t *testing.T) {
	// This value is in MB
	const PodMinimumMemory uint64 = 1024

	// A value for the memory used in the tests
	var memory_value = int64(PodMinimumMemory * 8 * uint64(math.Pow10(6)))

	// Case 1: No memory limits, no memory requested
	test_resource := v1.ResourceRequirements{}

	if err := CheckPodMemory(test_resource, PodMinimumMemory); err != nil {
		t.Errorf("Error case 1: %s", err.Error())
	}

	// Case 2: memory limit and memory requested
	test_resource = v1.ResourceRequirements{
		Limits: v1.ResourceList{
			v1.ResourceMemory: *resource.NewQuantity(memory_value, resource.BinarySI),
		},
		Requests: v1.ResourceList{
			v1.ResourceMemory: *resource.NewQuantity(memory_value, resource.BinarySI),
		},
	}

	if err := CheckPodMemory(test_resource, PodMinimumMemory); err != nil {
		t.Errorf("Error case 2: %s", err.Error())
	}

	// Only memory requested
	test_resource = v1.ResourceRequirements{
		Requests: v1.ResourceList{
			v1.ResourceMemory: *resource.NewQuantity(memory_value, resource.BinarySI),
		},
	}

	if err := CheckPodMemory(test_resource, PodMinimumMemory); err != nil {
		t.Errorf("Error case 3: %s", err.Error())
	}
}
