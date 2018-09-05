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

	cephv1beta1 "github.com/rook/rook/pkg/apis/ceph.rook.io/v1beta1"
	rookalpha "github.com/rook/rook/pkg/apis/rook.io/v1alpha2"
	"github.com/rook/rook/pkg/clusterd"
	cephconfig "github.com/rook/rook/pkg/daemon/ceph/config"
	mondaemon "github.com/rook/rook/pkg/daemon/ceph/mon"
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

func testCephMonCommonArgs(t *testing.T, c *Cluster, name string, cont v1.Container, startIndex int) {
	assert.Equal(t, "--name", cont.Args[startIndex])
	assert.Equal(t, fmt.Sprintf("mon.%s", name), cont.Args[startIndex+1])
	assert.Equal(t, "--cluster", cont.Args[startIndex+2])
	assert.Equal(t, c.clusterInfo.Name, cont.Args[startIndex+3])
	assert.Equal(t, "--mon-data", cont.Args[startIndex+4])
	assert.Equal(t, mondaemon.GetMonDataDirPath(c.context.ConfigDir, name), cont.Args[startIndex+5])
	assert.Equal(t, "--conf", cont.Args[startIndex+6])
	assert.Equal(t, cephconfig.DefaultConfigFilePath(), cont.Args[startIndex+7])
	assert.Equal(t, "--keyring", cont.Args[startIndex+8])
	assert.Equal(t, cephconfig.DefaultKeyringFilePath(), cont.Args[startIndex+9])
}

func testPodSpec(t *testing.T, dataDir string) {
	clientset := testop.New(1)
	c := New(
		&clusterd.Context{Clientset: clientset, ConfigDir: "/var/lib/rook"},
		"ns",
		dataDir,
		"rook/rook:myversion",
		cephv1beta1.MonSpec{Count: 3, AllowMultiplePerNode: true},
		rookalpha.Placement{},
		false,
		v1.ResourceRequirements{
			Limits: v1.ResourceList{
				v1.ResourceCPU: *resource.NewQuantity(100.0, resource.BinarySI),
			},
			Requests: v1.ResourceList{
				v1.ResourceMemory: *resource.NewQuantity(1337.0, resource.BinarySI),
			},
		},
		metav1.OwnerReference{},
	)
	c.clusterInfo = testop.CreateConfigDir(0)
	name := "a"
	config := &monConfig{ResourceName: name, DaemonName: name, Port: 6790, PublicIP: "2.4.6.1"}

	pod := c.makeMonPod(config, "foo")
	assert.NotNil(t, pod)
	assert.Equal(t, "a", pod.Name)
	assert.Equal(t, v1.RestartPolicyAlways, pod.Spec.RestartPolicy)
	assert.Equal(t, 3, len(pod.Spec.Volumes))
	assert.Equal(t, "rook-data", pod.Spec.Volumes[0].Name)
	assert.Equal(t, k8sutil.ConfigOverrideName, pod.Spec.Volumes[1].Name)
	assert.Equal(t, cephconfig.DefaultConfigVolume(), pod.Spec.Volumes[2])
	if dataDir == "" {
		assert.NotNil(t, pod.Spec.Volumes[0].EmptyDir)
		assert.Nil(t, pod.Spec.Volumes[0].HostPath)
	} else {
		assert.Nil(t, pod.Spec.Volumes[0].EmptyDir)
		assert.Equal(t, dataDir, pod.Spec.Volumes[0].HostPath.Path)
	}

	assert.Equal(t, "a", pod.ObjectMeta.Name)
	assert.Equal(t, appName, pod.ObjectMeta.Labels["app"])
	assert.Equal(t, c.Namespace, pod.ObjectMeta.Labels["mon_cluster"])

	// config w/ rook binary init container
	cont := pod.Spec.InitContainers[0]
	assert.Equal(t, "rook/rook:myversion", cont.Image)
	assert.Equal(t, 0, len(cont.Ports))
	assert.Equal(t, 3, len(cont.VolumeMounts))
	assert.Equal(t, cephconfig.DefaultConfigMount(), cont.VolumeMounts[1])
	assert.Equal(t, 7, len(cont.Env))
	assert.False(t, *cont.SecurityContext.Privileged)
	logCommandWithArgs("confg init", cont.Command, cont.Args)
	assert.Equal(t, 0, len(cont.Command))
	assert.Equal(t, "ceph", cont.Args[0])
	assert.Equal(t, mondaemon.InitCommand, cont.Args[1])
	assert.Equal(t, "--config-dir=/var/lib/rook", cont.Args[2])
	assert.Equal(t, fmt.Sprintf("--name=%s", name), cont.Args[3])
	assert.Equal(t, "--port=6790", cont.Args[4])
	assert.Equal(t, fmt.Sprintf("--fsid=%s", c.clusterInfo.FSID), cont.Args[5])

	// monmap init
	cont = pod.Spec.InitContainers[1]
	assert.Equal(t, "rook/rook:myversion", cont.Image)
	assert.Equal(t, 0, len(cont.Ports))
	assert.Equal(t, 2, len(cont.VolumeMounts))
	assert.Equal(t, cephconfig.DefaultConfigMount(), cont.VolumeMounts[1])
	assert.Equal(t, 0, len(cont.Env))
	assert.False(t, *cont.SecurityContext.Privileged)
	logCommandWithArgs("monmap init", cont.Command, cont.Args)
	assert.Equal(t, 1, len(cont.Command))
	assert.Equal(t, "/usr/bin/monmaptool", cont.Command[0])
	assert.Equal(t, "/var/lib/rook/mon-a/monmap", cont.Args[0])
	assert.Equal(t, "--create", cont.Args[1])
	assert.Equal(t, "--clobber", cont.Args[2])
	assert.Equal(t, "--fsid", cont.Args[3])
	assert.Equal(t, c.clusterInfo.FSID, cont.Args[4])

	// mon fs init
	cont = pod.Spec.InitContainers[2]
	assert.Equal(t, "rook/rook:myversion", cont.Image)
	assert.Equal(t, 0, len(cont.Ports))
	assert.Equal(t, 2, len(cont.VolumeMounts))
	assert.Equal(t, cephconfig.DefaultConfigMount(), cont.VolumeMounts[1])
	assert.Equal(t, 0, len(cont.Env))
	assert.False(t, *cont.SecurityContext.Privileged)
	logCommandWithArgs("mon fs init", cont.Command, cont.Args)
	assert.Equal(t, 1, len(cont.Command))
	assert.Equal(t, "/usr/bin/ceph-mon", cont.Command[0])
	assert.Equal(t, "--mkfs", cont.Args[0])
	assert.Equal(t, "--monmap", cont.Args[1])
	assert.Equal(t, "/var/lib/rook/mon-a/monmap", cont.Args[2])
	testCephMonCommonArgs(t, c, name, cont, 3)

	// main mon daemon
	cont = pod.Spec.Containers[0]
	assert.Equal(t, "rook/rook:myversion", cont.Image)
	assert.Equal(t, 1, len(cont.Ports))
	// 6790/TCP
	assert.Equal(t, cont.Ports[0].ContainerPort, int32(6790))
	assert.Equal(t, cont.Ports[0].Protocol, v1.ProtocolTCP)
	assert.Equal(t, 2, len(cont.VolumeMounts))
	assert.Equal(t, cephconfig.DefaultConfigMount(), cont.VolumeMounts[1])
	assert.Equal(t, 0, len(cont.Env))
	assert.False(t, *cont.SecurityContext.Privileged)
	logCommandWithArgs("main mon daemon", cont.Command, cont.Args)
	assert.Equal(t, 1, len(cont.Command))
	assert.Equal(t, "/usr/bin/ceph-mon", cont.Command[0])
	assert.Equal(t, "--foreground", cont.Args[0])
	assert.Equal(t, "--public-addr", cont.Args[1])
	assert.Equal(t, "2.4.6.1:6790", cont.Args[2])
	testCephMonCommonArgs(t, c, name, cont, 3)

	assert.Equal(t, "100", cont.Resources.Limits.Cpu().String())
	assert.Equal(t, "1337", cont.Resources.Requests.Memory().String())
}

func logCommandWithArgs(moniker string, command, args []string) {
	logger.Infof("%s command : %s %s", moniker, strings.Join(command, " "), strings.Join(args, " "))
}
