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

package mds

import (
	"testing"

	"github.com/rook/rook/pkg/operator/ceph/config"
	"github.com/rook/rook/pkg/operator/ceph/controller"
	"github.com/rook/rook/pkg/operator/ceph/test"
	"github.com/rook/rook/pkg/operator/k8sutil"

	cephv1 "github.com/rook/rook/pkg/apis/ceph.rook.io/v1"
	"github.com/rook/rook/pkg/clusterd"
	cephclient "github.com/rook/rook/pkg/daemon/ceph/client"
	cephver "github.com/rook/rook/pkg/operator/ceph/version"
	testop "github.com/rook/rook/pkg/operator/test"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	apps "k8s.io/api/apps/v1"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
)

func testDeploymentObject(t *testing.T, network cephv1.NetworkSpec) (*apps.Deployment, error) {
	return testDeploymentObjectWithMonPort(t, network, 3300)
}

func testDeploymentObjectWithMonPort(t *testing.T, network cephv1.NetworkSpec, monPort int32) (*apps.Deployment, error) {
	fs := cephv1.CephFilesystem{
		ObjectMeta: metav1.ObjectMeta{Name: "myfs", Namespace: "ns"},
		Spec: cephv1.FilesystemSpec{
			MetadataServer: cephv1.MetadataServerSpec{
				ActiveCount:   1,
				ActiveStandby: false,
				Resources: v1.ResourceRequirements{
					Limits: v1.ResourceList{
						v1.ResourceCPU:    *resource.NewQuantity(500.0, resource.BinarySI),
						v1.ResourceMemory: *resource.NewQuantity(4337.0, resource.BinarySI),
					},
					Requests: v1.ResourceList{
						v1.ResourceCPU:    *resource.NewQuantity(250.0, resource.BinarySI),
						v1.ResourceMemory: *resource.NewQuantity(2169.0, resource.BinarySI),
					},
				},
				PriorityClassName: "my-priority-class",
			},
		},
	}
	clusterInfo := &cephclient.ClusterInfo{
		FSID:        "myfsid",
		CephVersion: cephver.Squid,
		InternalMonitors: map[string]*cephclient.MonInfo{
			"a": cephclient.NewMonInfo("a", "10.0.0.1", monPort),
		},
	}
	clientset := testop.New(t, 1)

	c := NewCluster(
		clusterInfo,
		&clusterd.Context{Clientset: clientset},
		&cephv1.ClusterSpec{
			CephVersion:     cephv1.CephVersionSpec{Image: "quay.io/ceph/ceph:testversion"},
			Network:         network,
			DataDirHostPath: "/var/lib/rook/",
		},
		fs,
		&k8sutil.OwnerInfo{},
		"/var/lib/rook/",
		false,
	)
	mdsTestConfig := &mdsConfig{
		DaemonID:     "myfs-a",
		ResourceName: "rook-ceph-mds-myfs-a",
		DataPathMap:  config.NewStatelessDaemonDataPathMap(config.MdsType, "myfs-a", "rook-ceph", "/var/lib/rook/"),
	}
	fsNamespacedname := types.NamespacedName{Name: "myfs-a", Namespace: "ns"}

	t.Run(("check mds ConfigureProbe"), func(t *testing.T) {
		c.fs.Spec.MetadataServer.LivenessProbe = &cephv1.ProbeSpec{Disabled: false, Probe: &v1.Probe{InitialDelaySeconds: 900}}
		deployment, err := c.makeDeployment(mdsTestConfig, fsNamespacedname)
		assert.Nil(t, err)
		assert.NotNil(t, deployment)
		assert.NotNil(t, c.fs.Spec.MetadataServer.LivenessProbe)
		assert.Equal(t, "myfs-a", mdsTestConfig.DaemonID)
		assert.Equal(t, "myfs-a", fsNamespacedname.Name)
		assert.Equal(t, int32(900), deployment.Spec.Template.Spec.Containers[0].LivenessProbe.InitialDelaySeconds)
		assert.NotNil(t, deployment.Spec.Template.Spec.Containers[0].LivenessProbe.ProbeHandler.Exec)
	})
	return c.makeDeployment(mdsTestConfig, fsNamespacedname)
}

func TestPodSpecs(t *testing.T) {
	d, err := testDeploymentObject(t, cephv1.NetworkSpec{HostNetwork: false}) // no host network
	assert.Nil(t, err)

	assert.NotNil(t, d)
	assert.Equal(t, v1.RestartPolicyAlways, d.Spec.Template.Spec.RestartPolicy)
	assert.Equal(t, k8sutil.DefaultServiceAccount, d.Spec.Template.Spec.ServiceAccountName)

	// Deployment should have Ceph labels
	test.AssertLabelsContainCephRequirements(t, d.ObjectMeta.Labels,
		config.MdsType, "myfs-a", "rook-ceph-mds", "ns", "myfs", "cephfilesystems.ceph.rook.io", "ceph-mds")

	podTemplate := test.NewPodTemplateSpecTester(t, &d.Spec.Template)
	podTemplate.RunFullSuite(config.MdsType, "myfs-a", "rook-ceph-mds", "ns", "quay.io/ceph/ceph:testversion",
		"500", "250", "4337", "2169", /* resources */
		"my-priority-class", "myfs", "cephfilesystems.ceph.rook.io", "ceph-mds")

	// assert --public-addr is appended to args
	assert.Contains(t, d.Spec.Template.Spec.Containers[0].Args,
		config.NewFlag("public-addr", controller.ContainerEnvVarReference(podIPEnvVar)))
}

func TestHostNetwork(t *testing.T) {
	d, err := testDeploymentObject(t, cephv1.NetworkSpec{HostNetwork: true}) // host network
	assert.Nil(t, err)

	assert.Equal(t, true, d.Spec.Template.Spec.HostNetwork)
	assert.Equal(t, v1.DNSClusterFirstWithHostNet, d.Spec.Template.Spec.DNSPolicy)

	// assert --public-addr is not appended to args
	assert.NotContains(t, d.Spec.Template.Spec.Containers[0].Args,
		config.NewFlag("public-addr", controller.ContainerEnvVarReference(podIPEnvVar)))
	assert.NotContains(t, d.Spec.Template.Spec.Containers[0].Args, "--ms-bind-msgr1=false")
}

func TestRequireMsgr2(t *testing.T) {
	t.Run("requireMsgr2 true disables msgr1", func(t *testing.T) {
		d, err := testDeploymentObject(t, cephv1.NetworkSpec{
			Connections: &cephv1.ConnectionsSpec{RequireMsgr2: true},
		})
		assert.NoError(t, err)
		assert.Contains(t, d.Spec.Template.Spec.Containers[0].Args, "--ms-bind-msgr1=false")
	})

	t.Run("encryption enabled disables msgr1", func(t *testing.T) {
		d, err := testDeploymentObject(t, cephv1.NetworkSpec{
			Connections: &cephv1.ConnectionsSpec{
				Encryption: &cephv1.EncryptionSpec{Enabled: true},
			},
		})
		assert.NoError(t, err)
		assert.Contains(t, d.Spec.Template.Spec.Containers[0].Args, "--ms-bind-msgr1=false")
	})

	t.Run("requireMsgr2 true keeps msgr1 while a mon advertises v1", func(t *testing.T) {
		d, err := testDeploymentObjectWithMonPort(t, cephv1.NetworkSpec{
			Connections: &cephv1.ConnectionsSpec{RequireMsgr2: true},
		}, 6789)
		assert.NoError(t, err)
		assert.NotContains(t, d.Spec.Template.Spec.Containers[0].Args, "--ms-bind-msgr1=false")
	})
}

func TestMDSStartupProbe(t *testing.T) {
	makeContainer := func(t *testing.T, startupProbe *cephv1.ProbeSpec) v1.Container {
		t.Helper()
		fs := cephv1.CephFilesystem{
			ObjectMeta: metav1.ObjectMeta{Name: "myfs", Namespace: "ns"},
			Spec: cephv1.FilesystemSpec{
				MetadataServer: cephv1.MetadataServerSpec{ActiveCount: 1, StartupProbe: startupProbe},
			},
		}
		clusterInfo := &cephclient.ClusterInfo{FSID: "myfsid", CephVersion: cephver.Squid}
		c := NewCluster(
			clusterInfo,
			&clusterd.Context{Clientset: testop.New(t, 1)},
			&cephv1.ClusterSpec{CephVersion: cephv1.CephVersionSpec{Image: "quay.io/ceph/ceph:testversion"}},
			fs,
			&k8sutil.OwnerInfo{},
			"/var/lib/rook/",
			false,
		)
		mdsConfig := &mdsConfig{
			DaemonID:     "myfs-a",
			ResourceName: "rook-ceph-mds-myfs-a",
			DataPathMap:  config.NewStatelessDaemonDataPathMap(config.MdsType, "myfs-a", "rook-ceph", "/var/lib/rook/"),
		}
		d, err := c.makeDeployment(mdsConfig, types.NamespacedName{Name: "myfs", Namespace: "ns"})
		require.NoError(t, err)
		require.NotEmpty(t, d.Spec.Template.Spec.Containers)
		require.Equal(t, "mds", d.Spec.Template.Spec.Containers[0].Name)
		return d.Spec.Template.Spec.Containers[0]
	}

	t.Run("no startup probe unless the CR sets one", func(t *testing.T) {
		container := makeContainer(t, nil)
		assert.Nil(t, container.StartupProbe)
		assert.NotNil(t, container.LivenessProbe)
	})

	t.Run("timing only uses the liveness handler", func(t *testing.T) {
		container := makeContainer(t, &cephv1.ProbeSpec{Probe: &v1.Probe{TimeoutSeconds: 15}})
		require.NotNil(t, container.StartupProbe)
		require.NotNil(t, container.LivenessProbe)
		assert.Equal(t, container.LivenessProbe.ProbeHandler, container.StartupProbe.ProbeHandler)
		assert.NotNil(t, container.StartupProbe.Exec)
		assert.Equal(t, int32(15), container.StartupProbe.TimeoutSeconds)
		assert.Equal(t, container.LivenessProbe.PeriodSeconds, container.StartupProbe.PeriodSeconds)
	})

	t.Run("CR exec handler is replaced by the liveness handler", func(t *testing.T) {
		crProbe := &v1.Probe{
			ProbeHandler: v1.ProbeHandler{
				Exec: &v1.ExecAction{Command: []string{"sh", "-c", "touch /tmp/cr-handler-ran"}},
			},
			InitialDelaySeconds: 42,
			FailureThreshold:    60,
		}
		container := makeContainer(t, &cephv1.ProbeSpec{Probe: crProbe})
		require.NotNil(t, container.StartupProbe)
		require.NotNil(t, container.LivenessProbe)
		assert.Equal(t, container.LivenessProbe.ProbeHandler, container.StartupProbe.ProbeHandler)
		assert.Equal(t, int32(42), container.StartupProbe.InitialDelaySeconds)
		assert.Equal(t, int32(60), container.StartupProbe.FailureThreshold)
	})

	t.Run("disabled", func(t *testing.T) {
		container := makeContainer(t, &cephv1.ProbeSpec{Disabled: true, Probe: &v1.Probe{TimeoutSeconds: 15}})
		assert.Nil(t, container.StartupProbe)
	})
}
