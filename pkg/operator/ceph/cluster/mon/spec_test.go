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
	"testing"

	cephv1beta1 "github.com/rook/rook/pkg/apis/ceph.rook.io/v1beta1"
	rookalpha "github.com/rook/rook/pkg/apis/rook.io/v1alpha2"
	"github.com/rook/rook/pkg/clusterd"
	"github.com/rook/rook/pkg/operator/k8sutil"
	testop "github.com/rook/rook/pkg/operator/test"
	"github.com/stretchr/testify/assert"
	"k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestPodSpecs(t *testing.T) {
	testPodSpec(t, "")
	testPodSpec(t, "/var/lib/mydatadir")
}

func testPodSpec(t *testing.T, dataDir string) {
	clientset := testop.New(1)
	c := New(&clusterd.Context{Clientset: clientset}, "ns", dataDir, "rook/rook:myversion",
		cephv1beta1.MonSpec{Count: 3, AllowMultiplePerNode: true}, rookalpha.Placement{}, false,
		v1.ResourceRequirements{
			Limits: v1.ResourceList{
				v1.ResourceCPU: *resource.NewQuantity(100.0, resource.BinarySI),
			},
			Requests: v1.ResourceList{
				v1.ResourceMemory: *resource.NewQuantity(1337.0, resource.BinarySI),
			},
		}, metav1.OwnerReference{})
	c.clusterInfo = testop.CreateConfigDir(0)
	config := &monConfig{Name: "rook-ceph-mon0", Port: 6790}

	pod := c.makeMonPod(config, "foo")
	assert.NotNil(t, pod)
	assert.Equal(t, "rook-ceph-mon0", pod.Name)
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

	assert.Equal(t, "rook-ceph-mon0", pod.ObjectMeta.Name)
	assert.Equal(t, appName, pod.ObjectMeta.Labels["app"])
	assert.Equal(t, c.Namespace, pod.ObjectMeta.Labels["mon_cluster"])

	cont := pod.Spec.Containers[0]
	assert.Equal(t, "rook/rook:myversion", cont.Image)
	assert.Equal(t, 2, len(cont.VolumeMounts))
	assert.Equal(t, 7, len(cont.Env))
	assert.False(t, *cont.SecurityContext.Privileged)

	logger.Infof("Command : %+v", cont.Command)
	assert.Equal(t, "ceph", cont.Args[0])
	assert.Equal(t, "mon", cont.Args[1])
	assert.Equal(t, "--config-dir=/var/lib/rook", cont.Args[2])
	assert.Equal(t, "--name=rook-ceph-mon0", cont.Args[3])
	assert.Equal(t, "--port=6790", cont.Args[4])
	assert.Equal(t, fmt.Sprintf("--fsid=%s", c.clusterInfo.FSID), cont.Args[5])

	assert.Equal(t, "100", cont.Resources.Limits.Cpu().String())
	assert.Equal(t, "1337", cont.Resources.Requests.Memory().String())
}
