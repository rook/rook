/*
Copyright 2022 The Rook Authors. All rights reserved.

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
	cephclient "github.com/rook/rook/pkg/daemon/ceph/client"
	cephver "github.com/rook/rook/pkg/operator/ceph/version"
	optest "github.com/rook/rook/pkg/operator/test"
	"github.com/stretchr/testify/assert"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func baseCephNFS(name, namespace string) *cephv1.CephNFS {
	return &cephv1.CephNFS{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
		Spec: cephv1.NFSGaneshaSpec{
			Server: cephv1.GaneshaServerSpec{
				Active: 2,
			},
		},
	}
}

func mockReconcile() *ReconcileCephNFS {
	return &ReconcileCephNFS{
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
}

func mockPodSpec() *v1.PodSpec {
	return &v1.PodSpec{
		InitContainers: []v1.Container{
			{
				Name: "generate-minimal-ceph-conf",
			},
		},
		Containers: []v1.Container{
			{
				Name: "nfs-ganesha",
				VolumeMounts: []v1.VolumeMount{
					{Name: "dbus-socket"},
				},
			},
			{
				Name: "dbus-daemon",
				VolumeMounts: []v1.VolumeMount{
					{Name: "dbus-socket"},
				},
			},
		},
		Volumes: []v1.Volume{
			{Name: "dbus-socket"},
		},
	}
}

// basic tests to ensure the mockPodSpec() details haven't been clobbered
func mockPodSpecValuesUnchanged(t *testing.T, p *v1.PodSpec) {
	t.Helper()

	assert.Equal(t, "generate-minimal-ceph-conf", p.InitContainers[0].Name)

	ganesha := p.Containers[0]
	assert.Equal(t, "nfs-ganesha", ganesha.Name)
	assert.Equal(t, "dbus-socket", ganesha.VolumeMounts[0].Name)

	dbus := p.Containers[1]
	assert.Equal(t, "dbus-daemon", dbus.Name)
	assert.Equal(t, "dbus-socket", dbus.VolumeMounts[0].Name)

	assert.Equal(t, "dbus-socket", p.Volumes[0].Name)
}

func TestReconcileCephNFS_addSecurityConfigsToPod(t *testing.T) {
	name := "my-nfs"
	namespace := "rook-ceph"

	t.Run("security spec unset (nil) adds nothing", func(t *testing.T) {
		nfs := baseCephNFS(name, namespace)

		pod := &v1.PodSpec{}
		err := mockReconcile().addSecurityConfigsToPod(nfs, pod)

		assert.NoError(t, err)
		assert.Empty(t, pod)
	})

	// allow users to specify they want no security by setting 'security: {}'
	t.Run("security spec empty adds nothing", func(t *testing.T) {
		nfs := baseCephNFS(name, namespace)
		nfs.Spec.Security = &cephv1.NFSSecuritySpec{}

		pod := &v1.PodSpec{}
		err := mockReconcile().addSecurityConfigsToPod(nfs, pod)

		assert.NoError(t, err)
		assert.Empty(t, pod)
	})

	t.Run("security.sssd.sidecar with configMap", func(t *testing.T) {
		// this test is long but essentially makes sure that the sssd sidecar is added, including
		// any additional init containers needed, verifies that the right container images are used
		// for the right containers, and verifies the right resources are applied. Also ensures the
		// expected volume mounts are on the right containers. And that the original pod details
		// remain intact.

		nfs := baseCephNFS(name, namespace)
		nfs.Spec.Server.Resources = v1.ResourceRequirements{
			Limits: v1.ResourceList{
				v1.ResourceCPU:    *resource.NewQuantity(3000.0, resource.BinarySI),
				v1.ResourceMemory: *resource.NewQuantity(8192.0, resource.BinarySI),
			},
			Requests: v1.ResourceList{
				v1.ResourceCPU:    *resource.NewQuantity(3000.0, resource.BinarySI),
				v1.ResourceMemory: *resource.NewQuantity(8192.0, resource.BinarySI),
			},
		}
		nfs.Spec.Security = &cephv1.NFSSecuritySpec{
			SSSD: &cephv1.SSSDSpec{
				Sidecar: &cephv1.SSSDSidecar{
					Image: "my-image",
					SSSDConfigFile: cephv1.SSSDSidecarConfigFile{
						VolumeSource: &cephv1.ConfigFileVolumeSource{
							ConfigMap: &v1.ConfigMapVolumeSource{},
						},
					},
					Resources: v1.ResourceRequirements{
						Limits: v1.ResourceList{
							v1.ResourceCPU:    *resource.NewQuantity(2000.0, resource.BinarySI),
							v1.ResourceMemory: *resource.NewQuantity(1024.0, resource.BinarySI),
						},
						Requests: v1.ResourceList{
							v1.ResourceCPU:    *resource.NewQuantity(1000.0, resource.BinarySI),
							v1.ResourceMemory: *resource.NewQuantity(512.0, resource.BinarySI),
						},
					},
				},
			},
		}

		pod := mockPodSpec()
		err := mockReconcile().addSecurityConfigsToPod(nfs, pod)

		assert.NoError(t, err)
		mockPodSpecValuesUnchanged(t, pod)

		assert.ElementsMatch(t, volNames(pod.Volumes), []string{
			"dbus-socket",                                  // pre-existing vol still exists
			"nsswitch-conf",                                // nsswitch changes
			"sssd-sockets", "sssd-mmap-cache", "sssd-conf", // sssd container changes
		})

		assert.Len(t, pod.InitContainers, 3) // 2 init containers added (nss and copySockets)
		assert.Len(t, pod.Containers, 3)     // 1 sidecar added (sssd)

		// pre-existing init container still exists
		genConf := containerByName(pod.InitContainers, "generate-minimal-ceph-conf")
		assert.NotEmpty(t, genConf)

		// generate-nsswitch-conf init container
		nss := containerByName(pod.InitContainers, "generate-nsswitch-conf")
		assert.NotEmpty(t, nss)
		// container should have CLUSTER image and resources from SERVER spec
		assert.Equal(t, "quay.io/ceph/ceph:v17", nss.Image)
		nssTester := optest.NewContainersSpecTester(t, []v1.Container{nss})
		nssTester.AssertResourceSpec(optest.ResourceLimitExpectations{
			CPUResourceLimit:      "3000",
			MemoryResourceLimit:   "8Ki",
			CPUResourceRequest:    "3000",
			MemoryResourceRequest: "8Ki",
		})
		assert.ElementsMatch(t, volMountNames(nss.VolumeMounts), []string{
			"nsswitch-conf",
		})

		// copy-sssd-sockets init container
		copySockets := containerByName(pod.InitContainers, "copy-sssd-sockets")
		assert.NotEmpty(t, copySockets)
		assert.ElementsMatch(t, volMountNames(copySockets.VolumeMounts), []string{
			"sssd-sockets",
		})
		// image should be from sidecar spec
		assert.Equal(t, "my-image", copySockets.Image)

		// nfs-ganesha main container
		ganesha := containerByName(pod.Containers, "nfs-ganesha")
		assert.NotEmpty(t, ganesha)
		assert.ElementsMatch(t, volMountNames(ganesha.VolumeMounts), []string{
			"dbus-socket",                     // pre-existing vol still exists
			"nsswitch-conf",                   // nsswitch.conf override
			"sssd-sockets", "sssd-mmap-cache", // shared with sssd sidecar
		})

		// dbus-daemon sidecar
		dbus := containerByName(pod.Containers, "dbus-daemon")
		assert.NotEmpty(t, dbus)
		assert.ElementsMatch(t, volMountNames(dbus.VolumeMounts), []string{
			"dbus-socket", // only the pre-existing mount exists
		})

		// sssd sidecar
		sssd := containerByName(pod.Containers, "sssd")
		assert.NotEmpty(t, sssd)
		assert.ElementsMatch(t, volMountNames(sssd.VolumeMounts), []string{
			"sssd-sockets", "sssd-mmap-cache", // shared with nfs-ganesha container
			"sssd-conf", // conf for SSSD
		})
		assert.Equal(t, "my-image", sssd.Image)

		// the sssd sidecar and its init container should have resources from the sidecar spec
		sssdTester := optest.NewContainersSpecTester(t, []v1.Container{sssd, copySockets})
		sssdTester.AssertResourceSpec(optest.ResourceLimitExpectations{
			CPUResourceLimit:      "2000",
			MemoryResourceLimit:   "1Ki",
			CPUResourceRequest:    "1k",
			MemoryResourceRequest: "512",
		})
	})

	t.Run("security.sssd.sidecar with no config", func(t *testing.T) {
		// this test is long but essentially makes sure that the sssd sidecar is added, including
		// any additional init containers needed, verifies that the right container images are used
		// for the right containers, and verifies the right resources are applied. Also ensures the
		// expected volume mounts are on the right containers. And that the original pod details
		// remain intact.

		nfs := baseCephNFS(name, namespace)
		nfs.Spec.Server.Resources = v1.ResourceRequirements{
			Limits: v1.ResourceList{
				v1.ResourceCPU:    *resource.NewQuantity(3000.0, resource.BinarySI),
				v1.ResourceMemory: *resource.NewQuantity(8192.0, resource.BinarySI),
			},
			Requests: v1.ResourceList{
				v1.ResourceCPU:    *resource.NewQuantity(3000.0, resource.BinarySI),
				v1.ResourceMemory: *resource.NewQuantity(8192.0, resource.BinarySI),
			},
		}
		nfs.Spec.Security = &cephv1.NFSSecuritySpec{
			SSSD: &cephv1.SSSDSpec{
				Sidecar: &cephv1.SSSDSidecar{
					Image:          "my-image",
					SSSDConfigFile: cephv1.SSSDSidecarConfigFile{},
					Resources: v1.ResourceRequirements{
						Limits: v1.ResourceList{
							v1.ResourceCPU:    *resource.NewQuantity(2000.0, resource.BinarySI),
							v1.ResourceMemory: *resource.NewQuantity(1024.0, resource.BinarySI),
						},
						Requests: v1.ResourceList{
							v1.ResourceCPU:    *resource.NewQuantity(1000.0, resource.BinarySI),
							v1.ResourceMemory: *resource.NewQuantity(512.0, resource.BinarySI),
						},
					},
					DebugLevel: 6, // also verify debug level
				},
			},
		}

		pod := mockPodSpec()
		err := mockReconcile().addSecurityConfigsToPod(nfs, pod)

		assert.NoError(t, err)
		mockPodSpecValuesUnchanged(t, pod)

		assert.ElementsMatch(t, volNames(pod.Volumes), []string{
			"dbus-socket",                     // pre-existing vol still exists
			"nsswitch-conf",                   // nsswitch changes
			"sssd-sockets", "sssd-mmap-cache", // sssd container changes
		})

		assert.Len(t, pod.InitContainers, 3) // 2 init containers added (nss and copySockets)
		assert.Len(t, pod.Containers, 3)     // 1 sidecar added (sssd)

		// pre-existing init container still exists
		genConf := containerByName(pod.InitContainers, "generate-minimal-ceph-conf")
		assert.NotEmpty(t, genConf)

		// generate-nsswitch-conf init container
		nss := containerByName(pod.InitContainers, "generate-nsswitch-conf")
		assert.NotEmpty(t, nss)
		// container should have CLUSTER image and resources from SERVER spec
		assert.Equal(t, "quay.io/ceph/ceph:v17", nss.Image)
		nssTester := optest.NewContainersSpecTester(t, []v1.Container{nss})
		nssTester.AssertResourceSpec(optest.ResourceLimitExpectations{
			CPUResourceLimit:      "3000",
			MemoryResourceLimit:   "8Ki",
			CPUResourceRequest:    "3000",
			MemoryResourceRequest: "8Ki",
		})
		assert.ElementsMatch(t, volMountNames(nss.VolumeMounts), []string{
			"nsswitch-conf",
		})

		// copy-sssd-sockets init container
		copySockets := containerByName(pod.InitContainers, "copy-sssd-sockets")
		assert.NotEmpty(t, copySockets)
		assert.ElementsMatch(t, volMountNames(copySockets.VolumeMounts), []string{
			"sssd-sockets",
		})
		// image should be from sidecar spec
		assert.Equal(t, "my-image", copySockets.Image)

		// nfs-ganesha main container
		ganesha := containerByName(pod.Containers, "nfs-ganesha")
		assert.NotEmpty(t, ganesha)
		assert.ElementsMatch(t, volMountNames(ganesha.VolumeMounts), []string{
			"dbus-socket",                     // pre-existing vol still exists
			"nsswitch-conf",                   // nsswitch.conf override
			"sssd-sockets", "sssd-mmap-cache", // shared with sssd sidecar
		})

		// dbus-daemon sidecar
		dbus := containerByName(pod.Containers, "dbus-daemon")
		assert.NotEmpty(t, dbus)
		assert.ElementsMatch(t, volMountNames(dbus.VolumeMounts), []string{
			"dbus-socket", // only the pre-existing mount exists
		})

		// sssd sidecar
		sssd := containerByName(pod.Containers, "sssd")
		assert.NotEmpty(t, sssd)
		assert.ElementsMatch(t, volMountNames(sssd.VolumeMounts), []string{
			"sssd-sockets", "sssd-mmap-cache", // shared with nfs-ganesha container
		})
		assert.Equal(t, "my-image", sssd.Image)
		assert.Contains(t, sssd.Args, "--debug-level=6") // make sure debug-level flag is set

		// the sssd sidecar and its init container should have resources from the sidecar spec
		sssdTester := optest.NewContainersSpecTester(t, []v1.Container{sssd, copySockets})
		sssdTester.AssertResourceSpec(optest.ResourceLimitExpectations{
			CPUResourceLimit:      "2000",
			MemoryResourceLimit:   "1Ki",
			CPUResourceRequest:    "1k",
			MemoryResourceRequest: "512",
		})
	})
}

func volNames(vs []v1.Volume) []string {
	names := []string{}
	for _, vol := range vs {
		names = append(names, vol.Name)
	}
	return names
}

func volMountNames(vms []v1.VolumeMount) []string {
	names := []string{}
	for _, v := range vms {
		names = append(names, v.Name)
	}
	return names
}

func containerByName(cc []v1.Container, name string) v1.Container {
	for _, c := range cc {
		if c.Name == name {
			return c
		}
	}
	return v1.Container{}
}
