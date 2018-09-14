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
	"testing"

	"github.com/rook/rook/pkg/operator/k8sutil"
	"github.com/rook/rook/pkg/operator/test"
)

func TestPodVolumes(t *testing.T) {
	type args struct {
		dataDirHostPath string
	}
	t.Run("Empty string dataDirHostPath is EmptyDir volume", func(t *testing.T) {
		err := test.VolumeIsEmptyDir(k8sutil.DataDirVolume, PodVolumes(""))
		if err != nil {
			t.Errorf("PodVolumes(\"\") - dataDirHostPath is not EmptyDir: %s", err.Error())
		}
	})
	t.Run("Specified dataDirHostPath is HostPath volume", func(t *testing.T) {
		err := test.VolumeIsHostPath(k8sutil.DataDirVolume, "/dev/sdb", PodVolumes("/dev/sdb"))
		if err != nil {
			t.Errorf("PodVolumes(\"/dev/sdb\") - dataDirHostPath is not HostPath: %s", err.Error())
		}
	})
}

func TestMountsMatchVolumes(t *testing.T) {
	volsMountsTestDef := test.VolumesAndMountsTestDefinition{
		VolumesSpec: &test.VolumesSpec{
			Moniker: "PodVolumes(\"/dev/sdc\")", Volumes: PodVolumes("/dev/sdc")},
		MountsSpecItems: []*test.MountsSpec{
			{Moniker: "CephVolumeMounts()", Mounts: CephVolumeMounts()},
			{Moniker: "RookVolumeMounts()", Mounts: RookVolumeMounts()}},
	}
	volsMountsTestDef.TestMountsMatchVolumes(t)
}
