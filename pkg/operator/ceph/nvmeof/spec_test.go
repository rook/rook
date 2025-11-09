/*
Copyright 2025 The Rook Authors. All rights reserved.

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

package nvmeof

import (
	"fmt"
	"strings"
	"testing"

	cephv1 "github.com/rook/rook/pkg/apis/ceph.rook.io/v1"
	rookclient "github.com/rook/rook/pkg/client/clientset/versioned/fake"
	"github.com/rook/rook/pkg/client/clientset/versioned/scheme"
	"github.com/rook/rook/pkg/clusterd"
	cephclient "github.com/rook/rook/pkg/daemon/ceph/client"
	cephconfig "github.com/rook/rook/pkg/operator/ceph/config"
	cephTest "github.com/rook/rook/pkg/operator/ceph/test"
	cephver "github.com/rook/rook/pkg/operator/ceph/version"
	optest "github.com/rook/rook/pkg/operator/test"
	exectest "github.com/rook/rook/pkg/util/exec/test"
	"github.com/stretchr/testify/assert"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func newDeploymentSpecTest(t *testing.T) (*ReconcileCephNVMeOFGateway, string) {
	clientset := optest.New(t, 1)
	c := &clusterd.Context{
		Executor:      &exectest.MockExecutor{},
		RookClientset: rookclient.NewSimpleClientset(),
		Clientset:     clientset,
	}

	s := scheme.Scheme
	object := []runtime.Object{
		&cephv1.CephNVMeOFGateway{
			ObjectMeta: metav1.ObjectMeta{
				Name:      name,
				Namespace: namespace,
			},
			TypeMeta: controllerTypeMeta,
			Spec: cephv1.NVMeOFGatewaySpec{
				Image:     "quay.io/ceph/nvmeof:1.5",
				Pool:      "nvmeof",
				Group:     "group-a",
				Instances: 1,
			},
		},
	}
	cl := fake.NewClientBuilder().WithScheme(s).WithRuntimeObjects(object...).Build()

	r := &ReconcileCephNVMeOFGateway{
		client:  cl,
		scheme:  scheme.Scheme,
		context: c,
		clusterInfo: &cephclient.ClusterInfo{
			FSID:        "myfsid",
			CephVersion: cephver.Squid,
		},
		cephClusterSpec: &cephv1.ClusterSpec{
			CephVersion: cephv1.CephVersionSpec{
				Image: "quay.io/ceph/ceph:v19",
			},
		},
	}

	configHash := "dcb0d2f5f5e86ec4929d8243cd640b8154165f8ff9b89809964fc7993e9b0101"

	return r, configHash
}

func TestDeploymentSpec(t *testing.T) {
	t.Run("basic", func(t *testing.T) {
		nvmeof := &cephv1.CephNVMeOFGateway{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "my-nvmeof",
				Namespace: "rook-ceph-test-ns",
			},
			Spec: cephv1.NVMeOFGatewaySpec{
				Image:     "quay.io/ceph/nvmeof:1.5",
				Pool:      "nvmeof",
				Group:     "group-a",
				Instances: 3,
				Resources: v1.ResourceRequirements{
					Limits: v1.ResourceList{
						v1.ResourceCPU:    *resource.NewQuantity(500.0, resource.BinarySI),
						v1.ResourceMemory: *resource.NewQuantity(1024.0, resource.BinarySI),
					},
					Requests: v1.ResourceList{
						v1.ResourceCPU:    *resource.NewQuantity(200.0, resource.BinarySI),
						v1.ResourceMemory: *resource.NewQuantity(512.0, resource.BinarySI),
					},
				},
				PriorityClassName: "my-priority-class",
			},
		}

		r, configHash := newDeploymentSpecTest(t)

		configMapName := fmt.Sprintf("rook-ceph-nvmeof-%s-config", nvmeof.Name)
		d, err := r.makeDeployment(nvmeof, "0", configMapName, configHash)
		assert.NoError(t, err)
		assert.NotEmpty(t, d.Spec.Template.Annotations)
		assert.Equal(t, configHash, d.Spec.Template.Annotations["config-hash"])

		// Verify deployment labels contain Ceph requirements
		daemonID := nvmeof.Name + "-0"
		cephTest.AssertLabelsContainCephRequirements(t, d.ObjectMeta.Labels,
			"nvmeof", daemonID, AppName, nvmeof.Namespace, nvmeof.Name,
			"cephnvmeofgateways.ceph.rook.io", "ceph-nvmeof")

		podTemplate := optest.NewPodTemplateSpecTester(t, &d.Spec.Template)
		podTemplate.RunFullSuite(
			AppName,
			optest.ResourceLimitExpectations{
				CPUResourceLimit:      "500",
				MemoryResourceLimit:   "1Ki",
				CPUResourceRequest:    "200",
				MemoryResourceRequest: "512",
			},
		)
		assert.Equal(t, "my-priority-class", d.Spec.Template.Spec.PriorityClassName)
		assert.Equal(t, serviceAccountName, d.Spec.Template.Spec.ServiceAccountName)
	})

	t.Run("basic with default liveness-probe", func(t *testing.T) {
		nvmeof := &cephv1.CephNVMeOFGateway{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "my-nvmeof",
				Namespace: "rook-ceph-test-ns",
			},
			Spec: cephv1.NVMeOFGatewaySpec{
				Image:     "quay.io/ceph/nvmeof:1.5",
				Pool:      "nvmeof",
				Group:     "group-a",
				Instances: 3,
				Resources: v1.ResourceRequirements{
					Limits: v1.ResourceList{
						v1.ResourceCPU:    *resource.NewQuantity(500.0, resource.BinarySI),
						v1.ResourceMemory: *resource.NewQuantity(1024.0, resource.BinarySI),
					},
					Requests: v1.ResourceList{
						v1.ResourceCPU:    *resource.NewQuantity(200.0, resource.BinarySI),
						v1.ResourceMemory: *resource.NewQuantity(512.0, resource.BinarySI),
					},
				},
				PriorityClassName: "my-priority-class",
				LivenessProbe: &cephv1.ProbeSpec{
					Disabled: false,
				},
			},
		}

		r, configHash := newDeploymentSpecTest(t)
		configMapName := fmt.Sprintf("rook-ceph-nvmeof-%s-config", nvmeof.Name)
		d, err := r.makeDeployment(nvmeof, "0", configMapName, configHash)
		assert.NoError(t, err)
		assert.NotEmpty(t, d.Spec.Template.Annotations)

		var nvmeofCont *v1.Container = nil
		for i := range d.Spec.Template.Spec.Containers {
			if d.Spec.Template.Spec.Containers[i].Name == "nvmeof-gateway" {
				nvmeofCont = &d.Spec.Template.Spec.Containers[i]
				break
			}
		}
		assert.NotNil(t, nvmeofCont)
		assert.NotNil(t, nvmeofCont.LivenessProbe)
		assert.Equal(t, nvmeofCont.LivenessProbe.InitialDelaySeconds, int32(10))
		assert.Equal(t, nvmeofCont.LivenessProbe.FailureThreshold, int32(10))
		assert.GreaterOrEqual(t, nvmeofCont.LivenessProbe.TimeoutSeconds, int32(5))
	})

	t.Run("with host network", func(t *testing.T) {
		nvmeof := &cephv1.CephNVMeOFGateway{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "my-nvmeof",
				Namespace: "rook-ceph-test-ns",
			},
			Spec: cephv1.NVMeOFGatewaySpec{
				Image:     "quay.io/ceph/nvmeof:1.5",
				Pool:      "nvmeof",
				Group:     "group-a",
				Instances: 1,
			},
		}

		r, configHash := newDeploymentSpecTest(t)
		r.cephClusterSpec.Network = cephv1.NetworkSpec{
			HostNetwork: true,
		}

		configMapName := fmt.Sprintf("rook-ceph-nvmeof-%s-config", nvmeof.Name)
		d, err := r.makeDeployment(nvmeof, "0", configMapName, configHash)
		assert.NoError(t, err)
		assert.True(t, d.Spec.Template.Spec.HostNetwork)
		assert.Equal(t, v1.DNSClusterFirstWithHostNet, d.Spec.Template.Spec.DNSPolicy)

		svc := r.generateCephNVMeOFService(nvmeof, "0")
		assert.Equal(t, v1.ClusterIPNone, svc.Spec.ClusterIP)
	})

	t.Run("with custom configmap ref", func(t *testing.T) {
		nvmeof := &cephv1.CephNVMeOFGateway{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "my-nvmeof",
				Namespace: "rook-ceph-test-ns",
			},
			Spec: cephv1.NVMeOFGatewaySpec{
				Image:        "quay.io/ceph/nvmeof:1.5",
				Pool:         "nvmeof",
				Group:        "group-a",
				Instances:    1,
				ConfigMapRef: "custom-configmap",
			},
		}

		r, configHash := newDeploymentSpecTest(t)

		configMapName := "custom-configmap"
		d, err := r.makeDeployment(nvmeof, "0", configMapName, configHash)
		assert.NoError(t, err)
		assert.NotEmpty(t, d.Spec.Template.Annotations)

		var gatewayConfigVol *v1.Volume = nil
		for i := range d.Spec.Template.Spec.Volumes {
			if d.Spec.Template.Spec.Volumes[i].Name == "gateway-config" {
				gatewayConfigVol = &d.Spec.Template.Spec.Volumes[i]
				break
			}
		}
		assert.NotNil(t, gatewayConfigVol)
		assert.NotNil(t, gatewayConfigVol.ConfigMap)
		assert.Equal(t, "custom-configmap", gatewayConfigVol.ConfigMap.Name)
	})

	t.Run("init container configuration", func(t *testing.T) {
		nvmeof := &cephv1.CephNVMeOFGateway{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "my-nvmeof",
				Namespace: "rook-ceph-test-ns",
			},
			Spec: cephv1.NVMeOFGatewaySpec{
				Image:     "quay.io/ceph/nvmeof:1.5",
				Pool:      "nvmeof",
				Group:     "group-a",
				Instances: 1,
			},
		}

		r, configHash := newDeploymentSpecTest(t)

		configMapName := fmt.Sprintf("rook-ceph-nvmeof-%s-config", nvmeof.Name)
		d, err := r.makeDeployment(nvmeof, "0", configMapName, configHash)
		assert.NoError(t, err)

		assert.Len(t, d.Spec.Template.Spec.InitContainers, 1)
		initCont := d.Spec.Template.Spec.InitContainers[0]
		assert.Equal(t, "generate-ceph-conf", initCont.Name)
		assert.Equal(t, r.cephClusterSpec.CephVersion.Image, initCont.Image)
		assert.Equal(t, []string{"/bin/bash", "-c", connectionConfigScript}, initCont.Command)
		assert.Contains(t, initCont.Args, "--keyring=/etc/ceph/keyring")

		assertEnvVar(t, initCont.Env, "GATEWAY_NAME", instanceName(nvmeof, "0"))
		assertEnvVar(t, initCont.Env, "POOL_NAME", nvmeof.Spec.Pool)
		assertEnvVar(t, initCont.Env, "ANA_GROUP", nvmeof.Spec.Group)
		assertEnvVarPresent(t, initCont.Env, "POD_IP")

		volumeMountNames := []string{}
		for _, vm := range initCont.VolumeMounts {
			volumeMountNames = append(volumeMountNames, vm.Name)
		}
		assert.Contains(t, volumeMountNames, "ceph-admin-keyring")
		assert.Contains(t, volumeMountNames, "gateway-config")
		assert.Contains(t, volumeMountNames, "ceph-conf-emptydir")
	})

	t.Run("daemon container configuration", func(t *testing.T) {
		nvmeof := &cephv1.CephNVMeOFGateway{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "my-nvmeof",
				Namespace: "rook-ceph-test-ns",
			},
			Spec: cephv1.NVMeOFGatewaySpec{
				Image:     "quay.io/ceph/nvmeof:1.5",
				Pool:      "nvmeof",
				Group:     "group-a",
				Instances: 1,
			},
		}

		r, configHash := newDeploymentSpecTest(t)

		configMapName := fmt.Sprintf("rook-ceph-nvmeof-%s-config", nvmeof.Name)
		d, err := r.makeDeployment(nvmeof, "0", configMapName, configHash)
		assert.NoError(t, err)

		assert.Len(t, d.Spec.Template.Spec.Containers, 1)
		daemonCont := d.Spec.Template.Spec.Containers[0]
		assert.Equal(t, "nvmeof-gateway", daemonCont.Name)
		assert.Equal(t, nvmeof.Spec.Image, daemonCont.Image)

		expectedCephArgs := strings.Join(cephconfig.DefaultFlags(r.clusterInfo.FSID, "/etc/ceph/keyring"), " ")
		assertEnvVar(t, daemonCont.Env, "CEPH_ARGS", expectedCephArgs)

		portNames := []string{}
		for _, p := range daemonCont.Ports {
			portNames = append(portNames, p.Name)
		}
		assert.Contains(t, portNames, "io")
		assert.Contains(t, portNames, "gateway")
		assert.Contains(t, portNames, "monitor")
		assert.Contains(t, portNames, "discovery")

		assert.NotNil(t, daemonCont.SecurityContext)
		// SecurityContext should be set with privileged=true for NVMeOF gateway
		assert.NotNil(t, daemonCont.SecurityContext.Privileged)
		assert.True(t, *daemonCont.SecurityContext.Privileged)
	})

	t.Run("daemon container uses default image when not specified", func(t *testing.T) {
		nvmeof := &cephv1.CephNVMeOFGateway{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "my-nvmeof",
				Namespace: "rook-ceph-test-ns",
			},
			Spec: cephv1.NVMeOFGatewaySpec{
				// Image not specified - should use default
				Pool:      "nvmeof",
				Group:     "group-a",
				Instances: 1,
			},
		}

		r, configHash := newDeploymentSpecTest(t)

		configMapName := fmt.Sprintf("rook-ceph-nvmeof-%s-config", nvmeof.Name)
		d, err := r.makeDeployment(nvmeof, "0", configMapName, configHash)
		assert.NoError(t, err)

		assert.Len(t, d.Spec.Template.Spec.Containers, 1)
		daemonCont := d.Spec.Template.Spec.Containers[0]
		assert.Equal(t, "nvmeof-gateway", daemonCont.Name)
		assert.Equal(t, "quay.io/ceph/nvmeof:1.5", daemonCont.Image)
	})

	t.Run("hostname set when instance name is valid", func(t *testing.T) {
		nvmeof := &cephv1.CephNVMeOFGateway{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "my-nvmeof",
				Namespace: "rook-ceph-test-ns",
			},
			Spec: cephv1.NVMeOFGatewaySpec{
				Image:     "quay.io/ceph/nvmeof:1.5",
				Pool:      "nvmeof",
				Group:     "group-a",
				Instances: 1,
			},
		}

		r, configHash := newDeploymentSpecTest(t)
		configMapName := fmt.Sprintf("rook-ceph-nvmeof-%s-config", nvmeof.Name)
		d, err := r.makeDeployment(nvmeof, "0", configMapName, configHash)
		assert.NoError(t, err)
		assert.Equal(t, instanceName(nvmeof, "0"), d.Spec.Template.Spec.Hostname)
	})

	t.Run("hostname not set when instance name is invalid", func(t *testing.T) {
		nvmeof := &cephv1.CephNVMeOFGateway{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "my_nvmeof",
				Namespace: "rook-ceph-test-ns",
			},
			Spec: cephv1.NVMeOFGatewaySpec{
				Image:     "quay.io/ceph/nvmeof:1.5",
				Pool:      "nvmeof",
				Group:     "group-a",
				Instances: 1,
			},
		}

		r, configHash := newDeploymentSpecTest(t)
		configMapName := fmt.Sprintf("rook-ceph-nvmeof-%s-config", nvmeof.Name)
		d, err := r.makeDeployment(nvmeof, "0", configMapName, configHash)
		assert.NoError(t, err)
		assert.Empty(t, d.Spec.Template.Spec.Hostname)
	})

	t.Run("service generation", func(t *testing.T) {
		nvmeof := &cephv1.CephNVMeOFGateway{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "my-nvmeof",
				Namespace: "rook-ceph-test-ns",
			},
			Spec: cephv1.NVMeOFGatewaySpec{
				Image:     "quay.io/ceph/nvmeof:1.5",
				Pool:      "nvmeof",
				Group:     "group-a",
				Instances: 1,
			},
		}

		r, _ := newDeploymentSpecTest(t)

		svc := r.generateCephNVMeOFService(nvmeof, "0")
		assert.Equal(t, "rook-ceph-nvmeof-my-nvmeof-0", svc.Name)
		assert.Equal(t, "rook-ceph-test-ns", svc.Namespace)

		// Verify service labels contain Ceph requirements
		daemonID := nvmeof.Name + "-0"
		cephTest.AssertLabelsContainCephRequirements(t, svc.Labels,
			"nvmeof", daemonID, AppName, nvmeof.Namespace, nvmeof.Name,
			"cephnvmeofgateways.ceph.rook.io", "ceph-nvmeof")

		portNames := []string{}
		for _, p := range svc.Spec.Ports {
			portNames = append(portNames, p.Name)
		}
		assert.Contains(t, portNames, "io")
		assert.Contains(t, portNames, "gateway")
		assert.Contains(t, portNames, "monitor")
		assert.Contains(t, portNames, "discovery")

		for _, p := range svc.Spec.Ports {
			switch p.Name {
			case "io":
				assert.Equal(t, int32(nvmeofIOPort), p.Port)
			case "gateway":
				assert.Equal(t, int32(nvmeofGatewayPort), p.Port)
			case "monitor":
				assert.Equal(t, int32(nvmeofMonitorPort), p.Port)
			case "discovery":
				assert.Equal(t, int32(nvmeofDiscoveryPort), p.Port)
			}
		}
	})

	t.Run("custom ports", func(t *testing.T) {
		nvmeof := &cephv1.CephNVMeOFGateway{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "my-nvmeof",
				Namespace: "rook-ceph-test-ns",
			},
			Spec: cephv1.NVMeOFGatewaySpec{
				Image:     "quay.io/ceph/nvmeof:1.5",
				Pool:      "nvmeof",
				Group:     "group-a",
				Instances: 1,
				Ports: &cephv1.NVMeOFGatewayPorts{
					IOPort:        14420,
					GatewayPort:   15500,
					MonitorPort:   15499,
					DiscoveryPort: 18009,
				},
			},
		}

		r, configHash := newDeploymentSpecTest(t)

		configMapName := fmt.Sprintf("rook-ceph-nvmeof-%s-config", nvmeof.Name)
		d, err := r.makeDeployment(nvmeof, "0", configMapName, configHash)
		assert.NoError(t, err)

		daemonCont := d.Spec.Template.Spec.Containers[0]
		assertContainerPort(t, daemonCont.Ports, "io", 14420)
		assertContainerPort(t, daemonCont.Ports, "gateway", 15500)
		assertContainerPort(t, daemonCont.Ports, "monitor", 15499)
		assertContainerPort(t, daemonCont.Ports, "discovery", 18009)

		svc := r.generateCephNVMeOFService(nvmeof, "0")
		assertServicePort(t, svc.Spec.Ports, "io", 14420)
		assertServicePort(t, svc.Spec.Ports, "gateway", 15500)
		assertServicePort(t, svc.Spec.Ports, "monitor", 15499)
		assertServicePort(t, svc.Spec.Ports, "discovery", 18009)
	})
}

func assertEnvVar(t *testing.T, envVars []v1.EnvVar, name, value string) {
	t.Helper()
	for _, env := range envVars {
		if env.Name == name {
			assert.Equal(t, value, env.Value)
			return
		}
	}
	assert.Failf(t, "missing env var", "expected %s", name)
}

func assertEnvVarPresent(t *testing.T, envVars []v1.EnvVar, name string) {
	t.Helper()
	for _, env := range envVars {
		if env.Name == name {
			assert.NotNil(t, env.ValueFrom)
			return
		}
	}
	assert.Failf(t, "missing env var", "expected %s", name)
}

func assertServicePort(t *testing.T, ports []v1.ServicePort, name string, value int32) {
	t.Helper()
	for _, port := range ports {
		if port.Name == name {
			assert.Equal(t, value, port.Port)
			return
		}
	}
	assert.Failf(t, "missing port", "expected %s", name)
}

func assertContainerPort(t *testing.T, ports []v1.ContainerPort, name string, value int32) {
	t.Helper()
	for _, port := range ports {
		if port.Name == name {
			assert.Equal(t, value, port.ContainerPort)
			return
		}
	}
	assert.Failf(t, "missing port", "expected %s", name)
}
