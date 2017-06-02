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
package mon

import (
	"fmt"
	"strings"
	"testing"

	"github.com/rook/rook/pkg/clusterd"
	"github.com/rook/rook/pkg/operator/k8sutil"
	testop "github.com/rook/rook/pkg/operator/test"

	"github.com/stretchr/testify/assert"
	"k8s.io/client-go/pkg/api/v1"
)

func TestPodSpecs(t *testing.T) {
	testPodSpec(t, "")
	testPodSpec(t, "/var/lib/mydatadir")
}

func testPodSpec(t *testing.T, dataDir string) {
	clientset := testop.New(1)
	c := New(&clusterd.Context{KubeContext: clusterd.KubeContext{Clientset: clientset}}, "myname", "ns", dataDir, "myversion", k8sutil.Placement{})
	c.clusterInfo = testop.CreateClusterInfo(0)
	config := &MonConfig{Name: "mon0", Port: 6790}

	pod := c.makeMonPod(config, "foo")
	assert.NotNil(t, pod)
	assert.Equal(t, "mon0", pod.Name)
	assert.Equal(t, v1.RestartPolicyAlways, pod.Spec.RestartPolicy)
	assert.Equal(t, 2, len(pod.Spec.Volumes))
	assert.Equal(t, "rook-data", pod.Spec.Volumes[0].Name)
	assert.Equal(t, k8sutil.ConfigOverrideName, pod.Spec.Volumes[1].Name)
	if dataDir == "" {
		assert.NotNil(t, pod.Spec.Volumes[0].EmptyDir)
		assert.Nil(t, pod.Spec.Volumes[0].HostPath)
	} else {
		assert.Nil(t, pod.Spec.Volumes[0].EmptyDir)
		assert.Equal(t, dataDir, pod.Spec.Volumes[0].HostPath.Path)
	}

	assert.Equal(t, "mon0", pod.ObjectMeta.Name)
	assert.Equal(t, "mon", pod.ObjectMeta.Labels["app"])
	assert.Equal(t, c.Namespace, pod.ObjectMeta.Labels["mon_cluster"])
	assert.Equal(t, 1, len(pod.ObjectMeta.Annotations))
	assert.Equal(t, "myversion", pod.ObjectMeta.Annotations["rook_version"])

	cont := pod.Spec.Containers[0]
	assert.Equal(t, "quay.io/rook/rookd:myversion", cont.Image)
	assert.Equal(t, 2, len(cont.VolumeMounts))
	assert.Equal(t, 6, len(cont.Env))

	expectedCommand := fmt.Sprintf("/usr/local/bin/rookd mon --config-dir=/var/lib/rook --name=%s --port=%d --fsid=%s",
		config.Name, config.Port, c.clusterInfo.FSID)

	assert.NotEqual(t, -1, strings.Index(cont.Command[2], expectedCommand), cont.Command[2])
}
