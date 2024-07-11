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

package csi

import (
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"gopkg.in/yaml.v2"
	corev1 "k8s.io/api/core/v1"
)

func TestDaemonSetTemplate(t *testing.T) {
	tp := templateParam{
		Param:     CSIParam,
		Namespace: "foo",
	}
	ds, err := templateToDaemonSet("test-ds", RBDPluginTemplatePath, tp)
	assert.Nil(t, err)
	assert.Equal(t, "driver-registrar", ds.Spec.Template.Spec.Containers[0].Name)
}

func TestDeploymentTemplate(t *testing.T) {
	tp := templateParam{
		Param:     CSIParam,
		Namespace: "foo",
	}
	_, err := templateToDeployment("test-dep", RBDProvisionerDepTemplatePath, tp)
	assert.Nil(t, err)
}

func TestGetPortFromConfig(t *testing.T) {
	key := "TEST_CSI_PORT_ENV"
	var defaultPort uint16 = 8000
	data := map[string]string{}

	// empty env variable
	port, err := getPortFromConfig(data, key, defaultPort)
	assert.Nil(t, err)
	assert.Equal(t, port, defaultPort)

	// valid port is set in env
	t.Setenv(key, "9000")
	port, err = getPortFromConfig(data, key, defaultPort)
	assert.Nil(t, err)
	assert.Equal(t, port, uint16(9000))

	// higher port value is set in env
	t.Setenv(key, "65536")
	port, err = getPortFromConfig(data, key, defaultPort)
	assert.Error(t, err)
	assert.Equal(t, port, defaultPort)

	// negative port is set in env
	t.Setenv(key, "-1")
	port, err = getPortFromConfig(data, key, defaultPort)
	assert.Error(t, err)
	assert.Equal(t, port, defaultPort)

	err = os.Unsetenv(key)
	assert.Nil(t, err)
}

func TestApplyingResourcesToRBDPlugin(t *testing.T) {
	tp := templateParam{}
	rbdPlugin, err := templateToDaemonSet("rbdplugin", RBDPluginTemplatePath, tp)
	assert.Nil(t, err)
	params := make(map[string]string)

	// need to build using map[string]interface{} because the following resource
	// doesn't serialise nicely
	// https://pkg.go.dev/k8s.io/apimachinery/pkg/api/resource#Quantity
	resource := []map[string]interface{}{
		{
			"name": "driver-registrar",
			"resource": map[string]interface{}{
				"limits": map[string]interface{}{
					"cpu":    "200m",
					"memory": "256Mi",
				},
				"requests": map[string]interface{}{
					"cpu":    "100m",
					"memory": "128Mi",
				},
			},
		},
	}

	resourceRaw, err := yaml.Marshal(resource)
	assert.Nil(t, err)
	params[rbdPluginResource] = string(resourceRaw)
	applyResourcesToContainers(params, rbdPluginResource, &rbdPlugin.Spec.Template.Spec)
	assert.Equal(t, rbdPlugin.Spec.Template.Spec.Containers[0].Resources.Requests.Memory().String(), "128Mi")
	assert.Equal(t, rbdPlugin.Spec.Template.Spec.Containers[0].Resources.Limits.Memory().String(), "256Mi")
	assert.Equal(t, rbdPlugin.Spec.Template.Spec.Containers[0].Resources.Requests.Cpu().String(), "100m")
	assert.Equal(t, rbdPlugin.Spec.Template.Spec.Containers[0].Resources.Limits.Cpu().String(), "200m")
}

func Test_applyVolumeToPodSpec(t *testing.T) {
	// when no volumes specified
	config := make(map[string]string)
	configKey := "TEST_CSI_PLUGIN_VOLUME"
	dsName := "test-ds"
	tp := templateParam{
		Param:     CSIParam,
		Namespace: "foo",
	}
	// rbdplugin has 11 volumes by default
	defaultVolumes := 11
	ds, err := templateToDaemonSet(dsName, RBDPluginTemplatePath, tp)
	assert.Nil(t, err)
	applyVolumeToPodSpec(config, configKey, &ds.Spec.Template.Spec)

	assert.Len(t, ds.Spec.Template.Spec.Volumes, defaultVolumes)

	// enable csi logrotate, two more volume mounts get added
	tp.CSILogRotation = true
	ds, err = templateToDaemonSet(dsName, RBDPluginTemplatePath, tp)
	assert.Nil(t, err)
	applyVolumeToPodSpec(config, configKey, &ds.Spec.Template.Spec)
	assert.Len(t, ds.Spec.Template.Spec.Volumes, defaultVolumes+2)
	tp.CSILogRotation = false

	// add new volume
	volumes := []corev1.Volume{
		{
			Name:         "test-host-dev",
			VolumeSource: corev1.VolumeSource{HostPath: &corev1.HostPathVolumeSource{Path: "/dev"}},
		},
	}
	volumeRaw, err := yaml.Marshal(volumes)
	assert.Nil(t, err)
	config[configKey] = string(volumeRaw)
	ds, err = templateToDaemonSet(dsName, RBDPluginTemplatePath, tp)
	assert.Nil(t, err)
	applyVolumeToPodSpec(config, configKey, &ds.Spec.Template.Spec)

	assert.Len(t, ds.Spec.Template.Spec.Volumes, defaultVolumes+1)
	// add one more volume
	volumes = append(volumes, corev1.Volume{
		Name:         "test-host-run",
		VolumeSource: corev1.VolumeSource{HostPath: &corev1.HostPathVolumeSource{Path: "/run"}},
	})
	volumeRaw, err = yaml.Marshal(volumes)
	assert.Nil(t, err)
	config[configKey] = string(volumeRaw)
	ds, err = templateToDaemonSet(dsName, RBDPluginTemplatePath, tp)
	assert.Nil(t, err)
	applyVolumeToPodSpec(config, configKey, &ds.Spec.Template.Spec)
	assert.Len(t, ds.Spec.Template.Spec.Volumes, defaultVolumes+2)
	// override existing volume configuration
	volumes[1].VolumeSource = corev1.VolumeSource{
		HostPath: &corev1.HostPathVolumeSource{Path: "/run/test/run"},
	}
	volumeRaw, err = yaml.Marshal(volumes)
	assert.Nil(t, err)
	config[configKey] = string(volumeRaw)
	ds, err = templateToDaemonSet(dsName, RBDPluginTemplatePath, tp)
	assert.Nil(t, err)
	applyVolumeToPodSpec(config, configKey, &ds.Spec.Template.Spec)
	assert.Len(t, ds.Spec.Template.Spec.Volumes, defaultVolumes+2)
	// remove existing volume configuration
	volumes = []corev1.Volume{
		{
			Name:         "test-host-dev",
			VolumeSource: corev1.VolumeSource{HostPath: &corev1.HostPathVolumeSource{Path: "/dev"}},
		},
	}
	volumeRaw, err = yaml.Marshal(volumes)
	assert.Nil(t, err)
	config[configKey] = string(volumeRaw)
	ds, err = templateToDaemonSet(dsName, RBDPluginTemplatePath, tp)
	assert.Nil(t, err)
	applyVolumeToPodSpec(config, configKey, &ds.Spec.Template.Spec)
	assert.Len(t, ds.Spec.Template.Spec.Volumes, defaultVolumes+1)
}

func Test_applyVolumeMountToContainer(t *testing.T) {
	// when no volumes specified
	config := make(map[string]string)
	configKey := "TEST_CSI_PLUGIN_VOLUME_MOUNT"
	dsName := "test-ds"
	rbdContainerName := "csi-rbdplugin"
	tp := templateParam{
		Param:     CSIParam,
		Namespace: "foo",
	}
	// csi-rbdplugin has 10 volumes by default
	defaultVolumes := 10
	ds, err := templateToDaemonSet(dsName, RBDPluginTemplatePath, tp)
	assert.Nil(t, err)
	applyVolumeMountToContainer(config, configKey, rbdContainerName, &ds.Spec.Template.Spec)

	assert.Len(t, ds.Spec.Template.Spec.Containers[1].VolumeMounts, defaultVolumes)

	// enable csi logrotate, one more volumes get added
	tp.CSILogRotation = true
	ds, err = templateToDaemonSet(dsName, RBDPluginTemplatePath, tp)
	assert.Nil(t, err)
	applyVolumeMountToContainer(config, configKey, rbdContainerName, &ds.Spec.Template.Spec)
	assert.Len(t, ds.Spec.Template.Spec.Containers[1].VolumeMounts, defaultVolumes+1)
	tp.CSILogRotation = false

	// add new volume mount
	volumeMounts := []corev1.VolumeMount{
		{
			Name:      "test-host-dev",
			MountPath: "/dev/test",
		},
	}
	volumeMountsRaw, err := yaml.Marshal(volumeMounts)
	assert.Nil(t, err)
	config[configKey] = string(volumeMountsRaw)
	ds, err = templateToDaemonSet(dsName, RBDPluginTemplatePath, tp)
	assert.Nil(t, err)
	applyVolumeMountToContainer(config, configKey, rbdContainerName, &ds.Spec.Template.Spec)

	assert.Len(t, ds.Spec.Template.Spec.Containers[1].VolumeMounts, defaultVolumes+1)
	// add one more volumemount
	volumeMounts = append(volumeMounts, corev1.VolumeMount{
		Name:      "test-host-run",
		MountPath: "/run/test",
	})
	volumeMountsRaw, err = yaml.Marshal(volumeMounts)
	assert.Nil(t, err)
	config[configKey] = string(volumeMountsRaw)
	ds, err = templateToDaemonSet(dsName, RBDPluginTemplatePath, tp)
	assert.Nil(t, err)
	applyVolumeMountToContainer(config, configKey, rbdContainerName, &ds.Spec.Template.Spec)
	assert.Len(t, ds.Spec.Template.Spec.Containers[1].VolumeMounts, defaultVolumes+2)
	// override existing volume configuration
	volumeMounts[1].MountPath = "/run/test/run"
	volumeMountsRaw, err = yaml.Marshal(volumeMounts)
	assert.Nil(t, err)
	config[configKey] = string(volumeMountsRaw)
	ds, err = templateToDaemonSet(dsName, RBDPluginTemplatePath, tp)
	assert.Nil(t, err)
	applyVolumeMountToContainer(config, configKey, rbdContainerName, &ds.Spec.Template.Spec)
	assert.Len(t, ds.Spec.Template.Spec.Containers[1].VolumeMounts, defaultVolumes+2)
	// remove existing volume configuration
	volumeMounts = []corev1.VolumeMount{
		{
			Name:      "test-host-dev",
			MountPath: "/dev/test",
		},
	}
	volumeMountsRaw, err = yaml.Marshal(volumeMounts)
	assert.Nil(t, err)
	config[configKey] = string(volumeMountsRaw)
	ds, err = templateToDaemonSet(dsName, RBDPluginTemplatePath, tp)
	assert.Nil(t, err)
	applyVolumeMountToContainer(config, configKey, rbdContainerName, &ds.Spec.Template.Spec)
	assert.Len(t, ds.Spec.Template.Spec.Containers[1].VolumeMounts, defaultVolumes+1)
}

func Test_getImage(t *testing.T) {
	type args struct {
		data         map[string]string
		settingName  string
		defaultImage string
	}
	tests := []struct {
		name string
		args args
		want string
	}{
		{
			name: "test with default image",
			args: args{
				data:         map[string]string{},
				settingName:  "ROOK_CSI_CEPH_IMAGE",
				defaultImage: "quay.io/cephcsi/cephcsi:v3.11.0",
			},
			want: DefaultCSIPluginImage,
		},
		{
			name: "test with user image",
			args: args{
				data: map[string]string{
					"ROOK_CSI_CEPH_IMAGE": "registry.io/private/cephcsi:v8",
				},
				settingName:  "ROOK_CSI_CEPH_IMAGE",
				defaultImage: "quay.io/cephcsi/cephcsi:v3.11.0",
			},
			want: "registry.io/private/cephcsi:v8",
		},
		{
			name: "test with user image without version",
			args: args{
				data: map[string]string{
					"ROOK_CSI_CEPH_IMAGE": "registry.io/private/cephcsi",
				},
				settingName:  "ROOK_CSI_CEPH_IMAGE",
				defaultImage: "quay.io/cephcsi/cephcsi:v3.11.0",
			},
			want: "registry.io/private/cephcsi:v3.11.0",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := getImage(tt.args.data, tt.args.settingName, tt.args.defaultImage)
			assert.Equal(t, tt.want, got)
		})
	}
}
