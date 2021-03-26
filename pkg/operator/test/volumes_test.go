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

func TestVolumeExists(t *testing.T) {
	vols := []v1.Volume{{Name: "a"}, {Name: "b"}, {Name: "d"}}
	type args struct {
		volumeName string
		volumes    []v1.Volume
	}
	tests := []struct {
		name    string
		args    args
		wantErr bool
	}{
		{"exists", args{"d", vols}, false},
		{"does-not-exist", args{"c", vols}, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := VolumeExists(tt.args.volumeName, tt.args.volumes); (err != nil) != tt.wantErr {
				t.Errorf("VolumeExists() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func vols(v ...v1.Volume) []v1.Volume {
	return v
}
func TestVolumeIsEmptyDir(t *testing.T) {
	emptyVolume := v1.Volume{Name: "e", VolumeSource: v1.VolumeSource{EmptyDir: &v1.EmptyDirVolumeSource{}}}
	emptyAndHostVolume := v1.Volume{Name: "e&hp", VolumeSource: v1.VolumeSource{EmptyDir: &v1.EmptyDirVolumeSource{}}}
	emptyAndHostVolume.VolumeSource.HostPath = &v1.HostPathVolumeSource{Path: "/dev/sdx"}
	hostVolume := v1.Volume{Name: "h", VolumeSource: v1.VolumeSource{HostPath: &v1.HostPathVolumeSource{Path: "/dev/vdh"}}}
	type args struct {
		volumeName string
		volumes    []v1.Volume
	}
	tests := []struct {
		name    string
		args    args
		wantErr bool
	}{
		{"is EmptyDir", args{"e", vols(emptyVolume)}, false},
		{"is HostPath", args{"h", vols(hostVolume)}, true},
		{"EmptyDir and HostPath", args{"e&hp", vols(emptyAndHostVolume)}, true},
		{"not found", args{"e", vols(hostVolume)}, true},
		{"many ; ok", args{"e", vols(emptyVolume, hostVolume, emptyAndHostVolume)}, false},
		{"many ; nf", args{"e", vols(hostVolume, emptyAndHostVolume)}, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := VolumeIsEmptyDir(tt.args.volumeName, tt.args.volumes); (err != nil) != tt.wantErr {
				t.Errorf("VolumeIsEmptyDir() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestVolumeIsHostPath(t *testing.T) {
	emptyVolume := v1.Volume{Name: "e", VolumeSource: v1.VolumeSource{EmptyDir: &v1.EmptyDirVolumeSource{}}}
	emptyAndHostVolume := v1.Volume{Name: "e&hp", VolumeSource: v1.VolumeSource{EmptyDir: &v1.EmptyDirVolumeSource{}}}
	emptyAndHostVolume.VolumeSource.HostPath = &v1.HostPathVolumeSource{Path: "/dev/sdx"}
	hostVolume := v1.Volume{Name: "h", VolumeSource: v1.VolumeSource{HostPath: &v1.HostPathVolumeSource{Path: "/dev/vdh"}}}
	type args struct {
		volumeName string
		path       string
		volumes    []v1.Volume
	}
	tests := []struct {
		name    string
		args    args
		wantErr bool
	}{
		{"is EmptyDir", args{"e", "/dev/sdx", vols(emptyVolume)}, true},
		{"is HostPath", args{"h", "/dev/vdh", vols(hostVolume)}, false},
		{"wrong HostPath", args{"h", "/dev/sdx", vols(hostVolume)}, true},
		{"EmptyDir and HostPath", args{"e&hp", "/dev/sdx", vols(emptyAndHostVolume)}, true},
		{"not found", args{"e", "/dev/sdx", vols(hostVolume)}, true},
		{"many ; ok", args{"h", "/dev/vdh", vols(emptyVolume, hostVolume, emptyAndHostVolume)}, false},
		{"many ; nf", args{"h", "/dev/vdh", vols(emptyVolume, emptyAndHostVolume)}, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := VolumeIsHostPath(tt.args.volumeName, tt.args.path, tt.args.volumes); (err != nil) != tt.wantErr {
				t.Errorf("VolumeIsHostPath() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestVolumeMountExists(t *testing.T) {
	mounts := []v1.VolumeMount{{Name: "a"}, {Name: "b"}, {Name: "d"}}
	type args struct {
		mountName string
		mounts    []v1.VolumeMount
	}
	tests := []struct {
		name    string
		args    args
		wantErr bool
	}{
		{"exists", args{"d", mounts}, false},
		{"does-not-exist", args{"c", mounts}, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := VolumeMountExists(tt.args.mountName, tt.args.mounts); (err != nil) != tt.wantErr {
				t.Errorf("VolumeMountExists() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

// Don't test human readables since they aren't critical to function
