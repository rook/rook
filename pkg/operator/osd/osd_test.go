/*
Copyright 2016 The Rook Authors. All rights reserved.

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
package osd

import (
	"strings"
	"testing"

	"k8s.io/client-go/kubernetes/fake"
	"k8s.io/client-go/pkg/api/v1"

	"github.com/stretchr/testify/assert"
)

func TestStartDaemonset(t *testing.T) {
	clientset := fake.NewSimpleClientset()
	c := New(clientset, "ns", "myversion", "", "", false)

	// Start the first time
	err := c.Start()
	assert.Nil(t, err)

	// Should not fail if it already exists
	err = c.Start()
	assert.Nil(t, err)
}

func TestDaemonset(t *testing.T) {
	testPodDevices(t, "", "sda", true)
	testPodDevices(t, "/var/lib/mydatadir", "sdb", false)
	testPodDevices(t, "", "", true)
	testPodDevices(t, "", "", false)
}

func testPodDevices(t *testing.T, dataDir, deviceFilter string, allDevices bool) {
	clientset := fake.NewSimpleClientset()
	c := New(clientset, "ns", "myversion", deviceFilter, dataDir, allDevices)

	daemonSet, err := c.makeDaemonSet()
	assert.Nil(t, err)
	assert.NotNil(t, daemonSet)
	assert.Equal(t, "osd", daemonSet.Name)
	assert.Equal(t, c.Namespace, daemonSet.Namespace)
	assert.Equal(t, v1.RestartPolicyAlways, daemonSet.Spec.Template.Spec.RestartPolicy)
	assert.Equal(t, 2, len(daemonSet.Spec.Template.Spec.Volumes))
	assert.Equal(t, "rook-data", daemonSet.Spec.Template.Spec.Volumes[0].Name)
	assert.Equal(t, "devices", daemonSet.Spec.Template.Spec.Volumes[1].Name)
	if dataDir == "" {
		assert.NotNil(t, daemonSet.Spec.Template.Spec.Volumes[0].EmptyDir)
		assert.Nil(t, daemonSet.Spec.Template.Spec.Volumes[0].HostPath)
	} else {
		assert.Nil(t, daemonSet.Spec.Template.Spec.Volumes[0].EmptyDir)
		assert.Equal(t, dataDir, daemonSet.Spec.Template.Spec.Volumes[0].HostPath.Path)
	}

	assert.Equal(t, "osd", daemonSet.Spec.Template.ObjectMeta.Name)
	assert.Equal(t, "osd", daemonSet.Spec.Template.ObjectMeta.Labels["app"])
	assert.Equal(t, c.Namespace, daemonSet.Spec.Template.ObjectMeta.Labels["rook_cluster"])
	assert.Equal(t, 0, len(daemonSet.Spec.Template.ObjectMeta.Annotations))

	cont := daemonSet.Spec.Template.Spec.Containers[0]
	assert.Equal(t, "quay.io/rook/rookd:myversion", cont.Image)
	assert.Equal(t, 2, len(cont.VolumeMounts))
	assert.Equal(t, 5, len(cont.Env))

	expectedCommand := "/usr/bin/rookd osd --data-dir=/var/lib/rook "
	assert.NotEqual(t, -1, strings.Index(cont.Command[2], expectedCommand), cont.Command[2])
	devices := cont.Env[0].Value
	if deviceFilter != "" {
		assert.Equal(t, deviceFilter, devices)
	} else if allDevices {
		assert.Equal(t, "all", devices)
	} else {
		assert.Equal(t, "", devices)
	}
}
