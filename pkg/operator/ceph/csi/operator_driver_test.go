/*
Copyright 2024 The Rook Authors. All rights reserved.

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

package csi

import (
	"context"
	"fmt"
	"testing"

	csiopv1 "github.com/ceph/ceph-csi-operator/api/v1"
	cephv1 "github.com/rook/rook/pkg/apis/ceph.rook.io/v1"
	rookclient "github.com/rook/rook/pkg/client/clientset/versioned/fake"
	"github.com/rook/rook/pkg/client/clientset/versioned/scheme"
	"github.com/rook/rook/pkg/clusterd"
	clienttest "github.com/rook/rook/pkg/daemon/ceph/client/test"
	opcontroller "github.com/rook/rook/pkg/operator/ceph/controller"
	testop "github.com/rook/rook/pkg/operator/test"
	"github.com/stretchr/testify/assert"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func TestReconcileCSI_createOrUpdateDriverResources(t *testing.T) {
	tests := []struct {
		name               string
		networkSpec        cephv1.NetworkSpec
		expectMultusAnnots bool
	}{
		{
			name: "without multus",
			networkSpec: cephv1.NetworkSpec{
				Connections: &cephv1.ConnectionsSpec{
					Encryption: &cephv1.EncryptionSpec{
						Enabled: true,
					},
				},
			},
			expectMultusAnnots: false,
		},
		{
			name: "with multus",
			networkSpec: cephv1.NetworkSpec{
				Provider: "multus",
				Selectors: map[cephv1.CephNetworkType]string{
					"public":  "rook-ceph/rook-ceph-public-network",
					"cluster": "rook-ceph/rook-ceph-cluster-network",
				},
			},
			expectMultusAnnots: true,
		},
	}

	CSIParam.CSIRBDPodLabels = map[string]string{"rbd-label": "rbd-value"}
	CSIParam.CSICephFSPodLabels = map[string]string{"cephfs-label": "cephfs-value"}
	CSIParam.CSINFSPodLabels = map[string]string{"nfs-label": "nfs-value"}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ns := "test"
			r := &ReconcileCSI{
				context: &clusterd.Context{
					Clientset:     testop.New(t, 1),
					RookClientset: rookclient.NewSimpleClientset(),
				},
				opManagerContext: context.TODO(),
				opConfig: opcontroller.OperatorConfig{
					OperatorNamespace: "test",
				},
			}
			cluster := &cephv1.CephCluster{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "testCluster",
					Namespace: ns,
				},
				Spec: cephv1.ClusterSpec{
					CSI: cephv1.CSIDriverSpec{
						ReadAffinity: cephv1.ReadAffinitySpec{
							Enabled:             true,
							CrushLocationLabels: []string{"kubernetes.io/hostname"},
						},
						CephFS: cephv1.CSICephFSSpec{
							KernelMountOptions: "ms_mode=crc",
						},
					},
					Network: tt.networkSpec,
				},
			}
			driver := &csiopv1.Driver{}

			// Register operator types with the runtime scheme.
			s := scheme.Scheme
			s.AddKnownTypes(cephv1.SchemeGroupVersion, driver, &cephv1.CephCluster{}, &v1.ConfigMap{})
			object := []runtime.Object{
				cluster,
			}
			cl := fake.NewClientBuilder().WithScheme(s).WithRuntimeObjects(object...).Build()
			r.client = cl

			EnableRBD = true
			EnableCephFS = true
			EnableNFS = true

			c := clienttest.CreateTestClusterInfo(3)
			c.Namespace = ns
			c.SetName("testcluster")
			c.NamespacedName()

			cephv1.SetEnforceHostNetwork(true)
			defer cephv1.SetEnforceHostNetwork(false)
			err := r.createOrUpdateDriverResources(*cluster, c)
			assert.NoError(t, err)

			// Test RBD driver
			err = cl.Get(context.TODO(), types.NamespacedName{Name: fmt.Sprintf("%s.rbd.csi.ceph.com", c.Namespace), Namespace: ns}, driver)
			assert.NoError(t, err)
			assert.True(t, *driver.Spec.ControllerPlugin.HostNetwork)
			if tt.expectMultusAnnots {
				// Verify that both NodePlugin and ControllerPlugin have Multus annotations
				assert.Nil(t, driver.Spec.NodePlugin.PodCommonSpec.Annotations)
				assert.NotContains(t, driver.Spec.NodePlugin.PodCommonSpec.Annotations, "k8s.v1.cni.cncf.io/networks")

				assert.NotNil(t, driver.Spec.ControllerPlugin.PodCommonSpec.Annotations)
				assert.Contains(t, driver.Spec.ControllerPlugin.PodCommonSpec.Annotations, "k8s.v1.cni.cncf.io/networks")

				// Verify the annotation value contains the public network only not the cluster network
				nodeMultusAnnotation := driver.Spec.NodePlugin.PodCommonSpec.Annotations["k8s.v1.cni.cncf.io/networks"]
				assert.NotContains(t, nodeMultusAnnotation, "rook-ceph-public-network")
				assert.NotContains(t, nodeMultusAnnotation, "rook-ceph-cluster-network")

				controllerMultusAnnotation := driver.Spec.ControllerPlugin.PodCommonSpec.Annotations["k8s.v1.cni.cncf.io/networks"]
				assert.Contains(t, controllerMultusAnnotation, "rook-ceph-public-network")
				assert.NotContains(t, controllerMultusAnnotation, "rook-ceph-cluster-network")
			} else {
				// Verify no Multus annotations when not using Multus
				if driver.Spec.NodePlugin.PodCommonSpec.Annotations != nil {
					assert.NotContains(t, driver.Spec.NodePlugin.PodCommonSpec.Annotations, "k8s.v1.cni.cncf.io/networks")
				}
				if driver.Spec.ControllerPlugin.PodCommonSpec.Annotations != nil {
					assert.NotContains(t, driver.Spec.ControllerPlugin.PodCommonSpec.Annotations, "k8s.v1.cni.cncf.io/networks")
				}
			}

			assert.Equal(t, "rbd-value", driver.Spec.ControllerPlugin.PodCommonSpec.Labels["rbd-label"])
			assert.Equal(t, "rbd-value", driver.Spec.NodePlugin.PodCommonSpec.Labels["rbd-label"])
			assert.NotEqualValues(t, "rbd-value", driver.Spec.NodePlugin.PodCommonSpec.Labels["cephfs-label"])

			// Test CephFS driver
			err = cl.Get(context.TODO(), types.NamespacedName{Name: fmt.Sprintf("%s.cephfs.csi.ceph.com", c.Namespace), Namespace: ns}, driver)
			assert.NoError(t, err)

			assert.Equal(t, "cephfs-value", driver.Spec.ControllerPlugin.PodCommonSpec.Labels["cephfs-label"])
			assert.Equal(t, "cephfs-value", driver.Spec.NodePlugin.PodCommonSpec.Labels["cephfs-label"])
			assert.NotEqualValues(t, "cephfs-value", driver.Spec.NodePlugin.PodCommonSpec.Labels["rbd-label"])

			// Test NFS driver
			err = cl.Get(context.TODO(), types.NamespacedName{Name: fmt.Sprintf("%s.nfs.csi.ceph.com", c.Namespace), Namespace: ns}, driver)
			assert.NoError(t, err)

			assert.Equal(t, "nfs-value", driver.Spec.ControllerPlugin.PodCommonSpec.Labels["nfs-label"])
			assert.Equal(t, "nfs-value", driver.Spec.NodePlugin.PodCommonSpec.Labels["nfs-label"])
			assert.NotEqualValues(t, "nfs-value", driver.Spec.NodePlugin.PodCommonSpec.Labels["rbd-label"])
		})
	}
}
