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
	"testing"

	cephv1 "github.com/rook/rook/pkg/apis/ceph.rook.io/v1"
	rookclient "github.com/rook/rook/pkg/client/clientset/versioned/fake"
	"github.com/rook/rook/pkg/client/clientset/versioned/scheme"
	"github.com/rook/rook/pkg/clusterd"
	cephclient "github.com/rook/rook/pkg/daemon/ceph/client"
	cephTest "github.com/rook/rook/pkg/operator/ceph/test"
	cephver "github.com/rook/rook/pkg/operator/ceph/version"
	"github.com/rook/rook/pkg/operator/k8sutil"
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
				Pool:      "nvmeofpool",
				Group:     "mygroup",
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
				Pool:      "nvmeofpool",
				Group:     "mygroup",
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
				Pool:      "nvmeofpool",
				Group:     "mygroup",
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
				Pool:      "nvmeofpool",
				Group:     "mygroup",
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
				Pool:         "nvmeofpool",
				Group:        "mygroup",
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
				Pool:      "nvmeofpool",
				Group:     "mygroup",
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

		volumeMountNames := []string{}
		for _, vm := range initCont.VolumeMounts {
			volumeMountNames = append(volumeMountNames, vm.Name)
		}
		assert.Contains(t, volumeMountNames, "ceph-admin-keyring")
		assert.Contains(t, volumeMountNames, "gateway-config")
		assert.Contains(t, volumeMountNames, k8sutil.PathToVolumeName(cephclient.DefaultConfigDir))
	})

	t.Run("daemon container configuration", func(t *testing.T) {
		nvmeof := &cephv1.CephNVMeOFGateway{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "my-nvmeof",
				Namespace: "rook-ceph-test-ns",
			},
			Spec: cephv1.NVMeOFGatewaySpec{
				Pool:      "nvmeofpool",
				Group:     "mygroup",
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
		assert.Equal(t, defaultNVMeOFImage, daemonCont.Image)

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

	t.Run("service generation", func(t *testing.T) {
		nvmeof := &cephv1.CephNVMeOFGateway{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "my-nvmeof",
				Namespace: "rook-ceph-test-ns",
			},
			Spec: cephv1.NVMeOFGatewaySpec{
				Pool:      "nvmeofpool",
				Group:     "mygroup",
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
}
