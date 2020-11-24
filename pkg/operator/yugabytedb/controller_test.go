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
package yugabytedb

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	yugabytedbv1alpha1 "github.com/rook/rook/pkg/apis/yugabytedb.rook.io/v1alpha1"

	"github.com/rook/rook/pkg/clusterd"
	"github.com/rook/rook/pkg/operator/k8sutil"
	testop "github.com/rook/rook/pkg/operator/test"
	"github.com/stretchr/testify/assert"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/kubernetes/fake"
)

const (
	CustomPortShift         = 100
	ClusterName             = "ybdb-cluster"
	VolumeDataName          = "datadir"
	PodCreationWaitInterval = 100 * time.Millisecond
	PodCreationWaitTimeout  = 30 * time.Second
)

func TestValidateClusterSpec(t *testing.T) {
	// invalid master & tserver replica count
	spec := yugabytedbv1alpha1.YBClusterSpec{
		Master: yugabytedbv1alpha1.ServerSpec{
			Replicas: 0,
			Network: yugabytedbv1alpha1.NetworkSpec{
				Ports: []yugabytedbv1alpha1.PortSpec{
					{Name: masterUIPortName, Port: 123},
					{Name: masterRPCPortName, Port: 456},
				},
			},
			VolumeClaimTemplate: v1.PersistentVolumeClaim{},
		},
		TServer: yugabytedbv1alpha1.ServerSpec{
			Replicas:            0,
			Network:             yugabytedbv1alpha1.NetworkSpec{},
			VolumeClaimTemplate: v1.PersistentVolumeClaim{},
		},
	}
	err := validateClusterSpec(spec)
	assert.NotNil(t, err)
	assert.True(t, strings.Contains(err.Error(), "invalid Master replica count"))

	// invalid master replica count
	spec = yugabytedbv1alpha1.YBClusterSpec{
		Master: yugabytedbv1alpha1.ServerSpec{
			Replicas: 0,
			Network: yugabytedbv1alpha1.NetworkSpec{
				Ports: []yugabytedbv1alpha1.PortSpec{
					{Name: masterUIPortName, Port: 123},
					{Name: masterRPCPortName, Port: 456},
				},
			},
			VolumeClaimTemplate: v1.PersistentVolumeClaim{},
		},
		TServer: yugabytedbv1alpha1.ServerSpec{
			Replicas:            1,
			Network:             yugabytedbv1alpha1.NetworkSpec{},
			VolumeClaimTemplate: v1.PersistentVolumeClaim{},
		},
	}
	err = validateClusterSpec(spec)
	assert.NotNil(t, err)
	assert.True(t, strings.Contains(err.Error(), "invalid Master replica count"))

	// invalid tserver replica count
	spec = yugabytedbv1alpha1.YBClusterSpec{
		Master: yugabytedbv1alpha1.ServerSpec{
			Replicas: 1,
			Network: yugabytedbv1alpha1.NetworkSpec{
				Ports: []yugabytedbv1alpha1.PortSpec{
					{Name: masterUIPortName, Port: 123},
					{Name: masterRPCPortName, Port: 456},
				},
			},
			VolumeClaimTemplate: v1.PersistentVolumeClaim{},
		},
		TServer: yugabytedbv1alpha1.ServerSpec{
			Replicas:            0,
			Network:             yugabytedbv1alpha1.NetworkSpec{},
			VolumeClaimTemplate: v1.PersistentVolumeClaim{},
		},
	}
	err = validateClusterSpec(spec)
	assert.NotNil(t, err)
	assert.True(t, strings.Contains(err.Error(), "invalid TServer replica count"))

	// invalid master network spec
	spec = yugabytedbv1alpha1.YBClusterSpec{
		Master: yugabytedbv1alpha1.ServerSpec{
			Replicas: 1,
			Network: yugabytedbv1alpha1.NetworkSpec{
				Ports: []yugabytedbv1alpha1.PortSpec{
					{Name: "http", Port: 123},
				},
			},
			VolumeClaimTemplate: v1.PersistentVolumeClaim{},
		},
		TServer: yugabytedbv1alpha1.ServerSpec{
			Replicas:            1,
			Network:             yugabytedbv1alpha1.NetworkSpec{},
			VolumeClaimTemplate: v1.PersistentVolumeClaim{},
		},
	}
	err = validateClusterSpec(spec)
	assert.NotNil(t, err)
	assert.True(t, strings.Contains(err.Error(), "Invalid port name"))

	// invalid tserver network spec
	spec = yugabytedbv1alpha1.YBClusterSpec{
		Master: yugabytedbv1alpha1.ServerSpec{
			Replicas:            1,
			Network:             yugabytedbv1alpha1.NetworkSpec{},
			VolumeClaimTemplate: v1.PersistentVolumeClaim{},
		},
		TServer: yugabytedbv1alpha1.ServerSpec{
			Replicas: 1,
			Network: yugabytedbv1alpha1.NetworkSpec{
				Ports: []yugabytedbv1alpha1.PortSpec{
					{Name: "http", Port: 123},
				},
			},
			VolumeClaimTemplate: v1.PersistentVolumeClaim{},
		},
	}
	err = validateClusterSpec(spec)
	assert.NotNil(t, err)
	assert.True(t, strings.Contains(err.Error(), "Invalid port name"))

	// Valid spec.
	spec = yugabytedbv1alpha1.YBClusterSpec{
		Master: yugabytedbv1alpha1.ServerSpec{
			Replicas: 1,
			Network: yugabytedbv1alpha1.NetworkSpec{
				Ports: []yugabytedbv1alpha1.PortSpec{
					{Name: masterUIPortName, Port: 123},
					{Name: masterRPCPortName, Port: 456},
				},
			},
			VolumeClaimTemplate: v1.PersistentVolumeClaim{},
		},
		TServer: yugabytedbv1alpha1.ServerSpec{
			Replicas:            1,
			Network:             yugabytedbv1alpha1.NetworkSpec{},
			VolumeClaimTemplate: v1.PersistentVolumeClaim{},
		},
	}
	err = validateClusterSpec(spec)
	assert.Nil(t, err)

	// Valid spec, absent network attribute.
	spec = yugabytedbv1alpha1.YBClusterSpec{
		Master: yugabytedbv1alpha1.ServerSpec{
			Replicas:            1,
			VolumeClaimTemplate: v1.PersistentVolumeClaim{},
		},
		TServer: yugabytedbv1alpha1.ServerSpec{
			Replicas:            1,
			VolumeClaimTemplate: v1.PersistentVolumeClaim{},
		},
	}
	err = validateClusterSpec(spec)
	assert.Nil(t, err)

}

func TestGetPortsFromSpec(t *testing.T) {

	// All ports unspecified. Get all default ports
	spec := yugabytedbv1alpha1.NetworkSpec{}
	ports, err := getPortsFromSpec(spec)
	assert.Nil(t, err)
	assert.Equal(t, masterUIPortDefault, ports.masterPorts.ui)
	assert.Equal(t, masterRPCPortDefault, ports.masterPorts.rpc)
	assert.Equal(t, int32(0), ports.tserverPorts.ui)
	assert.Equal(t, tserverRPCPortDefault, ports.tserverPorts.rpc)
	assert.Equal(t, tserverCassandraPortDefault, ports.tserverPorts.cassandra)
	assert.Equal(t, tserverRedisPortDefault, ports.tserverPorts.redis)
	assert.Equal(t, tserverPostgresPortDefault, ports.tserverPorts.postgres)

	// All ports specified. Get all custom ports
	mUIPort := int32(123)
	mRPCPort := int32(456)
	tsUIPort := int32(789)
	tsRPCPort := int32(012)
	tsCassandraPort := int32(345)
	tsRedisPort := int32(678)
	tsPostgresPort := int32(901)

	spec = yugabytedbv1alpha1.NetworkSpec{
		Ports: []yugabytedbv1alpha1.PortSpec{
			{Name: masterUIPortName, Port: mUIPort},
			{Name: masterRPCPortName, Port: mRPCPort},
			{Name: tserverUIPortName, Port: tsUIPort},
			{Name: tserverRPCPortName, Port: tsRPCPort},
			{Name: tserverCassandraPortName, Port: tsCassandraPort},
			{Name: tserverRedisPortName, Port: tsRedisPort},
			{Name: tserverPostgresPortName, Port: tsPostgresPort},
		},
	}

	ports, err = getPortsFromSpec(spec)
	assert.Nil(t, err)
	assert.Equal(t, mUIPort, ports.masterPorts.ui)
	assert.Equal(t, mRPCPort, ports.masterPorts.rpc)
	assert.Equal(t, tsUIPort, ports.tserverPorts.ui)
	assert.Equal(t, tsRPCPort, ports.tserverPorts.rpc)
	assert.Equal(t, tsCassandraPort, ports.tserverPorts.cassandra)
	assert.Equal(t, tsRedisPort, ports.tserverPorts.redis)
	assert.Equal(t, tsPostgresPort, ports.tserverPorts.postgres)

	// All ports specified, except TServer-UI. Get all custom ports, except TServer-UI being 0.
	spec = yugabytedbv1alpha1.NetworkSpec{
		Ports: []yugabytedbv1alpha1.PortSpec{
			{Name: masterUIPortName, Port: mUIPort},
			{Name: masterRPCPortName, Port: mRPCPort},
			{Name: tserverRPCPortName, Port: tsRPCPort},
			{Name: tserverCassandraPortName, Port: tsCassandraPort},
			{Name: tserverRedisPortName, Port: tsRedisPort},
			{Name: tserverPostgresPortName, Port: tsPostgresPort},
		},
	}

	ports, err = getPortsFromSpec(spec)
	assert.Nil(t, err)
	assert.Equal(t, mUIPort, ports.masterPorts.ui)
	assert.Equal(t, mRPCPort, ports.masterPorts.rpc)
	assert.Equal(t, int32(0), ports.tserverPorts.ui)
	assert.Equal(t, tsRPCPort, ports.tserverPorts.rpc)
	assert.Equal(t, tsCassandraPort, ports.tserverPorts.cassandra)
	assert.Equal(t, tsRedisPort, ports.tserverPorts.redis)
	assert.Equal(t, tsPostgresPort, ports.tserverPorts.postgres)
}

func TestCreateMasterContainerCommand(t *testing.T) {
	replicationFactor := int32(3)
	resources := v1.ResourceRequirements{}

	expectedCommand := getMasterContainerCommand(replicationFactor)
	actualCommand := createMasterContainerCommand("default", masterNamePlural, masterName, int32(7100), replicationFactor, resources)

	assert.Equal(t, expectedCommand, actualCommand)
}

func TestCreateTServerContainerCommand(t *testing.T) {
	replicationFactor := int32(3)
	resources := v1.ResourceRequirements{}

	expectedCommand := getTserverContainerCommand(replicationFactor)
	actualCommand := createTServerContainerCommand("default", tserverNamePlural, masterNamePlural, masterName, int32(7100), int32(9100), int32(5433), replicationFactor, resources)

	assert.Equal(t, expectedCommand, actualCommand)
}

func TestCreateMasterContainerCommandRF1(t *testing.T) {
	replicationFactor := int32(1)
	resources := v1.ResourceRequirements{}

	expectedCommand := getMasterContainerCommand(replicationFactor)
	actualCommand := createMasterContainerCommand("default", masterNamePlural, masterName, int32(7100), replicationFactor, resources)

	assert.Equal(t, expectedCommand, actualCommand)
}

func TestCreateTServerContainerCommandRF1(t *testing.T) {
	replicationFactor := int32(1)
	resources := v1.ResourceRequirements{}

	expectedCommand := getTserverContainerCommand(replicationFactor)
	actualCommand := createTServerContainerCommand("default", tserverNamePlural, masterNamePlural, masterName, int32(7100), int32(9100), int32(5433), replicationFactor, resources)

	assert.Equal(t, expectedCommand, actualCommand)
}

func getMasterContainerCommand(replicationFactor int32) []string {
	expectedCommand := []string{
		"/home/yugabyte/bin/yb-master",
		"--fs_data_dirs=/mnt/data0",
		fmt.Sprintf("--rpc_bind_addresses=$(POD_IP):%d", masterRPCPortDefault),
		fmt.Sprintf("--server_broadcast_addresses=$(POD_NAME).yb-masters:%d", masterRPCPortDefault),
		"--use_private_ip=never",
		fmt.Sprintf("--master_addresses=%s", getMasterAddresses(masterName, masterNamePlural, "default", replicationFactor, int32(7100))),
		"--enable_ysql=true",
		fmt.Sprintf("--replication_factor=%d", replicationFactor),
		"--logtostderr",
	}
	return expectedCommand
}

func getTserverContainerCommand(replicationFactor int32) []string {
	expectedCommand := []string{
		"/home/yugabyte/bin/yb-tserver",
		"--fs_data_dirs=/mnt/data0",
		fmt.Sprintf("--rpc_bind_addresses=$(POD_IP):%d", tserverRPCPortDefault),
		fmt.Sprintf("--server_broadcast_addresses=$(POD_NAME).yb-tservers:%d", tserverRPCPortDefault),
		fmt.Sprintf("--pgsql_proxy_bind_address=$(POD_IP):%d", tserverPostgresPortDefault),
		"--use_private_ip=never",
		fmt.Sprintf("--tserver_master_addrs=%s", getMasterAddresses(masterName, masterNamePlural, "default", replicationFactor, int32(7100))),
		"--enable_ysql=true",
		"--logtostderr",
	}
	return expectedCommand
}

func TestOnAdd(t *testing.T) {
	ctx := context.TODO()
	namespace := "rook-yugabytedb"

	// initialize the controller and its dependencies
	clientset := testop.New(t, 3)
	context := &clusterd.Context{Clientset: clientset}
	controller := NewClusterController(context, "rook/yugabytedb:mockTag")
	controllerSet := &ControllerSet{
		ClientSet:  clientset,
		Context:    context,
		Controller: controller,
	}

	cluster := simulateARunningYugabyteCluster(ctx, t, controllerSet, namespace, int32(3), false)

	expectedServicePorts := []v1.ServicePort{
		{Name: uiPortName, Port: masterUIPortDefault, TargetPort: intstr.FromInt(int(masterUIPortDefault))},
	}

	// verify Master UI service is created
	clientService, err := clientset.CoreV1().Services(namespace).Get(ctx, addCRNameSuffix(masterUIServiceName), metav1.GetOptions{})
	assert.Nil(t, err)
	assert.NotNil(t, clientService)
	assert.Equal(t, v1.ServiceTypeClusterIP, clientService.Spec.Type)
	assert.Equal(t, expectedServicePorts, clientService.Spec.Ports)

	// verify TServer UI service is NOT created
	clientService, err = clientset.CoreV1().Services(namespace).Get(ctx, addCRNameSuffix(tserverUIServiceName), metav1.GetOptions{})
	assert.Nil(t, clientService)
	assert.NotNil(t, err)
	assert.True(t, errors.IsNotFound(err))

	// verify Master headless Service is created
	expectedServicePorts = []v1.ServicePort{
		{Name: uiPortName, Port: masterUIPortDefault, TargetPort: intstr.FromInt(int(masterUIPortDefault))},
		{Name: rpcPortName, Port: masterRPCPortDefault, TargetPort: intstr.FromInt(int(masterRPCPortDefault))},
	}

	headlessService, err := clientset.CoreV1().Services(namespace).Get(ctx, addCRNameSuffix(masterNamePlural), metav1.GetOptions{})
	assert.Nil(t, err)
	assert.NotNil(t, headlessService)
	assert.Equal(t, "None", headlessService.Spec.ClusterIP)
	assert.Equal(t, expectedServicePorts, headlessService.Spec.Ports)

	// verify TServer headless Service is created
	expectedServicePorts = []v1.ServicePort{
		{Name: uiPortName, Port: tserverUIPortDefault, TargetPort: intstr.FromInt(int(tserverUIPortDefault))},
		{Name: rpcPortName, Port: tserverRPCPortDefault, TargetPort: intstr.FromInt(int(tserverRPCPortDefault))},
		{Name: cassandraPortName, Port: tserverCassandraPortDefault, TargetPort: intstr.FromInt(int(tserverCassandraPortDefault))},
		{Name: redisPortName, Port: tserverRedisPortDefault, TargetPort: intstr.FromInt(int(tserverRedisPortDefault))},
		{Name: postgresPortName, Port: tserverPostgresPortDefault, TargetPort: intstr.FromInt(int(tserverPostgresPortDefault))},
	}

	headlessService, err = clientset.CoreV1().Services(namespace).Get(ctx, addCRNameSuffix(tserverNamePlural), metav1.GetOptions{})
	assert.Nil(t, err)
	assert.NotNil(t, headlessService)
	assert.Equal(t, "None", headlessService.Spec.ClusterIP)
	assert.Equal(t, expectedServicePorts, headlessService.Spec.Ports)

	// verify Master statefulSet is created
	statefulSets, err := clientset.AppsV1().StatefulSets(namespace).Get(ctx, addCRNameSuffix(masterName), metav1.GetOptions{})
	assert.Nil(t, err)
	assert.NotNil(t, statefulSets)
	assert.Equal(t, int32(3), *statefulSets.Spec.Replicas)
	assert.Equal(t, 1, len(statefulSets.Spec.VolumeClaimTemplates))

	vct := *cluster.Spec.Master.VolumeClaimTemplate.DeepCopy()
	vct.Name = addCRNameSuffix(vct.Name)
	expectedVolumeClaimTemplates := []v1.PersistentVolumeClaim{vct}
	assert.Equal(t, expectedVolumeClaimTemplates, statefulSets.Spec.VolumeClaimTemplates)
	assert.Equal(t, 1, len(statefulSets.Spec.Template.Spec.Containers))

	container := statefulSets.Spec.Template.Spec.Containers[0]
	expectedContainerPorts := []v1.ContainerPort{
		{Name: masterContainerUIPortName, ContainerPort: masterUIPortDefault},
		{Name: masterContainerRPCPortName, ContainerPort: masterRPCPortDefault},
	}
	assert.Equal(t, expectedContainerPorts, container.Ports)
	assert.NotNil(t, container.Resources)
	assert.Equal(t, 2, len(container.Resources.Requests))
	assert.Equal(t, 2, len(container.Resources.Limits))

	reqCPU, reqOk := container.Resources.Requests[v1.ResourceCPU]
	limCPU, limOk := container.Resources.Limits[v1.ResourceCPU]
	assert.True(t, reqOk)
	assert.True(t, limOk)
	assert.Equal(t, 0, (&reqCPU).Cmp(resource.MustParse(podCPULimitDefault)))
	assert.Equal(t, 0, (&limCPU).Cmp(resource.MustParse(podCPULimitDefault)))

	reqMem, reqOk := container.Resources.Requests[v1.ResourceMemory]
	limMem := container.Resources.Limits[v1.ResourceMemory]
	assert.True(t, reqOk)
	assert.True(t, reqOk)
	assert.Equal(t, 0, (&reqMem).Cmp(resource.MustParse(masterMemLimitDefault)))
	assert.Equal(t, 0, (&limMem).Cmp(resource.MustParse(masterMemLimitDefault)))

	volumeMountName := addCRNameSuffix(cluster.Spec.Master.VolumeClaimTemplate.Name)
	expectedVolumeMounts := []v1.VolumeMount{{Name: volumeMountName, MountPath: volumeMountPath}}
	assert.Equal(t, expectedVolumeMounts, container.VolumeMounts)

	expectedEnvVars := []v1.EnvVar{
		{
			Name:  envGetHostsFrom,
			Value: envGetHostsFromVal},
		{
			Name: envPodIP,
			ValueFrom: &v1.EnvVarSource{
				FieldRef: &v1.ObjectFieldSelector{
					FieldPath: envPodIPVal,
				},
			},
		},
		{
			Name: k8sutil.PodNameEnvVar,
			ValueFrom: &v1.EnvVarSource{
				FieldRef: &v1.ObjectFieldSelector{
					FieldPath: envPodNameVal,
				},
			},
		},
	}
	assert.Equal(t, expectedEnvVars, container.Env)

	expectedCommand := []string{
		"/home/yugabyte/bin/yb-master",
		"--fs_data_dirs=/mnt/data0",
		"--rpc_bind_addresses=$(POD_IP):7100",
		fmt.Sprintf("--server_broadcast_addresses=$(POD_NAME).%s:7100", addCRNameSuffix(masterNamePlural)),
		"--use_private_ip=never",
		fmt.Sprintf("--master_addresses=%s", getMasterAddresses(addCRNameSuffix(masterName), addCRNameSuffix(masterNamePlural), namespace, int32(3), int32(7100))),
		"--enable_ysql=true",
		"--replication_factor=3",
		"--logtostderr",
		"--memory_limit_hard_bytes=1824522240",
	}
	assert.Equal(t, expectedCommand, container.Command)

	// verify Master pods are created
	pods, err := clientset.CoreV1().Pods(namespace).List(ctx, metav1.ListOptions{
		LabelSelector: fmt.Sprintf("%s=%s", k8sutil.AppAttr, addCRNameSuffix(masterName)),
	})
	assert.Nil(t, err)
	assert.NotNil(t, pods)
	assert.Equal(t, 3, len(pods.Items))

	// verify TServer statefulSet is created
	statefulSets, err = clientset.AppsV1().StatefulSets(namespace).Get(ctx, addCRNameSuffix(tserverName), metav1.GetOptions{})
	assert.Nil(t, err)
	assert.NotNil(t, statefulSets)
	assert.Equal(t, int32(3), *statefulSets.Spec.Replicas)
	assert.Equal(t, 1, len(statefulSets.Spec.VolumeClaimTemplates))

	vct = *cluster.Spec.TServer.VolumeClaimTemplate.DeepCopy()
	vct.Name = addCRNameSuffix(vct.Name)
	expectedVolumeClaimTemplates = []v1.PersistentVolumeClaim{vct}
	assert.Equal(t, expectedVolumeClaimTemplates, statefulSets.Spec.VolumeClaimTemplates)
	assert.Equal(t, 1, len(statefulSets.Spec.Template.Spec.Containers))

	container = statefulSets.Spec.Template.Spec.Containers[0]
	expectedContainerPorts = []v1.ContainerPort{
		{Name: tserverContainerUIPortName, ContainerPort: tserverUIPortDefault},
		{Name: tserverContainerRPCPortName, ContainerPort: tserverRPCPortDefault},
		{Name: cassandraPortName, ContainerPort: tserverCassandraPortDefault},
		{Name: redisPortName, ContainerPort: tserverRedisPortDefault},
		{Name: postgresPortName, ContainerPort: tserverPostgresPortDefault},
	}
	assert.Equal(t, expectedContainerPorts, container.Ports)
	assert.NotNil(t, container.Resources)
	assert.Equal(t, 2, len(container.Resources.Requests))
	assert.Equal(t, 2, len(container.Resources.Limits))

	reqCPU, reqOk = container.Resources.Requests[v1.ResourceCPU]
	limCPU, limOk = container.Resources.Limits[v1.ResourceCPU]
	assert.True(t, reqOk)
	assert.True(t, limOk)
	assert.Equal(t, 0, (&reqCPU).Cmp(resource.MustParse(podCPULimitDefault)))
	assert.Equal(t, 0, (&limCPU).Cmp(resource.MustParse(podCPULimitDefault)))

	reqMem, reqOk = container.Resources.Requests[v1.ResourceMemory]
	limMem, limOk = container.Resources.Limits[v1.ResourceMemory]
	assert.True(t, limOk)
	assert.True(t, reqOk)
	assert.Equal(t, 0, (&reqMem).Cmp(resource.MustParse(tserverMemLimitDefault)))
	assert.Equal(t, 0, (&limMem).Cmp(resource.MustParse(tserverMemLimitDefault)))

	volumeMountName = addCRNameSuffix(cluster.Spec.TServer.VolumeClaimTemplate.Name)
	expectedVolumeMounts = []v1.VolumeMount{{Name: volumeMountName, MountPath: volumeMountPath}}
	assert.Equal(t, expectedVolumeMounts, container.VolumeMounts)

	expectedEnvVars = []v1.EnvVar{
		{
			Name:  envGetHostsFrom,
			Value: envGetHostsFromVal},
		{
			Name: envPodIP,
			ValueFrom: &v1.EnvVarSource{
				FieldRef: &v1.ObjectFieldSelector{
					FieldPath: envPodIPVal,
				},
			},
		},
		{
			Name: k8sutil.PodNameEnvVar,
			ValueFrom: &v1.EnvVarSource{
				FieldRef: &v1.ObjectFieldSelector{
					FieldPath: envPodNameVal,
				},
			},
		},
	}
	assert.Equal(t, expectedEnvVars, container.Env)

	expectedCommand = []string{
		"/home/yugabyte/bin/yb-tserver",
		"--fs_data_dirs=/mnt/data0",
		"--rpc_bind_addresses=$(POD_IP):9100",
		fmt.Sprintf("--server_broadcast_addresses=$(POD_NAME).%s:9100", addCRNameSuffix(tserverNamePlural)),
		"--pgsql_proxy_bind_address=$(POD_IP):5433",
		"--use_private_ip=never",
		fmt.Sprintf("--tserver_master_addrs=%s", getMasterAddresses(addCRNameSuffix(masterName), addCRNameSuffix(masterNamePlural), namespace, int32(3), int32(7100))),
		"--enable_ysql=true",
		"--logtostderr",
		"--memory_limit_hard_bytes=3649044480",
	}
	assert.Equal(t, expectedCommand, container.Command)

	// verify TServer pods are created
	pods, err = clientset.CoreV1().Pods(namespace).List(ctx, metav1.ListOptions{
		LabelSelector: fmt.Sprintf("%s=%s", k8sutil.AppAttr, addCRNameSuffix(tserverName)),
	})
	assert.Nil(t, err)
	assert.NotNil(t, pods)
	assert.Equal(t, 3, len(pods.Items))
}

func TestOnAddWithTServerUI(t *testing.T) {
	ctx := context.TODO()
	namespace := "rook-yugabytedb"

	// initialize the controller and its dependencies
	clientset := testop.New(t, 3)
	context := &clusterd.Context{Clientset: clientset}
	controller := NewClusterController(context, "rook/yugabytedb:mockTag")
	controllerSet := &ControllerSet{
		ClientSet:  clientset,
		Context:    context,
		Controller: controller,
	}

	simulateARunningYugabyteCluster(ctx, t, controllerSet, namespace, int32(1), true)

	expectedServicePorts := []v1.ServicePort{
		{Name: uiPortName, Port: masterUIPortDefault, TargetPort: intstr.FromInt(int(masterUIPortDefault))},
	}

	// verify Master UI service is created
	clientService, err := clientset.CoreV1().Services(namespace).Get(ctx, addCRNameSuffix(masterUIServiceName), metav1.GetOptions{})
	assert.Nil(t, err)
	assert.NotNil(t, clientService)
	assert.Equal(t, v1.ServiceTypeClusterIP, clientService.Spec.Type)
	assert.Equal(t, expectedServicePorts, clientService.Spec.Ports)

	// verify TServer UI service is ALSO created
	expectedServicePorts = []v1.ServicePort{
		{Name: uiPortName, Port: tserverUIPortDefault, TargetPort: intstr.FromInt(int(tserverUIPortDefault))},
	}

	clientService, err = clientset.CoreV1().Services(namespace).Get(ctx, addCRNameSuffix(tserverUIServiceName), metav1.GetOptions{})
	assert.Nil(t, err)
	assert.NotNil(t, clientService)
	assert.Equal(t, v1.ServiceTypeClusterIP, clientService.Spec.Type)
	assert.Equal(t, expectedServicePorts, clientService.Spec.Ports)

}

type ControllerSet struct {
	ClientSet  *fake.Clientset
	Context    *clusterd.Context
	Controller *ClusterController
}

func TestOnUpdate_replicaCount(t *testing.T) {
	ctx := context.TODO()
	// initialize the controller and its dependencies
	namespace := "rook-yugabytedb"
	initialReplicatCount := 3
	clientset := testop.New(t, initialReplicatCount)
	context := &clusterd.Context{Clientset: clientset}
	controller := NewClusterController(context, "rook/yugabytedb:mockTag")
	controllerSet := &ControllerSet{
		ClientSet:  clientset,
		Context:    context,
		Controller: controller,
	}

	cluster := simulateARunningYugabyteCluster(ctx, t, controllerSet, namespace, int32(initialReplicatCount), false)

	// Verify all must-have components exist before updation.
	verifyAllComponentsExist(ctx, t, clientset, namespace)

	// verify TServer UI service is NOT present
	clientService, _ := clientset.CoreV1().Services(namespace).Get(ctx, addCRNameSuffix(tserverUIServiceName), metav1.GetOptions{})
	assert.Nil(t, clientService)

	// verify Master pods count matches initial count.
	pods, _ := clientset.CoreV1().Pods(namespace).List(ctx, metav1.ListOptions{
		LabelSelector: fmt.Sprintf("%s=%s", k8sutil.AppAttr, addCRNameSuffix(masterName)),
	})
	assert.NotNil(t, pods)
	assert.Equal(t, initialReplicatCount, len(pods.Items))

	// verify TServer pods count matches initial count.
	pods, _ = clientset.CoreV1().Pods(namespace).List(ctx, metav1.ListOptions{
		LabelSelector: fmt.Sprintf("%s=%s", k8sutil.AppAttr, addCRNameSuffix(tserverName)),
	})
	assert.NotNil(t, pods)
	assert.Equal(t, initialReplicatCount, len(pods.Items))

	// Update replica size
	updatedMasterReplicaCount := 1
	updatedTServerReplicaCount := 2

	newCluster := cluster.DeepCopy()

	newCluster.Spec.Master.Replicas = int32(updatedMasterReplicaCount)
	newCluster.Spec.TServer.Replicas = int32(updatedTServerReplicaCount)

	controller.OnUpdate(cluster, newCluster)

	// Verify all must-have components exist after updation.
	verifyAllComponentsExist(ctx, t, clientset, namespace)

	// verify TServer UI service is NOT present
	clientService, _ = clientset.CoreV1().Services(namespace).Get(ctx, addCRNameSuffix(tserverUIServiceName), metav1.GetOptions{})
	assert.Nil(t, clientService)

	// verify Master pods count matches updated count.
	pods, _ = clientset.CoreV1().Pods(namespace).List(ctx, metav1.ListOptions{
		LabelSelector: fmt.Sprintf("%s=%s", k8sutil.AppAttr, addCRNameSuffix(masterName)),
	})
	assert.NotNil(t, pods)
	assert.Equal(t, initialReplicatCount, len(pods.Items))

	// verify TServer pods count matches updated count.
	pods, _ = clientset.CoreV1().Pods(namespace).List(ctx, metav1.ListOptions{
		LabelSelector: fmt.Sprintf("%s=%s", k8sutil.AppAttr, addCRNameSuffix(tserverName)),
	})
	assert.NotNil(t, pods)
	assert.Equal(t, initialReplicatCount, len(pods.Items))
}

func TestOnUpdate_volumeClaimTemplate(t *testing.T) {
	ctx := context.TODO()
	// initialize the controller and its dependencies
	namespace := "rook-yugabytedb"
	initialReplicatCount := 3
	clientset := testop.New(t, initialReplicatCount)
	context := &clusterd.Context{Clientset: clientset}
	controller := NewClusterController(context, "rook/yugabytedb:mockTag")
	controllerSet := &ControllerSet{
		ClientSet:  clientset,
		Context:    context,
		Controller: controller,
	}

	cluster := simulateARunningYugabyteCluster(ctx, t, controllerSet, namespace, int32(initialReplicatCount), false)

	// Verify all must-have components exist before updation.
	verifyAllComponentsExist(ctx, t, clientset, namespace)

	// verify TServer UI service is NOT present
	clientService, _ := clientset.CoreV1().Services(namespace).Get(ctx, addCRNameSuffix(tserverUIServiceName), metav1.GetOptions{})
	assert.Nil(t, clientService)

	// verify Master VolumeClaimTemplate size is as set initially.
	statefulSet, _ := clientset.AppsV1().StatefulSets(namespace).Get(ctx, addCRNameSuffix(masterName), metav1.GetOptions{})
	assert.NotNil(t, statefulSet)
	vct := statefulSet.Spec.VolumeClaimTemplates[0]
	assert.NotNil(t, vct)
	assert.Equal(t, resource.MustParse("1Mi"), vct.Spec.Resources.Requests[v1.ResourceStorage])

	// verify TServer VolumeClaimTemplate size is as set initially.
	statefulSet, _ = clientset.AppsV1().StatefulSets(namespace).Get(ctx, addCRNameSuffix(tserverName), metav1.GetOptions{})
	assert.NotNil(t, statefulSet)
	vct = statefulSet.Spec.VolumeClaimTemplates[0]
	assert.NotNil(t, vct)
	assert.Equal(t, resource.MustParse("1Mi"), vct.Spec.Resources.Requests[v1.ResourceStorage])

	// Update volumeclaimtemplates size
	newCluster := cluster.DeepCopy()

	newCluster.Spec.Master.VolumeClaimTemplate.Spec.Resources.Requests = v1.ResourceList{
		v1.ResourceStorage: resource.MustParse("10Mi"),
	}
	newCluster.Spec.TServer.VolumeClaimTemplate.Spec.Resources.Requests = v1.ResourceList{
		v1.ResourceStorage: resource.MustParse("20Mi"),
	}

	controller.OnUpdate(cluster, newCluster)

	// Verify all must-have components exist after updation.
	verifyAllComponentsExist(ctx, t, clientset, namespace)

	// verify TServer UI service is NOT present
	clientService, _ = clientset.CoreV1().Services(namespace).Get(ctx, addCRNameSuffix(tserverUIServiceName), metav1.GetOptions{})
	assert.Nil(t, clientService)

	// verify Master VolumeClaimTemplate is updated.
	statefulSet, _ = clientset.AppsV1().StatefulSets(namespace).Get(ctx, addCRNameSuffix(masterName), metav1.GetOptions{})
	assert.NotNil(t, statefulSet)
	vct = statefulSet.Spec.VolumeClaimTemplates[0]
	assert.NotNil(t, vct)
	assert.Equal(t, resource.MustParse("10Mi"), vct.Spec.Resources.Requests[v1.ResourceStorage])

	// verify TServer VolumeClaimTemplate is updated.
	statefulSet, _ = clientset.AppsV1().StatefulSets(namespace).Get(ctx, addCRNameSuffix(tserverName), metav1.GetOptions{})
	assert.NotNil(t, statefulSet)
	vct = statefulSet.Spec.VolumeClaimTemplates[0]
	assert.NotNil(t, vct)
	assert.Equal(t, resource.MustParse("20Mi"), vct.Spec.Resources.Requests[v1.ResourceStorage])
}

func TestOnUpdate_updateNetworkPorts(t *testing.T) {
	ctx := context.TODO()
	// initialize the controller and its dependencies
	namespace := "rook-yugabytedb"
	initialReplicatCount := 3
	clientset := testop.New(t, initialReplicatCount)
	context := &clusterd.Context{Clientset: clientset}
	controller := NewClusterController(context, "rook/yugabytedb:mockTag")
	controllerSet := &ControllerSet{
		ClientSet:  clientset,
		Context:    context,
		Controller: controller,
	}

	cluster := simulateARunningYugabyteCluster(ctx, t, controllerSet, namespace, int32(initialReplicatCount), false)

	verifyAllComponentsExist(ctx, t, clientset, namespace)

	// verify Master UI service ports
	expectedMUIServicePorts := getMasterUIServicePortsList(false)

	clientService, _ := clientset.CoreV1().Services(namespace).Get(ctx, addCRNameSuffix(masterUIServiceName), metav1.GetOptions{})
	assert.NotNil(t, clientService)
	assert.Equal(t, v1.ServiceTypeClusterIP, clientService.Spec.Type)
	assert.Equal(t, expectedMUIServicePorts, clientService.Spec.Ports)

	// verify TServer UI service is NOT created
	clientService, _ = clientset.CoreV1().Services(namespace).Get(ctx, addCRNameSuffix(tserverUIServiceName), metav1.GetOptions{})
	assert.Nil(t, clientService)

	// verify Master headless Service ports
	expectedMHServicePorts := getMasterHeadlessServicePortsList(false)

	headlessService, _ := clientset.CoreV1().Services(namespace).Get(ctx, addCRNameSuffix(masterNamePlural), metav1.GetOptions{})
	assert.NotNil(t, headlessService)
	assert.Equal(t, "None", headlessService.Spec.ClusterIP)
	assert.Equal(t, expectedMHServicePorts, headlessService.Spec.Ports)

	// verify TServer headless Service ports
	expectedTHServicePorts := getTServerHeadlessServicePortsList(false)

	headlessService, _ = clientset.CoreV1().Services(namespace).Get(ctx, addCRNameSuffix(tserverNamePlural), metav1.GetOptions{})
	assert.NotNil(t, headlessService)
	assert.Equal(t, "None", headlessService.Spec.ClusterIP)
	assert.Equal(t, expectedTHServicePorts, headlessService.Spec.Ports)

	// Update network ports
	newCluster := cluster.DeepCopy()

	newCluster.Spec.Master.Network = getUpdatedMasterNetworkSpec()
	newCluster.Spec.TServer.Network = getUpdatedTServerNetworkSpec()

	controller.OnUpdate(cluster, newCluster)

	verifyAllComponentsExist(ctx, t, clientset, namespace)

	// verify Master UI service ports
	expectedMUIServicePorts = getMasterUIServicePortsList(true)

	clientService, _ = clientset.CoreV1().Services(namespace).Get(ctx, addCRNameSuffix(masterUIServiceName), metav1.GetOptions{})
	assert.NotNil(t, clientService)
	assert.Equal(t, v1.ServiceTypeClusterIP, clientService.Spec.Type)
	assert.Equal(t, expectedMUIServicePorts, clientService.Spec.Ports)

	// verify TServer UI service is NOT created
	clientService, _ = clientset.CoreV1().Services(namespace).Get(ctx, addCRNameSuffix(tserverUIServiceName), metav1.GetOptions{})
	assert.Nil(t, clientService)

	// verify Master headless Service ports
	expectedMHServicePorts = getMasterHeadlessServicePortsList(true)

	headlessService, _ = clientset.CoreV1().Services(namespace).Get(ctx, addCRNameSuffix(masterNamePlural), metav1.GetOptions{})
	assert.NotNil(t, headlessService)
	assert.Equal(t, "None", headlessService.Spec.ClusterIP)
	assert.Equal(t, expectedMHServicePorts, headlessService.Spec.Ports)

	// verify TServer headless Service ports
	expectedTHServicePorts = getTServerHeadlessServicePortsList(true)

	// Since updated YBDB cluster spec doesn't have TServer UI port specified, the port on TServer Headless service will be of default value.
	expectedTHServicePorts[0].Port = tserverUIPortDefault
	expectedTHServicePorts[0].TargetPort = intstr.FromInt(int(tserverUIPortDefault))

	headlessService, _ = clientset.CoreV1().Services(namespace).Get(ctx, addCRNameSuffix(tserverNamePlural), metav1.GetOptions{})
	assert.NotNil(t, headlessService)
	assert.Equal(t, "None", headlessService.Spec.ClusterIP)
	assert.Equal(t, expectedTHServicePorts, headlessService.Spec.Ports)
}

func TestOnUpdate_addTServerUIPort(t *testing.T) {
	ctx := context.TODO()
	// initialize the controller and its dependencies
	namespace := "rook-yugabytedb"
	initialReplicatCount := 3
	clientset := testop.New(t, initialReplicatCount)
	context := &clusterd.Context{Clientset: clientset}
	controller := NewClusterController(context, "rook/yugabytedb:mockTag")
	controllerSet := &ControllerSet{
		ClientSet:  clientset,
		Context:    context,
		Controller: controller,
	}

	cluster := simulateARunningYugabyteCluster(ctx, t, controllerSet, namespace, int32(initialReplicatCount), false)

	verifyAllComponentsExist(ctx, t, clientset, namespace)

	// verify Master UI service ports
	expectedMUIServicePorts := getMasterUIServicePortsList(false)

	clientService, _ := clientset.CoreV1().Services(namespace).Get(ctx, addCRNameSuffix(masterUIServiceName), metav1.GetOptions{})
	assert.NotNil(t, clientService)
	assert.Equal(t, v1.ServiceTypeClusterIP, clientService.Spec.Type)
	assert.Equal(t, expectedMUIServicePorts, clientService.Spec.Ports)

	// verify TServer UI service is NOT created
	clientService, _ = clientset.CoreV1().Services(namespace).Get(ctx, addCRNameSuffix(tserverUIServiceName), metav1.GetOptions{})
	assert.Nil(t, clientService)

	// verify Master headless Service ports
	expectedMHServicePorts := getMasterHeadlessServicePortsList(false)

	headlessService, _ := clientset.CoreV1().Services(namespace).Get(ctx, addCRNameSuffix(masterNamePlural), metav1.GetOptions{})
	assert.NotNil(t, headlessService)
	assert.Equal(t, "None", headlessService.Spec.ClusterIP)
	assert.Equal(t, expectedMHServicePorts, headlessService.Spec.Ports)

	// verify TServer headless Service ports
	expectedTHServicePorts := getTServerHeadlessServicePortsList(false)

	headlessService, _ = clientset.CoreV1().Services(namespace).Get(ctx, addCRNameSuffix(tserverNamePlural), metav1.GetOptions{})
	assert.NotNil(t, headlessService)
	assert.Equal(t, "None", headlessService.Spec.ClusterIP)
	assert.Equal(t, expectedTHServicePorts, headlessService.Spec.Ports)

	// Update network ports, which contain TServer UI Port number
	newCluster := cluster.DeepCopy()

	updatedTServerNetworkSpec := getUpdatedTServerNetworkSpec()
	updatedTServerNetworkSpec.Ports = append(updatedTServerNetworkSpec.Ports, yugabytedbv1alpha1.PortSpec{
		Name: tserverUIPortName,
		Port: tserverUIPortDefault + int32(CustomPortShift),
	})

	newCluster.Spec.Master.Network = getUpdatedMasterNetworkSpec()
	newCluster.Spec.TServer.Network = updatedTServerNetworkSpec

	logger.Info("Updated TServer network specs: ", newCluster.Spec.TServer.Network)

	controller.OnUpdate(cluster, newCluster)

	verifyAllComponentsExist(ctx, t, clientset, namespace)

	// verify Master UI service ports
	expectedMUIServicePorts = getMasterUIServicePortsList(true)

	clientService, _ = clientset.CoreV1().Services(namespace).Get(ctx, addCRNameSuffix(masterUIServiceName), metav1.GetOptions{})
	assert.NotNil(t, clientService)
	assert.Equal(t, v1.ServiceTypeClusterIP, clientService.Spec.Type)
	assert.Equal(t, expectedMUIServicePorts, clientService.Spec.Ports)

	// verify TServer UI service IS created
	expectedTUIServicePorts := getTServerUIServicePortsList(true)

	clientService2, err := clientset.CoreV1().Services(namespace).Get(ctx, addCRNameSuffix(tserverUIServiceName), metav1.GetOptions{})
	assert.Nil(t, err)
	assert.NotNil(t, clientService2)
	assert.Equal(t, v1.ServiceTypeClusterIP, clientService2.Spec.Type)
	assert.Equal(t, expectedTUIServicePorts, clientService2.Spec.Ports)

	// verify Master headless Service ports
	expectedMHServicePorts = getMasterHeadlessServicePortsList(true)

	headlessService, _ = clientset.CoreV1().Services(namespace).Get(ctx, addCRNameSuffix(masterNamePlural), metav1.GetOptions{})
	assert.NotNil(t, headlessService)
	assert.Equal(t, "None", headlessService.Spec.ClusterIP)
	assert.Equal(t, expectedMHServicePorts, headlessService.Spec.Ports)

	// verify TServer headless Service ports
	expectedTHServicePorts = getTServerHeadlessServicePortsList(true)

	headlessService, _ = clientset.CoreV1().Services(namespace).Get(ctx, addCRNameSuffix(tserverNamePlural), metav1.GetOptions{})
	assert.NotNil(t, headlessService)
	assert.Equal(t, "None", headlessService.Spec.ClusterIP)
	assert.Equal(t, expectedTHServicePorts, headlessService.Spec.Ports)
}

func TestOnUpdate_removeTServerUIPort(t *testing.T) {
	ctx := context.TODO()
	// initialize the controller and its dependencies
	namespace := "rook-yugabytedb"
	initialReplicatCount := 3
	clientset := testop.New(t, initialReplicatCount)
	context := &clusterd.Context{Clientset: clientset}
	controller := NewClusterController(context, "rook/yugabytedb:mockTag")
	controllerSet := &ControllerSet{
		ClientSet:  clientset,
		Context:    context,
		Controller: controller,
	}

	cluster := simulateARunningYugabyteCluster(ctx, t, controllerSet, namespace, int32(initialReplicatCount), true)

	verifyAllComponentsExist(ctx, t, clientset, namespace)

	// verify Master UI service ports
	expectedMUIServicePorts := getMasterUIServicePortsList(false)

	clientService, _ := clientset.CoreV1().Services(namespace).Get(ctx, addCRNameSuffix(masterUIServiceName), metav1.GetOptions{})
	assert.NotNil(t, clientService)
	assert.Equal(t, v1.ServiceTypeClusterIP, clientService.Spec.Type)
	assert.Equal(t, expectedMUIServicePorts, clientService.Spec.Ports)

	// verify TServer UI service IS created
	expectedTUIServicePorts := getTServerUIServicePortsList(false)

	clientService2, err := clientset.CoreV1().Services(namespace).Get(ctx, addCRNameSuffix(tserverUIServiceName), metav1.GetOptions{})
	assert.Nil(t, err)
	assert.NotNil(t, clientService2)
	assert.Equal(t, v1.ServiceTypeClusterIP, clientService2.Spec.Type)
	assert.Equal(t, expectedTUIServicePorts, clientService2.Spec.Ports)

	// verify Master headless Service ports
	expectedMHServicePorts := getMasterHeadlessServicePortsList(false)

	headlessService, _ := clientset.CoreV1().Services(namespace).Get(ctx, addCRNameSuffix(masterNamePlural), metav1.GetOptions{})
	assert.NotNil(t, headlessService)
	assert.Equal(t, "None", headlessService.Spec.ClusterIP)
	assert.Equal(t, expectedMHServicePorts, headlessService.Spec.Ports)

	// verify TServer headless Service ports
	expectedTHServicePorts := getTServerHeadlessServicePortsList(false)

	headlessService, _ = clientset.CoreV1().Services(namespace).Get(ctx, addCRNameSuffix(tserverNamePlural), metav1.GetOptions{})
	assert.NotNil(t, headlessService)
	assert.Equal(t, "None", headlessService.Spec.ClusterIP)
	assert.Equal(t, expectedTHServicePorts, headlessService.Spec.Ports)

	// Update network ports, which lack TServer UI Port number
	newCluster := cluster.DeepCopy()

	newCluster.Spec.Master.Network = getUpdatedMasterNetworkSpec()
	newCluster.Spec.TServer.Network = getUpdatedTServerNetworkSpec()

	controller.OnUpdate(cluster, newCluster)

	verifyAllComponentsExist(ctx, t, clientset, namespace)

	// verify Master UI service ports
	expectedMUIServicePorts = getMasterUIServicePortsList(true)

	clientService, _ = clientset.CoreV1().Services(namespace).Get(ctx, addCRNameSuffix(masterUIServiceName), metav1.GetOptions{})
	assert.NotNil(t, clientService)
	assert.Equal(t, v1.ServiceTypeClusterIP, clientService.Spec.Type)
	assert.Equal(t, expectedMUIServicePorts, clientService.Spec.Ports)

	// verify TServer UI service IS DELETED
	clientService2, err = clientset.CoreV1().Services(namespace).Get(ctx, addCRNameSuffix(tserverUIServiceName), metav1.GetOptions{})
	assert.NotNil(t, err)
	assert.True(t, errors.IsNotFound(err))
	assert.Nil(t, clientService2)

	// verify Master headless Service ports
	expectedMHServicePorts = getMasterHeadlessServicePortsList(true)

	headlessService, _ = clientset.CoreV1().Services(namespace).Get(ctx, addCRNameSuffix(masterNamePlural), metav1.GetOptions{})
	assert.NotNil(t, headlessService)
	assert.Equal(t, "None", headlessService.Spec.ClusterIP)
	assert.Equal(t, expectedMHServicePorts, headlessService.Spec.Ports)

	// verify TServer headless Service ports
	expectedTHServicePorts = getTServerHeadlessServicePortsList(true)

	// Since updated YBDB cluster spec doesn't have TServer UI port specified, the port on TServer Headless service will be of default value.
	expectedTHServicePorts[0].Port = tserverUIPortDefault
	expectedTHServicePorts[0].TargetPort = intstr.FromInt(int(tserverUIPortDefault))

	headlessService, _ = clientset.CoreV1().Services(namespace).Get(ctx, addCRNameSuffix(tserverNamePlural), metav1.GetOptions{})
	assert.NotNil(t, headlessService)
	assert.Equal(t, "None", headlessService.Spec.ClusterIP)
	assert.Equal(t, expectedTHServicePorts, headlessService.Spec.Ports)
}

// Verify all must-have components exist after updation.
func verifyAllComponentsExist(ctx context.Context, t *testing.T, clientset *fake.Clientset, namespace string) {
	// verify Master UI service is created
	clientService, err := clientset.CoreV1().Services(namespace).Get(ctx, addCRNameSuffix(masterUIServiceName), metav1.GetOptions{})
	assert.Nil(t, err)
	assert.NotNil(t, clientService)

	// verify Master headless Service is created
	headlessService, err := clientset.CoreV1().Services(namespace).Get(ctx, addCRNameSuffix(masterNamePlural), metav1.GetOptions{})
	assert.Nil(t, err)
	assert.NotNil(t, headlessService)

	// verify TServer headless Service is created
	headlessService, err = clientset.CoreV1().Services(namespace).Get(ctx, addCRNameSuffix(tserverNamePlural), metav1.GetOptions{})
	assert.Nil(t, err)
	assert.NotNil(t, headlessService)

	// verify Master statefulSet is created
	statefulSets, err := clientset.AppsV1().StatefulSets(namespace).Get(ctx, addCRNameSuffix(masterName), metav1.GetOptions{})
	assert.NoError(t, err)
	assert.NotNil(t, statefulSets)

	// verify Master pods are created
	pods, err := clientset.CoreV1().Pods(namespace).List(ctx, metav1.ListOptions{
		LabelSelector: fmt.Sprintf("%s=%s", k8sutil.AppAttr, addCRNameSuffix(masterName)),
	})
	assert.Nil(t, err)
	assert.NotNil(t, pods)

	// verify TServer statefulSet is created
	statefulSets, err = clientset.AppsV1().StatefulSets(namespace).Get(ctx, addCRNameSuffix(tserverName), metav1.GetOptions{})
	assert.Nil(t, err)
	assert.NotNil(t, statefulSets)

	// verify TServer pods are created
	pods, err = clientset.CoreV1().Pods(namespace).List(ctx, metav1.ListOptions{
		LabelSelector: fmt.Sprintf("%s=%s", k8sutil.AppAttr, addCRNameSuffix(tserverName)),
	})
	assert.Nil(t, err)
	assert.NotNil(t, pods)
}

func simulateARunningYugabyteCluster(ctx context.Context, t *testing.T, controllerSet *ControllerSet, namespace string, replicaCount int32, addTServerUIService bool) *yugabytedbv1alpha1.YBCluster {
	cluster := &yugabytedbv1alpha1.YBCluster{
		ObjectMeta: metav1.ObjectMeta{
			Name:      ClusterName,
			Namespace: namespace,
		},
		Spec: yugabytedbv1alpha1.YBClusterSpec{
			Master: yugabytedbv1alpha1.ServerSpec{
				Replicas: replicaCount,
				VolumeClaimTemplate: v1.PersistentVolumeClaim{
					ObjectMeta: metav1.ObjectMeta{
						Name: VolumeDataName,
					},
					Spec: v1.PersistentVolumeClaimSpec{
						AccessModes: []v1.PersistentVolumeAccessMode{
							v1.ReadWriteOnce,
						},
						Resources: v1.ResourceRequirements{
							Requests: v1.ResourceList{
								v1.ResourceStorage: resource.MustParse("1Mi"),
							},
						},
					},
				},
			},
			TServer: yugabytedbv1alpha1.ServerSpec{
				Replicas: replicaCount,
				VolumeClaimTemplate: v1.PersistentVolumeClaim{
					ObjectMeta: metav1.ObjectMeta{
						Name: VolumeDataName,
					},
					Spec: v1.PersistentVolumeClaimSpec{
						AccessModes: []v1.PersistentVolumeAccessMode{
							v1.ReadWriteOnce,
						},
						Resources: v1.ResourceRequirements{
							Requests: v1.ResourceList{
								v1.ResourceStorage: resource.MustParse("1Mi"),
							},
						},
					},
				},
			},
		},
	}

	if addTServerUIService {
		cluster.Spec.TServer.Network.Ports = append(cluster.Spec.TServer.Network.Ports, yugabytedbv1alpha1.PortSpec{
			Name: tserverUIPortName,
			Port: tserverUIPortDefault,
		})
	}

	// in a background thread, simulate running pods for Master & TServer processes. (fake statefulsets don't automatically do that)
	go simulateMasterPodsRunning(ctx, t, controllerSet.ClientSet, namespace, replicaCount)
	go simulateTServerPodsRunning(ctx, t, controllerSet.ClientSet, namespace, replicaCount)

	// Wait for Pods to start & go to running state
	waitForPodsToStart(ctx, controllerSet.ClientSet, namespace, replicaCount)

	// call OnAdd given the specified cluster
	controllerSet.Controller.OnAdd(cluster)

	return cluster
}

func simulateMasterPodsRunning(ctx context.Context, t *testing.T, clientset *fake.Clientset, namespace string, podCount int32) {
	simulatePodsRunning(ctx, t, clientset, namespace, podCount, addCRNameSuffix(masterName))
}

func simulateTServerPodsRunning(ctx context.Context, t *testing.T, clientset *fake.Clientset, namespace string, podCount int32) {
	simulatePodsRunning(ctx, t, clientset, namespace, podCount, addCRNameSuffix(tserverName))
}

func waitForPodsToStart(ctx context.Context, clientset *fake.Clientset, namespace string, podCount int32) {
	logger.Info("Waiting for Master & TServer pods to start & go to running state")
	err := wait.Poll(PodCreationWaitInterval, PodCreationWaitTimeout, func() (bool, error) {
		// Check if Master Pods are running
		if err := isPodsRunning(ctx, clientset, namespace, masterName, podCount); err != nil {
			logger.Warningf("Master pods are not yet running: %+v", err)
			return false, nil
		}

		// Check if TServer Pods are running
		if err := isPodsRunning(ctx, clientset, namespace, tserverName, podCount); err != nil {
			logger.Warningf("TServer pods are not yet running: %+v", err)
			return false, nil
		}

		return true, nil
	})

	if err != nil {
		logger.Errorf("failed to start Master & TServer pods in namespace %s: %+v", namespace, err)
		return
	}
}

func isPodsRunning(ctx context.Context, clientset *fake.Clientset, namespace, label string, podCount int32) error {
	pods, err := clientset.CoreV1().Pods(namespace).List(ctx, metav1.ListOptions{
		LabelSelector: fmt.Sprintf("%s=%s", k8sutil.AppAttr, addCRNameSuffix(label)),
	})
	if err != nil {
		return fmt.Errorf("failed to list %s pods. error: %+v", label, err)
	}

	runningPods := len(k8sutil.GetPodPhaseMap(pods)[v1.PodRunning])
	if runningPods != int(podCount) {
		return fmt.Errorf("need %d %s pods & found %d ", podCount, label, runningPods)
	}

	return nil
}

func simulatePodsRunning(ctx context.Context, t *testing.T, clientset *fake.Clientset, namespace string, podCount int32, podName string) {
	for i := 0; i < int(podCount); i++ {
		pod := &v1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Name:      fmt.Sprintf("%s-%d", podName, i),
				Namespace: namespace,
				Labels:    map[string]string{k8sutil.AppAttr: podName},
			},
			Status: v1.PodStatus{Phase: v1.PodRunning},
		}
		_, err := clientset.CoreV1().Pods(namespace).Create(ctx, pod, metav1.CreateOptions{})
		if err != nil {
			assert.NoError(t, err)
		}
	}
}

func getMasterUIServicePortsList(returnCustomPorts bool) []v1.ServicePort {
	if returnCustomPorts {
		return []v1.ServicePort{
			getServicePort(uiPortName, int(masterUIPortDefault)+CustomPortShift),
		}
	}

	return []v1.ServicePort{
		getServicePort(uiPortName, int(masterUIPortDefault)),
	}
}

func getTServerUIServicePortsList(returnCustomPorts bool) []v1.ServicePort {
	if returnCustomPorts {
		return []v1.ServicePort{
			getServicePort(uiPortName, int(tserverUIPortDefault)+CustomPortShift),
		}
	}

	return []v1.ServicePort{
		getServicePort(uiPortName, int(tserverUIPortDefault)),
	}
}

func getMasterHeadlessServicePortsList(returnCustomPorts bool) []v1.ServicePort {
	if returnCustomPorts {
		return []v1.ServicePort{
			getServicePort(uiPortName, int(masterUIPortDefault)+CustomPortShift),
			getServicePort(rpcPortName, int(masterRPCPortDefault)+CustomPortShift),
		}
	}

	return []v1.ServicePort{
		getServicePort(uiPortName, int(masterUIPortDefault)),
		getServicePort(rpcPortName, int(masterRPCPortDefault)),
	}
}

func getTServerHeadlessServicePortsList(returnCustomPorts bool) []v1.ServicePort {
	if returnCustomPorts {
		return []v1.ServicePort{
			getServicePort(uiPortName, int(tserverUIPortDefault)+CustomPortShift),
			getServicePort(rpcPortName, int(tserverRPCPortDefault)+CustomPortShift),
			getServicePort(cassandraPortName, int(tserverCassandraPortDefault)+CustomPortShift),
			getServicePort(redisPortName, int(tserverRedisPortDefault)+CustomPortShift),
			getServicePort(postgresPortName, int(tserverPostgresPortDefault)+CustomPortShift),
		}
	}

	return []v1.ServicePort{
		getServicePort(uiPortName, int(tserverUIPortDefault)),
		getServicePort(rpcPortName, int(tserverRPCPortDefault)),
		getServicePort(cassandraPortName, int(tserverCassandraPortDefault)),
		getServicePort(redisPortName, int(tserverRedisPortDefault)),
		getServicePort(postgresPortName, int(tserverPostgresPortDefault)),
	}
}

func getServicePort(portName string, portNum int) v1.ServicePort {
	return v1.ServicePort{
		Name:       portName,
		Port:       int32(portNum),
		TargetPort: intstr.FromInt(portNum),
	}
}

func getUpdatedMasterNetworkSpec() yugabytedbv1alpha1.NetworkSpec {
	return yugabytedbv1alpha1.NetworkSpec{
		Ports: []yugabytedbv1alpha1.PortSpec{
			{
				Name: masterUIPortName,
				Port: masterUIPortDefault + int32(CustomPortShift),
			},
			{
				Name: masterRPCPortName,
				Port: masterRPCPortDefault + int32(CustomPortShift),
			},
		},
	}
}

func getUpdatedTServerNetworkSpec() yugabytedbv1alpha1.NetworkSpec {
	return yugabytedbv1alpha1.NetworkSpec{
		Ports: []yugabytedbv1alpha1.PortSpec{
			{
				Name: tserverRPCPortName,
				Port: tserverRPCPortDefault + int32(CustomPortShift),
			},
			{
				Name: tserverCassandraPortName,
				Port: tserverCassandraPortDefault + int32(CustomPortShift),
			},
			{
				Name: tserverPostgresPortName,
				Port: tserverPostgresPortDefault + int32(CustomPortShift),
			},
			{
				Name: tserverRedisPortName,
				Port: tserverRedisPortDefault + int32(CustomPortShift),
			},
		},
	}
}

func addCRNameSuffix(str string) string {
	return fmt.Sprintf("%s-%s", str, ClusterName)
}
