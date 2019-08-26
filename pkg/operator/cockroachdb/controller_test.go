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
package cockroachdb

import (
	"fmt"
	"strings"
	"testing"
	"time"

	cockroachdbv1alpha1 "github.com/rook/rook/pkg/apis/cockroachdb.rook.io/v1alpha1"
	rookalpha "github.com/rook/rook/pkg/apis/rook.io/v1alpha2"
	"github.com/rook/rook/pkg/clusterd"
	"github.com/rook/rook/pkg/operator/k8sutil"
	testop "github.com/rook/rook/pkg/operator/test"
	exectest "github.com/rook/rook/pkg/util/exec/test"
	"github.com/stretchr/testify/assert"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/client-go/kubernetes/fake"
)

func TestValidateClusterSpec(t *testing.T) {
	// invalid node count
	spec := cockroachdbv1alpha1.ClusterSpec{
		Storage: rookalpha.StorageScopeSpec{NodeCount: 0},
		Network: cockroachdbv1alpha1.NetworkSpec{
			Ports: []cockroachdbv1alpha1.PortSpec{
				{Name: "http", Port: 123},
				{Name: "grpc", Port: 456},
			},
		},
	}
	err := validateClusterSpec(spec)
	assert.NotNil(t, err)
	assert.True(t, strings.Contains(err.Error(), "invalid node count"))

	// invalid cache percent
	spec = cockroachdbv1alpha1.ClusterSpec{
		Storage: rookalpha.StorageScopeSpec{NodeCount: 1},
		Network: cockroachdbv1alpha1.NetworkSpec{
			Ports: []cockroachdbv1alpha1.PortSpec{
				{Name: "http", Port: 123},
				{Name: "grpc", Port: 456},
			},
		},
		CachePercent: 150,
	}
	err = validateClusterSpec(spec)
	assert.NotNil(t, err)
	assert.True(t, strings.Contains(err.Error(), "invalid value (150) for cache percent"))

	// invalid max SQL memory percent
	spec = cockroachdbv1alpha1.ClusterSpec{
		Storage: rookalpha.StorageScopeSpec{NodeCount: 1},
		Network: cockroachdbv1alpha1.NetworkSpec{
			Ports: []cockroachdbv1alpha1.PortSpec{
				{Name: "http", Port: 123},
				{Name: "grpc", Port: 456},
			},
		},
		MaxSQLMemoryPercent: 250,
	}
	err = validateClusterSpec(spec)
	assert.NotNil(t, err)
	assert.True(t, strings.Contains(err.Error(), "invalid value (250) for maxSQLMemory percent"))

	// invalid port spec
	spec = cockroachdbv1alpha1.ClusterSpec{
		Storage: rookalpha.StorageScopeSpec{NodeCount: 1},
		Network: cockroachdbv1alpha1.NetworkSpec{
			Ports: []cockroachdbv1alpha1.PortSpec{
				{Name: "foo-port", Port: 123},
			},
		},
	}
	err = validateClusterSpec(spec)
	assert.NotNil(t, err)
	assert.True(t, strings.Contains(err.Error(), "unknown port name"))

	// valid spec
	spec = cockroachdbv1alpha1.ClusterSpec{
		Storage: rookalpha.StorageScopeSpec{NodeCount: 1},
		Network: cockroachdbv1alpha1.NetworkSpec{
			Ports: []cockroachdbv1alpha1.PortSpec{
				{Name: "http", Port: 123},
				{Name: "grpc", Port: 456},
			},
		},
	}
	err = validateClusterSpec(spec)
	assert.Nil(t, err)
}

func TestOnAdd(t *testing.T) {
	namespace := "rook-cockroachdb-315"
	cluster := &cockroachdbv1alpha1.Cluster{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "cluster-824",
			Namespace: namespace,
		},
		Spec: cockroachdbv1alpha1.ClusterSpec{
			Storage: rookalpha.StorageScopeSpec{
				NodeCount: 5,
				Selection: rookalpha.Selection{
					VolumeClaimTemplates: []v1.PersistentVolumeClaim{
						{
							ObjectMeta: metav1.ObjectMeta{
								Name: "rook-cockroachdb-test",
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
			},
			Network: cockroachdbv1alpha1.NetworkSpec{
				Ports: []cockroachdbv1alpha1.PortSpec{
					{Name: "http", Port: 123},
					{Name: "grpc", Port: 456},
				},
			},
			CachePercent:        30,
			MaxSQLMemoryPercent: 40,
		},
	}

	// keep track of if the cockroachdb init command was called
	initCalled := false
	executor := &exectest.MockExecutor{
		MockExecuteCommandWithCombinedOutput: func(debug bool, actionName string, command string, arg ...string) (string, error) {
			if strings.Contains(command, "cockroach") && arg[0] == "init" {
				initCalled = true
			}

			return "", nil
		},
	}

	// initialize the controller and its dependencies
	clientset := testop.New(3)
	context := &clusterd.Context{Clientset: clientset, Executor: executor}
	controller := NewClusterController(context, "rook/cockroachdb:mockTag")
	controller.createInitRetryInterval = 1 * time.Millisecond

	// in a background thread, simulate the pods running (fake statefulsets don't automatically do that)
	go simulatePodsRunning(clientset, namespace, cluster.Spec.Storage.NodeCount)

	// call onAdd given the specified cluster
	controller.onAdd(cluster)

	expectedServicePorts := []v1.ServicePort{
		{Name: "grpc", Port: int32(456), TargetPort: intstr.FromInt(456)},
		{Name: "http", Port: int32(123), TargetPort: intstr.FromInt(123)},
	}

	// verify client service
	clientService, err := clientset.CoreV1().Services(namespace).Get("cockroachdb-public", metav1.GetOptions{})
	assert.Nil(t, err)
	assert.NotNil(t, clientService)
	assert.Equal(t, v1.ServiceTypeClusterIP, clientService.Spec.Type)
	assert.Equal(t, expectedServicePorts, clientService.Spec.Ports)

	// verify replica Service
	replicaService, err := clientset.CoreV1().Services(namespace).Get(appName, metav1.GetOptions{})
	assert.Nil(t, err)
	assert.NotNil(t, replicaService)
	assert.Equal(t, "None", replicaService.Spec.ClusterIP)
	assert.Equal(t, expectedServicePorts, replicaService.Spec.Ports)
	assert.True(t, replicaService.Spec.PublishNotReadyAddresses)
	assert.Equal(t, "123", replicaService.Annotations["prometheus.io/port"])

	// verify pod disruption budget
	pdb, err := clientset.PolicyV1beta1().PodDisruptionBudgets(namespace).Get("cockroachdb-budget", metav1.GetOptions{})
	assert.Nil(t, err)
	assert.NotNil(t, pdb)
	assert.Equal(t, createAppLabels(), pdb.Spec.Selector.MatchLabels)
	assert.Equal(t, int32(1), pdb.Spec.MaxUnavailable.IntVal)

	// verify stateful set
	ss, err := clientset.AppsV1().StatefulSets(namespace).Get(appName, metav1.GetOptions{})
	assert.Nil(t, err)
	assert.NotNil(t, ss)
	assert.Equal(t, int32(5), *ss.Spec.Replicas)
	assert.Equal(t, 1, len(ss.Spec.VolumeClaimTemplates))
	assert.Equal(t, cluster.Spec.Storage.Selection.VolumeClaimTemplates,
		ss.Spec.VolumeClaimTemplates)
	assert.Equal(t, 1, len(ss.Spec.Template.Spec.Containers))
	container := ss.Spec.Template.Spec.Containers[0]

	expectedContainerPorts := []v1.ContainerPort{
		{Name: "grpc", ContainerPort: int32(456)},
		{Name: "http", ContainerPort: int32(123)},
	}
	assert.Equal(t, expectedContainerPorts, container.Ports)

	expectedVolumeMounts := []v1.VolumeMount{{Name: "rook-cockroachdb-test", MountPath: "/cockroach/cockroach-data"}}
	assert.Equal(t, expectedVolumeMounts, container.VolumeMounts)

	expectedEnvVars := []v1.EnvVar{{Name: "COCKROACH_CHANNEL", Value: "kubernetes-insecure"}}
	assert.Equal(t, expectedEnvVars, container.Env)

	expectedCommand := []string{"/bin/bash", "-ecx", `exec /cockroach/cockroach start --logtostderr --insecure --advertise-host $(hostname -f) --http-host 0.0.0.0 --port 456 --http-port 123 --join rook-cockroachdb-0.rook-cockroachdb.rook-cockroachdb-315,rook-cockroachdb-1.rook-cockroachdb.rook-cockroachdb-315,rook-cockroachdb-2.rook-cockroachdb.rook-cockroachdb-315,rook-cockroachdb-3.rook-cockroachdb.rook-cockroachdb-315,rook-cockroachdb-4.rook-cockroachdb.rook-cockroachdb-315 --cache 30% --max-sql-memory 40%`}
	assert.Equal(t, expectedCommand, container.Command)

	// cockroachdb init should have been called
	assert.True(t, initCalled)
}

func simulatePodsRunning(clientset *fake.Clientset, namespace string, podCount int) {
	for i := 0; i < podCount; i++ {
		pod := &v1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Name:      fmt.Sprintf("pod%d", i),
				Namespace: namespace,
				Labels:    map[string]string{k8sutil.AppAttr: appName},
			},
			Status: v1.PodStatus{Phase: v1.PodRunning},
		}
		clientset.CoreV1().Pods(namespace).Create(pod)
	}
}
