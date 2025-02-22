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

// Package discover to discover devices on storage nodes.
package discover

import (
	"context"
	"os"
	"testing"

	cephv1 "github.com/rook/rook/pkg/apis/ceph.rook.io/v1"
	"github.com/rook/rook/pkg/clusterd"
	discoverDaemon "github.com/rook/rook/pkg/daemon/discover"
	"github.com/rook/rook/pkg/operator/k8sutil"
	"github.com/rook/rook/pkg/operator/test"

	"github.com/stretchr/testify/assert"

	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestStartDiscoveryDaemonset(t *testing.T) {
	ctx := context.TODO()
	clientset := test.New(t, 3)

	t.Setenv(k8sutil.PodNamespaceEnvVar, "rook-system")
	t.Setenv(k8sutil.PodNameEnvVar, "rook-operator")
	t.Setenv(discoverDaemonsetPriorityClassNameEnv, "my-priority-class")

	namespace := "ns"
	a := New(clientset)

	// Create an operator pod
	pod := v1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "rook-operator",
			Namespace: "rook-system",
		},
		Spec: v1.PodSpec{
			Containers: []v1.Container{
				{
					Name:  "mypodContainer",
					Image: "rook/test",
				},
			},
		},
	}
	_, err := clientset.CoreV1().Pods("rook-system").Create(ctx, &pod, metav1.CreateOptions{})
	assert.NoError(t, err)
	// start a basic cluster
	err = a.Start(ctx, namespace, "rook/rook:myversion", "mysa", false)
	assert.Nil(t, err)

	// check daemonset parameters
	agentDS, err := clientset.AppsV1().DaemonSets(namespace).Get(ctx, "rook-discover", metav1.GetOptions{})
	assert.Nil(t, err)
	assert.Equal(t, namespace, agentDS.Namespace)
	assert.Equal(t, "rook-discover", agentDS.Name)
	assert.Equal(t, "mysa", agentDS.Spec.Template.Spec.ServiceAccountName)
	assert.Equal(t, "my-priority-class", agentDS.Spec.Template.Spec.PriorityClassName)
	assert.True(t, *agentDS.Spec.Template.Spec.Containers[0].SecurityContext.Privileged)
	assert.Equal(t, int64(0), *agentDS.Spec.Template.Spec.Containers[0].SecurityContext.RunAsUser)
	volumes := agentDS.Spec.Template.Spec.Volumes
	assert.Equal(t, 3, len(volumes))
	volumeMounts := agentDS.Spec.Template.Spec.Containers[0].VolumeMounts
	assert.Equal(t, 3, len(volumeMounts))
	envs := agentDS.Spec.Template.Spec.Containers[0].Env
	assert.Equal(t, 3, len(envs))
	image := agentDS.Spec.Template.Spec.Containers[0].Image
	assert.Equal(t, "rook/rook:myversion", image)
	assert.Nil(t, agentDS.Spec.Template.Spec.Tolerations)

	// Test with rook override configmap setting
	os.Setenv("DISCOVER_TOLERATIONS", "- effect: NoSchedule\n  key: node-role.kubernetes.io/control-plane\n  operator: Exists\n- effect: NoExecute\n  key: node-role.kubernetes.io/etcd\n  operator: Exists")
	defer os.Unsetenv("DISCOVER_TOLERATIONS")

	// start a basic cluster
	err = a.Start(ctx, namespace, "rook/rook:myversion", "mysa", false)
	assert.Nil(t, err)

	agentDS, err = clientset.AppsV1().DaemonSets(namespace).Get(ctx, "rook-discover", metav1.GetOptions{})
	assert.NoError(t, err)

	want := []v1.Toleration{
		{
			Key:      "node-role.kubernetes.io/control-plane",
			Operator: v1.TolerationOpExists,
			Effect:   v1.TaintEffectNoSchedule,
		},
		{
			Key:      "node-role.kubernetes.io/etcd",
			Operator: v1.TolerationOpExists,
			Effect:   v1.TaintEffectNoExecute,
		},
	}
	assert.Equal(t, want, agentDS.Spec.Template.Spec.Tolerations)
}

func TestGetAvailableDevices(t *testing.T) {
	ctx := context.TODO()
	clientset := test.New(t, 3)
	pvcBackedOSD := false
	ns := "rook-system"
	nodeName := "node123"
	t.Setenv(k8sutil.PodNamespaceEnvVar, ns)
	t.Setenv(k8sutil.PodNameEnvVar, "rook-operator")

	data := make(map[string]string, 1)
	data[discoverDaemon.LocalDiskCMData] = `[{"name":"sdd","parent":"","hasChildren":false,"devLinks":"/dev/disk/by-id/scsi-36001405f826bd553d8c4dbf9f41c18be    /dev/disk/by-id/wwn-0x6001405f826bd553d8c4dbf9f41c18be /dev/disk/by-path/ip-127.0.0.1:3260-iscsi-iqn.2016-06.world.srv:storage.target01-lun-1","size":10737418240,"uuid":"","serial":"36001405f826bd553d8c4dbf9f41c18be","type":"disk","rotational":true,"readOnly":false,"ownPartition":true,"filesystem":"","vendor":"LIO-ORG","model":"disk02","wwn":"0x6001405f826bd553","wwnVendorExtension":"0x6001405f826bd553d8c4dbf9f41c18be","empty":true},{"name":"sdb","parent":"","hasChildren":false,"devLinks":"/dev/disk/by-id/scsi-3600140577f462d9908b409d94114e042   /dev/disk/by-id/wwn-0x600140577f462d9908b409d94114e042 /dev/disk/by-path/ip-127.0.0.1:3260-iscsi-iqn.2016-06.world.srv:storage.target01-lun-3","size":5368709120,"uuid":"","serial":"3600140577f462d9908b409d94114e042","type":"disk","rotational":true,"readOnly":false,"ownPartition":false,"filesystem":"","vendor":"LIO-ORG","model":"disk04","wwn":"0x600140577f462d99","wwnVendorExtension":"0x600140577f462d9908b409d94114e042","empty":true},{"name":"sdc","parent":"","hasChildren":false,"devLinks":"/dev/disk/by-id/scsi-3600140568c0bd28d4ee43769387c9f02    /dev/disk/by-id/wwn-0x600140568c0bd28d4ee43769387c9f02 /dev/disk/by-path/ip-127.0.0.1:3260-iscsi-iqn.2016-06.world.srv:storage.target01-lun-2","size":5368709120,"uuid":"","serial":"3600140568c0bd28d4ee43769387c9f02","type":"disk","rotational":true,"readOnly":false,"ownPartition":true,"filesystem":"","vendor":"LIO-ORG","model":"disk03","wwn":"0x600140568c0bd28d","wwnVendorExtension":"0x600140568c0bd28d4ee43769387c9f02","empty":true},{"name":"sda","parent":"","hasChildren":false,"devLinks":"/dev/disk/by-id/scsi-36001405fc00c75fb4c243aa9d61987bd    /dev/disk/by-id/wwn-0x6001405fc00c75fb4c243aa9d61987bd /dev/disk/by-path/ip-127.0.0.1:3260-iscsi-iqn.2016-06.world.srv:storage.target01-lun-0","size":10737418240,"uuid":"","serial":"36001405fc00c75fb4c243aa9d61987bd","type":"disk","rotational":true,"readOnly":false,"ownPartition":false,"filesystem":"","vendor":"LIO-ORG","model":"disk01","wwn":"0x6001405fc00c75fb","wwnVendorExtension":"0x6001405fc00c75fb4c243aa9d61987bd","empty":true},{"name":"nvme0n1","parent":"","hasChildren":false,"devLinks":"/dev/disk/by-id/nvme-eui.002538c5710091a7","size":512110190592,"uuid":"","serial":"","type":"disk","rotational":false,"readOnly":false,"ownPartition":false,"filesystem":"","vendor":"","model":"","wwn":"","wwnVendorExtension":"","empty":true}]`
	cm := &v1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "local-device-" + nodeName,
			Namespace: ns,
			Labels: map[string]string{
				k8sutil.AppAttr:         discoverDaemon.AppName,
				discoverDaemon.NodeAttr: nodeName,
			},
		},
		Data: data,
	}
	_, err := clientset.CoreV1().ConfigMaps(ns).Create(ctx, cm, metav1.CreateOptions{})
	assert.Nil(t, err)
	context := &clusterd.Context{
		Clientset: clientset,
	}
	d := []cephv1.Device{
		{
			Name: "sdc",
		},
		{
			Name: "foo",
		},
	}

	nodeDevices, err := ListDevices(ctx, context, ns, "" /* all nodes */)
	assert.Nil(t, err)
	assert.Equal(t, 1, len(nodeDevices))

	devices, err := GetAvailableDevices(ctx, context, nodeName, ns, d, "^sd.", pvcBackedOSD)
	assert.Nil(t, err)
	assert.Equal(t, 1, len(devices))
	// devices should be in use now, 2nd try gets the same list
	devices, err = GetAvailableDevices(ctx, context, nodeName, ns, d, "^sd.", pvcBackedOSD)
	assert.Nil(t, err)
	assert.Equal(t, 1, len(devices))
}
