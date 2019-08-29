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
package mgr

import (
	"io/ioutil"
	"os"
	"strconv"
	"testing"

	edgefsv1 "github.com/rook/rook/pkg/apis/edgefs.rook.io/v1"
	rookalpha "github.com/rook/rook/pkg/apis/rook.io/v1alpha2"
	"github.com/rook/rook/pkg/clusterd"
	testop "github.com/rook/rook/pkg/operator/test"
	exectest "github.com/rook/rook/pkg/util/exec/test"
	"github.com/stretchr/testify/assert"
	v1 "k8s.io/api/core/v1"
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
	volSize := resource.NewQuantity(100000.0, resource.BinarySI)
	c := New(context, "ns", "myversion", "", "", *volSize, rookalpha.Annotations{}, rookalpha.Placement{}, rookalpha.NetworkSpec{},
		edgefsv1.DashboardSpec{}, v1.ResourceRequirements{}, "", metav1.OwnerReference{}, false)

	// start a basic service
	err := c.Start("edgefs")
	assert.Nil(t, err)
	validateStart(t, c)
}

func validateStart(t *testing.T, c *Cluster) {

	_, err := c.context.Clientset.AppsV1().Deployments(c.Namespace).Get("rook-edgefs-mgr", metav1.GetOptions{})
	assert.Nil(t, err)

	_, err = c.context.Clientset.CoreV1().Services(c.Namespace).Get("rook-edgefs-mgr", metav1.GetOptions{})
	assert.Nil(t, err)
}

func TestPodSpec(t *testing.T) {
	volSize := resource.NewQuantity(100000.0, resource.BinarySI)
	c := New(&clusterd.Context{Clientset: testop.New(1)}, "ns", "rook/rook:myversion", "", "", *volSize, rookalpha.Annotations{}, rookalpha.Placement{},
		rookalpha.NetworkSpec{}, edgefsv1.DashboardSpec{}, v1.ResourceRequirements{
			Limits: v1.ResourceList{
				v1.ResourceCPU: *resource.NewQuantity(100.0, resource.BinarySI),
			},
			Requests: v1.ResourceList{
				v1.ResourceMemory: *resource.NewQuantity(1337.0, resource.BinarySI),
			},
		}, "", metav1.OwnerReference{}, false)

	d := c.makeDeployment("mgr-a", "rook-edgefs", "edgefs", 1)
	assert.NotNil(t, d)
	assert.Equal(t, "mgr-a", d.Name)
	assert.Equal(t, v1.RestartPolicyAlways, d.Spec.Template.Spec.RestartPolicy)
	assert.Equal(t, 1, len(d.Spec.Template.Spec.Volumes))

	//check ports
	assert.Equal(t, 3, len(d.Spec.Template.Spec.Containers[0].Ports))
	assert.Equal(t, int32(8881), d.Spec.Template.Spec.Containers[0].Ports[0].ContainerPort)
	assert.Equal(t, int32(8080), d.Spec.Template.Spec.Containers[0].Ports[1].ContainerPort)
	assert.Equal(t, int32(4443), d.Spec.Template.Spec.Containers[0].Ports[2].ContainerPort)

	assert.Equal(t, "edgefs-datadir", d.Spec.Template.Spec.Volumes[0].Name)
	assert.Equal(t, "mgr-a", d.ObjectMeta.Name)
	assert.Equal(t, appName, d.Spec.Template.ObjectMeta.Labels["app"])
	assert.Equal(t, c.Namespace, d.Spec.Template.ObjectMeta.Labels["rook_cluster"])
	assert.Equal(t, 0, len(d.ObjectMeta.Annotations))

	assert.Equal(t, 2, len(d.Spec.Template.ObjectMeta.Annotations))
	assert.Equal(t, "true", d.Spec.Template.ObjectMeta.Annotations["prometheus.io/scrape"])
	assert.Equal(t, strconv.Itoa(defaultMetricsPort), d.Spec.Template.ObjectMeta.Annotations["prometheus.io/port"])

	cont := d.Spec.Template.Spec.Containers[0]
	assert.Equal(t, 2, len(cont.VolumeMounts))
	assert.Equal(t, "mgmt", cont.Args[0])

	assert.Equal(t, "100", cont.Resources.Limits.Cpu().String())
	assert.Equal(t, "1337", cont.Resources.Requests.Memory().String())
}

func TestServiceSpec(t *testing.T) {
	volSize := resource.NewQuantity(100000.0, resource.BinarySI)
	c := New(&clusterd.Context{}, "ns", "myversion", "", "", *volSize, rookalpha.Annotations{}, rookalpha.Placement{},
		rookalpha.NetworkSpec{}, edgefsv1.DashboardSpec{}, v1.ResourceRequirements{},
		"", metav1.OwnerReference{}, false)

	s := c.makeMgrService("rook-edgefs-mgr")
	assert.NotNil(t, s)
	assert.Equal(t, "rook-edgefs-mgr", s.Name)
	assert.Equal(t, 1, len(s.Spec.Ports))
}

func TestHostNetwork(t *testing.T) {
	volSize := resource.NewQuantity(100000.0, resource.BinarySI)
	net := rookalpha.NetworkSpec{
		Provider: "host",
		Selectors: map[string]string{
			"server": "eth0",
		},
	}
	c := New(&clusterd.Context{Clientset: testop.New(1)}, "ns", "myversion", "", "", *volSize, rookalpha.Annotations{}, rookalpha.Placement{},
		net, edgefsv1.DashboardSpec{}, v1.ResourceRequirements{},
		"", metav1.OwnerReference{}, false)

	d := c.makeDeployment("mgr-a", "a", "edgefs", 1)
	assert.NotNil(t, d)

	assert.Equal(t, true, d.Spec.Template.Spec.HostNetwork)
	assert.Equal(t, v1.DNSClusterFirstWithHostNet, d.Spec.Template.Spec.DNSPolicy)
}
