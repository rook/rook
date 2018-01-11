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
package mgr

import (
	"fmt"
	"io/ioutil"
	"os"
	"testing"

	rookalpha "github.com/rook/rook/pkg/apis/rook.io/v1alpha1"
	"github.com/rook/rook/pkg/clusterd"
	testop "github.com/rook/rook/pkg/operator/test"
	exectest "github.com/rook/rook/pkg/util/exec/test"
	"github.com/stretchr/testify/assert"
	"k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestStartMGR(t *testing.T) {
	executor := &exectest.MockExecutor{
		MockExecuteCommandWithOutputFile: func(debug bool, actionName string, command string, outFileArg string, args ...string) (string, error) {
			return "{\"key\":\"mysecurekey\"}", nil
		},
	}

	configDir, _ := ioutil.TempDir("", "")
	defer os.RemoveAll(configDir)
	context := &clusterd.Context{
		Executor:  executor,
		ConfigDir: configDir,
		Clientset: testop.New(3)}
	c := New(context, "ns", "myversion", rookalpha.Placement{}, false, v1.ResourceRequirements{}, metav1.OwnerReference{})
	defer os.RemoveAll(c.dataDir)

	// start a basic service
	err := c.Start()
	assert.Nil(t, err)
	validateStart(t, c)

	// starting again with more replicas
	c.Replicas = 3
	err = c.Start()
	assert.Nil(t, err)
	validateStart(t, c)
}

func validateStart(t *testing.T, c *Cluster) {

	for i := 0; i < c.Replicas; i++ {
		logger.Infof("Looking for cephmgr replica %d", i)
		_, err := c.context.Clientset.ExtensionsV1beta1().Deployments(c.Namespace).Get(fmt.Sprintf("rook-ceph-mgr%d", i), metav1.GetOptions{})
		assert.Nil(t, err)
	}
}

func TestPodSpec(t *testing.T) {
	c := New(nil, "ns", "rook/rook:myversion", rookalpha.Placement{}, false, v1.ResourceRequirements{
		Limits: v1.ResourceList{
			v1.ResourceCPU: *resource.NewQuantity(100.0, resource.BinarySI),
		},
		Requests: v1.ResourceList{
			v1.ResourceMemory: *resource.NewQuantity(1337.0, resource.BinarySI),
		},
	}, metav1.OwnerReference{})

	d := c.makeDeployment("mgr1")
	assert.NotNil(t, d)
	assert.Equal(t, "mgr1", d.Name)
	assert.Equal(t, v1.RestartPolicyAlways, d.Spec.Template.Spec.RestartPolicy)
	assert.Equal(t, 2, len(d.Spec.Template.Spec.Volumes))
	assert.Equal(t, 2, len(d.Spec.Template.Spec.Containers[0].Ports))
	assert.Equal(t, "rook-data", d.Spec.Template.Spec.Volumes[0].Name)

	assert.Equal(t, "mgr1", d.ObjectMeta.Name)
	assert.Equal(t, appName, d.Spec.Template.ObjectMeta.Labels["app"])
	assert.Equal(t, c.Namespace, d.Spec.Template.ObjectMeta.Labels["rook_cluster"])
	assert.Equal(t, 0, len(d.ObjectMeta.Annotations))

	cont := d.Spec.Template.Spec.Containers[0]
	assert.Equal(t, "rook/rook:myversion", cont.Image)
	assert.Equal(t, 2, len(cont.VolumeMounts))

	assert.Equal(t, "mgr", cont.Args[0])
	assert.Equal(t, "--config-dir=/var/lib/rook", cont.Args[1])

	assert.Equal(t, "100", cont.Resources.Limits.Cpu().String())
	assert.Equal(t, "1337", cont.Resources.Requests.Memory().String())
}

func TestServiceSpec(t *testing.T) {
	c := New(nil, "ns", "myversion", rookalpha.Placement{}, false, v1.ResourceRequirements{}, metav1.OwnerReference{})

	s := c.makeService("rook-mgr")
	assert.NotNil(t, s)
	assert.Equal(t, "rook-mgr", s.Name)
	assert.Equal(t, 1, len(s.Spec.Ports))
}

func TestHostNetwork(t *testing.T) {
	c := New(nil, "ns", "myversion", rookalpha.Placement{}, true, v1.ResourceRequirements{}, metav1.OwnerReference{})

	d := c.makeDeployment("mgr1")
	assert.NotNil(t, d)

	assert.Equal(t, true, d.Spec.Template.Spec.HostNetwork)
	assert.Equal(t, v1.DNSClusterFirstWithHostNet, d.Spec.Template.Spec.DNSPolicy)
}
