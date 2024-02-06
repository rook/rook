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

package nfs

import (
	"testing"

	cephv1 "github.com/rook/rook/pkg/apis/ceph.rook.io/v1"
	rookclient "github.com/rook/rook/pkg/client/clientset/versioned/fake"
	"github.com/rook/rook/pkg/client/clientset/versioned/scheme"
	"github.com/rook/rook/pkg/clusterd"
	cephclient "github.com/rook/rook/pkg/daemon/ceph/client"
	"github.com/rook/rook/pkg/operator/ceph/config"
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

func newDeploymentSpecTest(t *testing.T) (*ReconcileCephNFS, daemonConfig) {
	clientset := optest.New(t, 1)
	c := &clusterd.Context{
		Executor:      &exectest.MockExecutor{},
		RookClientset: rookclient.NewSimpleClientset(),
		Clientset:     clientset,
	}

	s := scheme.Scheme
	object := []runtime.Object{&cephv1.CephNFS{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
		TypeMeta: controllerTypeMeta,
		Spec: cephv1.NFSGaneshaSpec{
			RADOS: cephv1.GaneshaRADOSSpec{
				Pool:      "foo",
				Namespace: namespace,
			},
			Server: cephv1.GaneshaServerSpec{
				Active: 1,
			},
		},
	},
	}
	cl := fake.NewClientBuilder().WithScheme(s).WithRuntimeObjects(object...).Build()

	r := &ReconcileCephNFS{
		client:  cl,
		scheme:  scheme.Scheme,
		context: c,
		clusterInfo: &cephclient.ClusterInfo{
			FSID:        "myfsid",
			CephVersion: cephver.Quincy,
		},
		cephClusterSpec: &cephv1.ClusterSpec{
			CephVersion: cephv1.CephVersionSpec{
				Image: "quay.io/ceph/ceph:v17",
			},
		},
	}

	id := "i"
	configName := "rook-ceph-nfs-my-nfs-i"
	cfg := daemonConfig{
		ID:                  id,
		ConfigConfigMap:     configName,
		ConfigConfigMapHash: "dcb0d2f5f5e86ec4929d8243cd640b8154165f8ff9b89809964fc7993e9b0101",
		DataPathMap: &config.DataPathMap{
			HostDataDir:        "",                          // nfs daemon does not store data on host, ...
			ContainerDataDir:   cephclient.DefaultConfigDir, // does share data in containers using emptyDir, ...
			HostLogAndCrashDir: "",                          // and does not log to /var/log/ceph dir nor creates crash dumps
		},
	}

	return r, cfg
}

func TestDeploymentSpec(t *testing.T) {
	t.Run("basic", func(t *testing.T) {
		nfs := &cephv1.CephNFS{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "my-nfs",
				Namespace: "rook-ceph-test-ns",
			},
			Spec: cephv1.NFSGaneshaSpec{
				RADOS: cephv1.GaneshaRADOSSpec{
					Pool:      "myfs-data0",
					Namespace: "nfs-test-ns",
				},
				Server: cephv1.GaneshaServerSpec{
					Active: 3,
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
			},
		}

		r, cfg := newDeploymentSpecTest(t)

		d, err := r.makeDeployment(nfs, cfg)
		assert.NoError(t, err)
		assert.NotEmpty(t, d.Spec.Template.Annotations)
		assert.Equal(t, "dcb0d2f5f5e86ec4929d8243cd640b8154165f8ff9b89809964fc7993e9b0101", d.Spec.Template.Annotations["config-hash"])

		// Deployment should have Ceph labels
		optest.AssertLabelsContainRookRequirements(t, d.ObjectMeta.Labels, AppName)

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
	})

	t.Run("with sssd sidecar", func(t *testing.T) {
		nfs := &cephv1.CephNFS{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "my-nfs",
				Namespace: "rook-ceph-test-ns",
			},
			Spec: cephv1.NFSGaneshaSpec{
				Server: cephv1.GaneshaServerSpec{
					Active: 3,
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
				},
				Security: &cephv1.NFSSecuritySpec{
					SSSD: &cephv1.SSSDSpec{
						Sidecar: &cephv1.SSSDSidecar{
							Image:      "quay.io/sssd/sssd:latest",
							DebugLevel: 6,
							SSSDConfigFile: cephv1.SSSDSidecarConfigFile{
								VolumeSource: &cephv1.ConfigFileVolumeSource{
									ConfigMap: &v1.ConfigMapVolumeSource{},
								},
							},
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
						},
					},
				},
			},
		}

		r, cfg := newDeploymentSpecTest(t)

		d, err := r.makeDeployment(nfs, cfg)
		assert.NoError(t, err)
		assert.NotEmpty(t, d.Spec.Template.Annotations)
		assert.Equal(t, "dcb0d2f5f5e86ec4929d8243cd640b8154165f8ff9b89809964fc7993e9b0101", d.Spec.Template.Annotations["config-hash"])

		// Deployment should have Ceph labels
		optest.AssertLabelsContainRookRequirements(t, d.ObjectMeta.Labels, AppName)

		// this also ensures that the things added for the SSSD sidecar don't include duplicate
		// volumes/mounts and that all volumes/mounts have a corresponding mount/volume
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

		initNames := []string{}
		for _, init := range d.Spec.Template.Spec.InitContainers {
			initNames = append(initNames, init.Name)
		}
		assert.Contains(t, initNames, "generate-nsswitch-conf")
		assert.Contains(t, initNames, "copy-sssd-sockets")

		contNames := []string{}
		for _, cont := range d.Spec.Template.Spec.Containers {
			contNames = append(contNames, cont.Name)
		}
		assert.Contains(t, contNames, "sssd")
	})

	t.Run("with kerberos", func(t *testing.T) {
		nfs := &cephv1.CephNFS{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "my-nfs",
				Namespace: "rook-ceph-test-ns",
			},
			Spec: cephv1.NFSGaneshaSpec{
				Server: cephv1.GaneshaServerSpec{
					Active: 3,
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
				},
				Security: &cephv1.NFSSecuritySpec{
					Kerberos: &cephv1.KerberosSpec{
						ConfigFiles: cephv1.KerberosConfigFiles{
							VolumeSource: &cephv1.ConfigFileVolumeSource{
								ConfigMap: &v1.ConfigMapVolumeSource{},
							},
						},
						KeytabFile: cephv1.KerberosKeytabFile{
							VolumeSource: &cephv1.ConfigFileVolumeSource{
								Secret: &v1.SecretVolumeSource{},
							},
						},
					},
				},
			},
		}

		r, cfg := newDeploymentSpecTest(t)

		d, err := r.makeDeployment(nfs, cfg)
		assert.NoError(t, err)
		assert.NotEmpty(t, d.Spec.Template.Annotations)
		assert.Equal(t, "dcb0d2f5f5e86ec4929d8243cd640b8154165f8ff9b89809964fc7993e9b0101", d.Spec.Template.Annotations["config-hash"])

		// Deployment should have Ceph labels
		optest.AssertLabelsContainRookRequirements(t, d.ObjectMeta.Labels, AppName)

		// this also ensures that the things added for the Kerberos don't include duplicate
		// volumes/mounts and that all volumes/mounts have a corresponding mount/volume
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

		initNames := []string{}
		for _, init := range d.Spec.Template.Spec.InitContainers {
			initNames = append(initNames, init.Name)
		}
		assert.Contains(t, initNames, "generate-krb5-conf")

		contNames := []string{}
		for _, cont := range d.Spec.Template.Spec.Containers {
			contNames = append(contNames, cont.Name)
		}
		assert.NotContains(t, contNames, "sssd")
	})

	t.Run("with sssd sidecar and kerberos", func(t *testing.T) {
		nfs := &cephv1.CephNFS{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "my-nfs",
				Namespace: "rook-ceph-test-ns",
			},
			Spec: cephv1.NFSGaneshaSpec{
				RADOS: cephv1.GaneshaRADOSSpec{
					Pool:      "myfs-data0",
					Namespace: "nfs-test-ns",
				},
				Server: cephv1.GaneshaServerSpec{
					Active: 3,
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
				},
				Security: &cephv1.NFSSecuritySpec{
					Kerberos: &cephv1.KerberosSpec{
						ConfigFiles: cephv1.KerberosConfigFiles{
							VolumeSource: &cephv1.ConfigFileVolumeSource{
								ConfigMap: &v1.ConfigMapVolumeSource{},
							},
						},
						KeytabFile: cephv1.KerberosKeytabFile{
							VolumeSource: &cephv1.ConfigFileVolumeSource{
								Secret: &v1.SecretVolumeSource{},
							},
						},
					},
					SSSD: &cephv1.SSSDSpec{
						Sidecar: &cephv1.SSSDSidecar{
							Image:      "quay.io/sssd/sssd:latest",
							DebugLevel: 6,
							SSSDConfigFile: cephv1.SSSDSidecarConfigFile{
								VolumeSource: &cephv1.ConfigFileVolumeSource{
									ConfigMap: &v1.ConfigMapVolumeSource{},
								},
							},
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
						},
					},
				},
			},
		}

		r, cfg := newDeploymentSpecTest(t)

		d, err := r.makeDeployment(nfs, cfg)
		assert.NoError(t, err)
		assert.NotEmpty(t, d.Spec.Template.Annotations)
		assert.Equal(t, "dcb0d2f5f5e86ec4929d8243cd640b8154165f8ff9b89809964fc7993e9b0101", d.Spec.Template.Annotations["config-hash"])

		// Deployment should have Ceph labels
		optest.AssertLabelsContainRookRequirements(t, d.ObjectMeta.Labels, AppName)

		// this also ensures that the things added for the SSSD sidecar don't include duplicate
		// volumes/mounts and that all volumes/mounts have a corresponding mount/volume
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

		initNames := []string{}
		for _, init := range d.Spec.Template.Spec.InitContainers {
			initNames = append(initNames, init.Name)
		}
		assert.Contains(t, initNames, "generate-nsswitch-conf")
		assert.Contains(t, initNames, "copy-sssd-sockets")
		assert.Contains(t, initNames, "generate-krb5-conf")

		contNames := []string{}
		for _, cont := range d.Spec.Template.Spec.Containers {
			contNames = append(contNames, cont.Name)
		}
		assert.Contains(t, contNames, "sssd")
	})

	t.Run("basic with default liveness-probe", func(t *testing.T) {
		nfs := &cephv1.CephNFS{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "my-nfs",
				Namespace: "rook-ceph-test-ns",
			},
			Spec: cephv1.NFSGaneshaSpec{
				RADOS: cephv1.GaneshaRADOSSpec{
					Pool:      "myfs-data0",
					Namespace: "nfs-test-ns",
				},
				Server: cephv1.GaneshaServerSpec{
					Active: 3,
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
			},
		}

		r, cfg := newDeploymentSpecTest(t)
		d, err := r.makeDeployment(nfs, cfg)
		assert.NoError(t, err)
		assert.NotEmpty(t, d.Spec.Template.Annotations)

		// Expects valid settings to default livness-probe
		var ganeshaCont *v1.Container = nil
		for i := range d.Spec.Template.Spec.Containers {
			if d.Spec.Template.Spec.Containers[i].Name == "nfs-ganesha" {
				ganeshaCont = &d.Spec.Template.Spec.Containers[i]
				break
			}
		}
		assert.NotNil(t, ganeshaCont)
		assert.NotNil(t, ganeshaCont.LivenessProbe)
		assert.Equal(t, ganeshaCont.LivenessProbe.InitialDelaySeconds, int32(10))
		assert.Equal(t, ganeshaCont.LivenessProbe.FailureThreshold, int32(10))
		assert.GreaterOrEqual(t, ganeshaCont.LivenessProbe.TimeoutSeconds, int32(5))
	})
}
