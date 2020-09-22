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
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	v1 "k8s.io/api/core/v1"
)

/*
 * Simple Volume and VolumeMount Tests
 */

// VolumeExists returns a descriptive error if the volume does not exist.
func VolumeExists(volumeName string, volumes []v1.Volume) error {
	_, err := getVolume(volumeName, volumes)
	return err
}

// VolumeIsEmptyDir returns a descriptive error if the volume does not exist or is not an empty dir
func VolumeIsEmptyDir(volumeName string, volumes []v1.Volume) error {
	volume, err := getVolume(volumeName, volumes)
	if err == nil && volume.VolumeSource.EmptyDir == nil {
		return fmt.Errorf("volume %s is not EmptyDir: %s", volumeName, HumanReadableVolume(volume))
	}
	if err == nil && volume.VolumeSource.HostPath != nil {
		return fmt.Errorf("volume %s is both EmptyDir and HostPath: %s", volumeName, HumanReadableVolume(volume))
	}
	return err
}

// VolumeIsHostPath returns a descriptive error if the volume does not exist, is not a HostPath
// volume, or if the volume's path is not as expected.
func VolumeIsHostPath(volumeName, path string, volumes []v1.Volume) error {
	volume, err := getVolume(volumeName, volumes)
	if err == nil && volume.VolumeSource.HostPath == nil {
		return fmt.Errorf("volume %s is not HostPath: %s", volumeName, HumanReadableVolume(volume))
	}
	if err == nil && volume.VolumeSource.HostPath.Path != path {
		return fmt.Errorf("volume %s is HostPath but has wrong path: %s", volumeName, HumanReadableVolume(volume))
	}
	if err == nil && volume.VolumeSource.EmptyDir != nil {
		return fmt.Errorf("volume %s is both HostPath and EmptyDir: %s", volumeName, HumanReadableVolume(volume))
	}
	return err
}

// VolumeMountExists returns returns a descriptive error if the volume mount does not exist.
func VolumeMountExists(mountName string, mounts []v1.VolumeMount) error {
	_, err := getMount(mountName, mounts)
	return err
}

/*
 * Human-readable representations of Volumes and VolumeMounts
 */

// HumanReadableVolumes returns a string representation of a list of Kubernetes volumes which is
// more compact and human readable than the default string go prints.
func HumanReadableVolumes(volumes []v1.Volume) string {
	stringVols := []string{}
	for _, volume := range volumes {
		localvolume := volume
		stringVols = append(stringVols, HumanReadableVolume(&localvolume))
	}
	return fmt.Sprintf("%v", stringVols)
}

// HumanReadableVolume returns a string representation of a Kubernetes volume which is more compact
// and human readable than the default string go prints.
func HumanReadableVolume(v *v1.Volume) string {
	var sourceString string
	if v.VolumeSource.EmptyDir != nil {
		sourceString = "EmptyDir"
	} else if v.VolumeSource.HostPath != nil {
		sourceString = v.VolumeSource.HostPath.Path
	} else {
		// If can't convert the vol source to something human readable, just output something useful
		sourceString = fmt.Sprintf("%v", v.VolumeSource)
	}
	return fmt.Sprintf("{%s : %s}", v.Name, sourceString)
}

// HumanReadableVolumeMounts returns a string representation of a list of Kubernetes volume mounts which
// is more compact and human readable than the default string go prints.
func HumanReadableVolumeMounts(mounts []v1.VolumeMount) string {
	stringMounts := []string{}
	for _, mount := range mounts {
		localmount := mount
		stringMounts = append(stringMounts, HumanReadableVolumeMount(&localmount))
	}
	return fmt.Sprintf("%v", stringMounts)
}

// HumanReadableVolumeMount returns a string representation of a Kubernetes volume mount which is more
// compact and human readable than the default string go prints.
func HumanReadableVolumeMount(m *v1.VolumeMount) string {
	return fmt.Sprintf("{%s : %s}", m.Name, m.MountPath)
}

/*
 * Fully-implemented test for matching Volumes and VolumeMounts
 */

// VolumesSpec is a struct which includes a list of Kubernetes volumes as well as additional
// metadata about the volume list for better identification during tests.
type VolumesSpec struct {
	// Moniker is a name given to the list to help identify it
	Moniker string
	Volumes []v1.Volume
}

// MountsSpec is a struct which includes a list of Kubernetes volume mounts as well as additional
// metadata about the volume mount list for better identification during tests.
type MountsSpec struct {
	// Moniker is a name given to the list to help identify it
	Moniker string
	Mounts  []v1.VolumeMount
}

// VolumesAndMountsTestDefinition defines which volumes and mounts to test and what those values
// should be. The test is intended to be defined with VolumesSpec defined as a pod's volumes, and
// the list of MountsSpec items defined as the volume mounts from every container in the pod.
type VolumesAndMountsTestDefinition struct {
	VolumesSpec     *VolumesSpec
	MountsSpecItems []*MountsSpec
}

// TestMountsMatchVolumes tests two things:
// (1) That each volume mount in each every MountsSpec has a corresponding volume to source it
//     in the VolumesSpec
// (2) That there are no extraneous volumes defined in the VolumesSpec that do not have a
//     corresponding volume mount in any of the MountsSpec items
func (d *VolumesAndMountsTestDefinition) TestMountsMatchVolumes(t *testing.T) {
	// Run a test for each MountsSpec item to verify that all the volume mounts within each item
	// have a corresponding volume in VolumesSpec to source it
	for _, mountsSpec := range d.MountsSpecItems {
		t.Run(
			fmt.Sprintf("%s mounts match volumes in %s", mountsSpec.Moniker, d.VolumesSpec.Moniker),
			func(t *testing.T) {
				for _, mount := range mountsSpec.Mounts {
					localmount := mount
					assert.Nil(t, VolumeExists(mount.Name, d.VolumesSpec.Volumes),
						"%s volume mount %s does not have a corresponding %s volume to source it: %s",
						mountsSpec.Moniker, HumanReadableVolumeMount(&localmount),
						d.VolumesSpec.Moniker, HumanReadableVolumes(d.VolumesSpec.Volumes))
				}
			},
		)
	}

	// Test that each volume in VolumesSpec has a usage in at least one of the volume mounts in at
	// least one of the MountsSpecs items
	t.Run(
		fmt.Sprintf("No extraneous %s volumes exist", d.VolumesSpec.Moniker),
		func(t *testing.T) {
			for _, volume := range d.VolumesSpec.Volumes {
				localvolume := volume
				assert.Nil(t, mountExistsInMountsSpecItems(volume.Name, d.MountsSpecItems),
					"%s volume %s is not used by any volume mount: %v",
					d.VolumesSpec.Moniker, HumanReadableVolume(&localvolume),
					humanReadableMountsSpecItems(d.MountsSpecItems))
			}
		},
	)
}

/*
 * Helpers
 */

func mountExistsInMountsSpecItems(name string, mountsSpecItems []*MountsSpec) error {
	for _, mountsSpec := range mountsSpecItems {
		if err := VolumeMountExists(name, mountsSpec.Mounts); err == nil {
			return nil // successfully found that the mount exists at least once
		}
	}
	return fmt.Errorf("") // not in any mounts; calling func will output a better error
}

func humanReadableMountsSpecItems(mountsSpecItems []*MountsSpec) (humanReadable string) {
	for _, mountsSpec := range mountsSpecItems {
		humanReadable = fmt.Sprintf("%s%s - %s\n", humanReadable,
			mountsSpec.Moniker, HumanReadableVolumeMounts(mountsSpec.Mounts),
		)
	}
	return
}

const (
	volNotFound = "Volume Was Not Found by getVolume()"
	mntNotFound = "VolumeMount Was Not Found by getMount()"
)

func getVolume(volumeName string, volumes []v1.Volume) (*v1.Volume, error) {
	for _, volume := range volumes {
		if volume.Name == volumeName {
			return &volume, nil
		}
	}
	return &v1.Volume{Name: volNotFound},
		fmt.Errorf("volume %s does not exist in %s", volumeName, HumanReadableVolumes(volumes))
}

func getMount(mountName string, mounts []v1.VolumeMount) (*v1.VolumeMount, error) {
	for _, mount := range mounts {
		if mount.Name == mountName {
			return &mount, nil
		}
	}
	return &v1.VolumeMount{Name: mntNotFound},
		fmt.Errorf("volume mount %s does not exist in %s", mountName, HumanReadableVolumeMounts(mounts))
}
