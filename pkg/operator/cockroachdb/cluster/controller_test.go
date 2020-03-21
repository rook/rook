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
package cluster

import (
	"fmt"
	"strings"
	"testing"

	cockroachdbv1alpha1 "github.com/rook/rook/pkg/apis/cockroachdb.rook.io/v1alpha1"
	rookv1 "github.com/rook/rook/pkg/apis/rook.io/v1"
	rookclient "github.com/rook/rook/pkg/client/clientset/versioned/fake"
	"github.com/rook/rook/pkg/client/clientset/versioned/scheme"
	"github.com/rook/rook/pkg/clusterd"
	opcontroller "github.com/rook/rook/pkg/operator/ceph/controller"
	"github.com/rook/rook/pkg/operator/k8sutil"
	testop "github.com/rook/rook/pkg/operator/test"
	exectest "github.com/rook/rook/pkg/util/exec/test"
	"github.com/stretchr/testify/assert"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	fakeclient "k8s.io/client-go/kubernetes/fake"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

func TestValidateClusterSpec(t *testing.T) {
	// invalid node count
	spec := cockroachdbv1alpha1.ClusterSpec{
		Storage: rookv1.StorageScopeSpec{NodeCount: 0},
		Network: cockroachdbv1alpha1.NetworkSpec{
			Ports: []cockroachdbv1alpha1.PortSpec{
				{Name: "http", Port: 123},
				{Name: "grpc", Port: 456},
			},
		},
	}
	err := ValidateClusterSpec(&spec)
	assert.NotNil(t, err)
	assert.True(t, strings.Contains(err.Error(), "invalid node count"))

	// invalid cache percent
	spec = cockroachdbv1alpha1.ClusterSpec{
		Storage: rookv1.StorageScopeSpec{NodeCount: 1},
		Network: cockroachdbv1alpha1.NetworkSpec{
			Ports: []cockroachdbv1alpha1.PortSpec{
				{Name: "http", Port: 123},
				{Name: "grpc", Port: 456},
			},
		},
		CachePercent: 150,
	}
	err = ValidateClusterSpec(&spec)
	assert.NotNil(t, err)
	assert.True(t, strings.Contains(err.Error(), "invalid value (150) for cache percent"))

	// invalid max SQL memory percent
	spec = cockroachdbv1alpha1.ClusterSpec{
		Storage: rookv1.StorageScopeSpec{NodeCount: 1},
		Network: cockroachdbv1alpha1.NetworkSpec{
			Ports: []cockroachdbv1alpha1.PortSpec{
				{Name: "http", Port: 123},
				{Name: "grpc", Port: 456},
			},
		},
		MaxSQLMemoryPercent: 250,
	}
	err = ValidateClusterSpec(&spec)
	assert.NotNil(t, err)
	assert.True(t, strings.Contains(err.Error(), "invalid value (250) for maxSQLMemory percent"))

	// invalid port spec
	spec = cockroachdbv1alpha1.ClusterSpec{
		Storage: rookv1.StorageScopeSpec{NodeCount: 1},
		Network: cockroachdbv1alpha1.NetworkSpec{
			Ports: []cockroachdbv1alpha1.PortSpec{
				{Name: "foo-port", Port: 123},
			},
		},
	}
	err = ValidateClusterSpec(&spec)
	assert.NotNil(t, err)
	assert.True(t, strings.Contains(err.Error(), "unknown port name"))

	// valid spec
	spec = cockroachdbv1alpha1.ClusterSpec{
		Storage: rookv1.StorageScopeSpec{NodeCount: 1},
		Network: cockroachdbv1alpha1.NetworkSpec{
			Ports: []cockroachdbv1alpha1.PortSpec{
				{Name: "http", Port: 123},
				{Name: "grpc", Port: 456},
			},
		},
	}
	err = ValidateClusterSpec(&spec)
	assert.Nil(t, err)
}

func TestCreateCluster(t *testing.T) {
	name := "cluster-824"
	namespace := "rook-cockroachdb-315"
	cluster := &cockroachdbv1alpha1.Cluster{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
		Spec: cockroachdbv1alpha1.ClusterSpec{
			Storage: rookv1.StorageScopeSpec{
				NodeCount: 5,
				Selection: rookv1.Selection{
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

	object := []runtime.Object{
		cluster,
	}

	executor := &exectest.MockExecutor{
		MockExecuteCommandWithCombinedOutput: func(command string, arg ...string) (string, error) {
			return "", nil
		},
	}
	clientset := testop.New(t, 3)
	context := &clusterd.Context{
		Clientset:     clientset,
		RookClientset: rookclient.NewSimpleClientset(),
		Executor:      executor,
	}

	s := scheme.Scheme
	s.AddKnownTypes(cockroachdbv1alpha1.SchemeGroupVersion, cluster)

	// Create a fake client to mock API calls.
	cl := fake.NewFakeClientWithScheme(s, object...)
	// Create a ReconcileCephBlockPool object with the scheme and fake client.
	r := &ReconcileCockroachDBCluster{client: cl, scheme: s, context: context}
	r.createInitRetryInterval = createInitRetryIntervalDefault
	r.containerImage = "rook/cockroachdb:mockTag"
	ownerRef, _ := opcontroller.GetControllerObjectOwnerReference(cluster, s)

	go simulatePodsRunning(clientset, namespace, cluster.Spec.Storage.NodeCount)

	err := r.CreateCluster(context, cluster, ownerRef)
	assert.Nil(t, err)

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
}

func TestCockroachDBClusterController(t *testing.T) {
	name := "cluster-824"
	namespace := "rook-cockroachdb-315"
	cluster := &cockroachdbv1alpha1.Cluster{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
		Spec: cockroachdbv1alpha1.ClusterSpec{
			Storage: rookv1.StorageScopeSpec{
				NodeCount: 5,
				Selection: rookv1.Selection{
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
		Status: &cockroachdbv1alpha1.Status{},
	}

	executor := &exectest.MockExecutor{
		MockExecuteCommandWithCombinedOutput: func(command string, arg ...string) (string, error) {
			return "", nil
		},
	}
	clientset := testop.New(t, 3)
	context := &clusterd.Context{
		Clientset:     clientset,
		RookClientset: rookclient.NewSimpleClientset(),
		Executor:      executor,
	}

	s := scheme.Scheme
	s.AddKnownTypes(cockroachdbv1alpha1.SchemeGroupVersion, cluster)

	// UPDATE success
	cluster.Status.Phase = k8sutil.ReadyStatus
	object := []runtime.Object{
		cluster,
	}

	// Create a fake client to mock API calls.
	cl := fake.NewFakeClientWithScheme(s, object...)
	// Create a ReconcileCephBlockPool object with the scheme and fake client.
	r := &ReconcileCockroachDBCluster{client: cl, scheme: s, context: context}
	r.createInitRetryInterval = createInitRetryIntervalDefault
	r.containerImage = "rook/cockroachdb:mockTag"

	// Mock request to simulate Reconcile() being called on an event for a
	// watched resource .
	req := reconcile.Request{
		NamespacedName: types.NamespacedName{
			Name:      name,
			Namespace: namespace,
		},
	}

	go simulatePodsRunning(clientset, namespace, cluster.Spec.Storage.NodeCount)

	res, err := r.Reconcile(req)
	assert.NoError(t, err)
	assert.False(t, res.Requeue)
}

func simulatePodsRunning(clientset *fakeclient.Clientset, namespace string, podCount int) {
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
