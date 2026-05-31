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

package mgr

import (
	"testing"

	cephv1 "github.com/rook/rook/pkg/apis/ceph.rook.io/v1"
	"github.com/rook/rook/pkg/clusterd"
	cephclient "github.com/rook/rook/pkg/daemon/ceph/client"
	"github.com/rook/rook/pkg/operator/ceph/config"
	"github.com/rook/rook/pkg/operator/ceph/test"
	optest "github.com/rook/rook/pkg/operator/test"
	"github.com/stretchr/testify/assert"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
)

func TestPodSpec(t *testing.T) {
	clientset := optest.New(t, 1)
	ownerInfo := cephclient.NewMinimumOwnerInfoWithOwnerRef()
	clusterInfo := &cephclient.ClusterInfo{Namespace: "ns", FSID: "myfsid", OwnerInfo: ownerInfo}
	clusterInfo.SetName("test")
	clusterSpec := cephv1.ClusterSpec{
		CephVersion:        cephv1.CephVersionSpec{Image: "quay.io/ceph/ceph:myceph"},
		Dashboard:          cephv1.DashboardSpec{Port: 1234},
		PriorityClassNames: map[cephv1.KeyType]string{cephv1.KeyMgr: "my-priority-class"},
		DataDirHostPath:    "/var/lib/rook/",
		Resources: cephv1.ResourceSpec{
			string(cephv1.KeyMgr): v1.ResourceRequirements{
				Limits: v1.ResourceList{
					v1.ResourceCPU:    *resource.NewQuantity(200.0, resource.BinarySI),
					v1.ResourceMemory: *resource.NewQuantity(500.0, resource.BinarySI),
				},
				Requests: v1.ResourceList{
					v1.ResourceCPU:    *resource.NewQuantity(100.0, resource.BinarySI),
					v1.ResourceMemory: *resource.NewQuantity(250.0, resource.BinarySI),
				},
			},
		},
	}
	c := New(&clusterd.Context{Clientset: clientset}, clusterInfo, clusterSpec, "rook/rook:myversion")

	mgrTestConfig := mgrConfig{
		DaemonID:     "a",
		ResourceName: "rook-ceph-mgr-a",
		DataPathMap:  config.NewStatelessDaemonDataPathMap(config.MgrType, "a", "rook-ceph", "/var/lib/rook/"),
	}

	t.Run("traditional deployment", func(t *testing.T) {
		d, err := c.makeDeployment(&mgrTestConfig)
		assert.NoError(t, err)

		// Deployment should have Ceph labels
		test.AssertLabelsContainCephRequirements(t, d.ObjectMeta.Labels,
			config.MgrType, "a", AppName, "ns", "test", "cephclusters.ceph.rook.io", "ceph-mgr")

		podTemplate := test.NewPodTemplateSpecTester(t, &d.Spec.Template)
		podTemplate.Spec().Containers().RequireAdditionalEnvVars(
			"CEPH_ARGS",
			"ROOK_OPERATOR_NAMESPACE", "ROOK_CEPH_CLUSTER_CRD_VERSION",
			"ROOK_CEPH_CLUSTER_CRD_NAME")
		podTemplate.RunFullSuite(config.MgrType, "a", AppName, "ns", "quay.io/ceph/ceph:myceph",
			"200", "100", "500", "250", /* resources */
			"my-priority-class", "test", "cephclusters.ceph.rook.io", "ceph-mgr")
		assert.Equal(t, 2, len(d.Spec.Template.Annotations))
		assert.Equal(t, 1, len(d.Spec.Template.Spec.Containers))
		assert.Equal(t, 6, len(d.Spec.Template.Spec.Containers[0].VolumeMounts))
	})

	t.Run("deployment with multus with new sidecar proxy command container", func(t *testing.T) {
		c.spec.Network.Provider = "multus"
		d, err := c.makeDeployment(&mgrTestConfig)
		assert.NoError(t, err)
		assert.Equal(t, 3, len(d.Spec.Template.Annotations))                                                                                                                              // Multus annotations
		assert.Equal(t, 2, len(d.Spec.Template.Spec.Containers))                                                                                                                          // mgr pod + sidecar
		assert.Equal(t, cephclient.CommandProxyInitContainerName, d.Spec.Template.Spec.Containers[1].Name)                                                                                // sidecar pod
		assert.Equal(t, 7, len(d.Spec.Template.Spec.Containers[1].VolumeMounts))                                                                                                          // + admin keyring
		assert.Equal(t, "CEPH_ARGS", d.Spec.Template.Spec.Containers[1].Env[len(d.Spec.Template.Spec.Containers[1].Env)-1].Name)                                                          // connection info to the cluster
		assert.Equal(t, "-m $(ROOK_CEPH_MON_HOST) -k /etc/ceph/admin-keyring-store/keyring", d.Spec.Template.Spec.Containers[1].Env[len(d.Spec.Template.Spec.Containers[1].Env)-1].Value) // connection info to the cluster
	})

	t.Run(("check mgr ConfigureProbe"), func(t *testing.T) {
		c.spec.HealthCheck.StartupProbe = make(map[cephv1.KeyType]*cephv1.ProbeSpec)
		c.spec.HealthCheck.StartupProbe[cephv1.KeyMgr] = &cephv1.ProbeSpec{Disabled: false, Probe: &v1.Probe{InitialDelaySeconds: 1000}}
		c.spec.HealthCheck.LivenessProbe = make(map[cephv1.KeyType]*cephv1.ProbeSpec)
		c.spec.HealthCheck.LivenessProbe[cephv1.KeyMgr] = &cephv1.ProbeSpec{Disabled: false, Probe: &v1.Probe{InitialDelaySeconds: 900}}
		container := c.makeMgrDaemonContainer(&mgrTestConfig)
		assert.NotNil(t, container.LivenessProbe)
		assert.NotNil(t, container.StartupProbe)
		assert.Equal(t, int32(900), container.LivenessProbe.InitialDelaySeconds)
		assert.Equal(t, int32(1000), container.StartupProbe.InitialDelaySeconds)
	})
}

func TestServiceSpec(t *testing.T) {
	clientset := optest.New(t, 1)
	ownerInfo := cephclient.NewMinimumOwnerInfoWithOwnerRef()
	clusterInfo := &cephclient.ClusterInfo{Namespace: "ns", FSID: "myfsid", OwnerInfo: ownerInfo}
	clusterSpec := cephv1.ClusterSpec{}
	c := New(&clusterd.Context{Clientset: clientset}, clusterInfo, clusterSpec, "myversion")

	s, err := c.MakeMetricsService("rook-mgr", serviceMetricName)
	assert.NoError(t, err)
	assert.NotNil(t, s)
	assert.Equal(t, "rook-mgr", s.Name)
	assert.Equal(t, 1, len(s.Spec.Ports))
	assert.Equal(t, 2, len(s.Labels))
	// Check service selector
	assert.Equal(t, 3, len(s.Spec.Selector))
	assert.Equal(t, s.Spec.Selector["app"], "rook-ceph-mgr")
	assert.Equal(t, s.Spec.Selector["mgr_role"], "active")
	assert.Equal(t, s.Spec.Selector["rook_cluster"], "ns")
}

func TestHostNetwork(t *testing.T) {
	clientset := optest.New(t, 1)
	ownerInfo := cephclient.NewMinimumOwnerInfoWithOwnerRef()
	clusterInfo := &cephclient.ClusterInfo{Namespace: "ns", FSID: "myfsid", OwnerInfo: ownerInfo}
	clusterInfo.SetName("test")
	clusterSpec := cephv1.ClusterSpec{
		Network:         cephv1.NetworkSpec{HostNetwork: true},
		Dashboard:       cephv1.DashboardSpec{Port: 1234},
		DataDirHostPath: "/var/lib/rook/",
	}
	c := New(&clusterd.Context{Clientset: clientset}, clusterInfo, clusterSpec, "myversion")

	mgrTestConfig := mgrConfig{
		DaemonID:     "a",
		ResourceName: "mgr-a",
		DataPathMap:  config.NewStatelessDaemonDataPathMap(config.MgrType, "a", "rook-ceph", "/var/lib/rook/"),
	}

	d, err := c.makeDeployment(&mgrTestConfig)
	assert.NoError(t, err)
	assert.NotNil(t, d)

	assert.Equal(t, true, c.spec.Network.IsHost())
	assert.Equal(t, v1.DNSClusterFirstWithHostNet, d.Spec.Template.Spec.DNSPolicy)
}

func TestApplyPrometheusAnnotations(t *testing.T) {
	clientset := optest.New(t, 1)
	clusterSpec := cephv1.ClusterSpec{
		DataDirHostPath: "/var/lib/rook/",
	}
	ownerInfo := cephclient.NewMinimumOwnerInfoWithOwnerRef()
	clusterInfo := &cephclient.ClusterInfo{Namespace: "ns", FSID: "myfsid", OwnerInfo: ownerInfo}
	clusterInfo.SetName("test")
	c := New(&clusterd.Context{Clientset: clientset}, clusterInfo, clusterSpec, "myversion")

	mgrTestConfig := mgrConfig{
		DaemonID:     "a",
		ResourceName: "rook-ceph-mgr-a",
		DataPathMap:  config.NewStatelessDaemonDataPathMap(config.MgrType, "a", "rook-ceph", "/var/lib/rook/"),
	}

	d, err := c.makeDeployment(&mgrTestConfig)
	assert.NoError(t, err)

	// Test without annotations
	c.applyPrometheusAnnotations(&d.ObjectMeta)
	assert.Equal(t, 2, len(d.ObjectMeta.Annotations))

	// Test with existing annotations
	// applyPrometheusAnnotations() shouldn't do anything
	// re-initialize "d"
	d, err = c.makeDeployment(&mgrTestConfig)
	assert.NoError(t, err)

	fakeAnnotations := cephv1.Annotations{
		"foo.io/bar": "foobar",
	}
	c.spec.Annotations = map[cephv1.KeyType]cephv1.Annotations{cephv1.KeyMgr: fakeAnnotations}

	c.applyPrometheusAnnotations(&d.ObjectMeta)
	assert.Equal(t, 1, len(c.spec.Annotations))
	assert.Equal(t, 0, len(d.ObjectMeta.Annotations))
}

func TestMgrNetwork(t *testing.T) {
	clientset := optest.New(t, 1)
	ownerInfo := cephclient.NewMinimumOwnerInfoWithOwnerRef()
	clusterInfo := &cephclient.ClusterInfo{Namespace: "ns", FSID: "myfsid", OwnerInfo: ownerInfo}
	clusterInfo.SetName("test")
	ptrToFalse := false
	ptrToTrue := true

	mgrTestConfig := mgrConfig{
		DaemonID:     "a",
		ResourceName: "mgr-a",
		DataPathMap:  config.NewStatelessDaemonDataPathMap(config.MgrType, "a", "rook-ceph", "/var/lib/rook/"),
	}

	t.Run("cluster HostNetwork true, manager Hostnetwork false", func(t *testing.T) {
		clusterSpec := cephv1.ClusterSpec{
			Network: cephv1.NetworkSpec{HostNetwork: true},
			Mgr:     cephv1.MgrSpec{HostNetwork: &ptrToFalse},
		}
		c := New(&clusterd.Context{Clientset: clientset}, clusterInfo, clusterSpec, "myversion")

		d, err := c.makeDeployment(&mgrTestConfig)
		assert.NoError(t, err)
		assert.NotNil(t, d)

		assert.Equal(t, false, d.Spec.Template.Spec.HostNetwork)
	})

	t.Run("cluster hostNetwork true, manager hostnetwork true", func(t *testing.T) {
		clusterSpec := cephv1.ClusterSpec{
			Network: cephv1.NetworkSpec{HostNetwork: true},
			Mgr:     cephv1.MgrSpec{HostNetwork: &ptrToTrue},
		}
		c := New(&clusterd.Context{Clientset: clientset}, clusterInfo, clusterSpec, "myversion")

		d, err := c.makeDeployment(&mgrTestConfig)
		assert.NoError(t, err)
		assert.NotNil(t, d)

		assert.Equal(t, true, d.Spec.Template.Spec.HostNetwork)
	})
	t.Run("cluster hostNetwork false, manager hostnetwork empty", func(t *testing.T) {
		clusterSpec := cephv1.ClusterSpec{
			Network: cephv1.NetworkSpec{HostNetwork: false},
			// Mgr:     cephv1.MgrSpec{HostNetwork: &ptrToTrue},
		}
		c := New(&clusterd.Context{Clientset: clientset}, clusterInfo, clusterSpec, "myversion")

		d, err := c.makeDeployment(&mgrTestConfig)
		assert.NoError(t, err)
		assert.NotNil(t, d)

		assert.Equal(t, false, d.Spec.Template.Spec.HostNetwork)
	})
	t.Run("cluster hostNetwork true manager hostnetwork empty", func(t *testing.T) {
		clusterSpec := cephv1.ClusterSpec{
			Network: cephv1.NetworkSpec{HostNetwork: true},
			// Mgr:     cephv1.MgrSpec{HostNetwork: &ptrToTrue},
		}
		c := New(&clusterd.Context{Clientset: clientset}, clusterInfo, clusterSpec, "myversion")

		d, err := c.makeDeployment(&mgrTestConfig)
		assert.NoError(t, err)
		assert.NotNil(t, d)

		assert.Equal(t, true, d.Spec.Template.Spec.HostNetwork)
	})
}
