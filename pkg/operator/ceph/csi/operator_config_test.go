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
	appsv1 "k8s.io/api/apps/v1"
	v1 "k8s.io/api/core/v1"
	apifake "k8s.io/apiextensions-apiserver/pkg/client/clientset/clientset/fake"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func TestReconcileCSI_helmLabels(t *testing.T) {
	const ns = "test"
	tests := []struct {
		name, opRelease, clusterRelease string
		expectHelm                      bool
	}{
		{"rook operator helm chart only", operatorHelmReleaseName, "", true},
		{"ceph cluster helm chart only", "", "my-ceph-cluster", true},
		{"rook operator and ceph cluster helm charts", operatorHelmReleaseName, "my-ceph-cluster", true},
		{"no helm charts", "", "", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			clientset := testop.New(t, 1)
			opAnnots := map[string]string{}
			if tt.opRelease != "" {
				opAnnots["meta.helm.sh/release-name"] = tt.opRelease
			}
			_, err := clientset.AppsV1().Deployments(ns).Create(context.TODO(), &appsv1.Deployment{
				ObjectMeta: metav1.ObjectMeta{Name: "rook-ceph-operator", Namespace: ns, Annotations: opAnnots},
			}, metav1.CreateOptions{})
			assert.NoError(t, err)

			cluster := &cephv1.CephCluster{
				ObjectMeta: metav1.ObjectMeta{Name: "testCluster", Namespace: ns},
				Spec: cephv1.ClusterSpec{CSI: cephv1.CSIDriverSpec{
					ReadAffinity: cephv1.ReadAffinitySpec{Enabled: true, CrushLocationLabels: []string{"kubernetes.io/hostname"}},
				}},
			}
			if tt.clusterRelease != "" {
				cluster.Annotations = map[string]string{"meta.helm.sh/release-name": tt.clusterRelease}
			}

			s := scheme.Scheme
			s.AddKnownTypes(cephv1.SchemeGroupVersion, &csiopv1.OperatorConfig{}, &csiopv1.Driver{}, &cephv1.CephCluster{}, &v1.ConfigMap{})
			r := &ReconcileCSI{
				context: &clusterd.Context{
					Clientset:           clientset,
					RookClientset:       rookclient.NewSimpleClientset(),
					ApiExtensionsClient: apifake.NewClientset(),
				},
				opManagerContext: context.TODO(),
				opConfig:         opcontroller.OperatorConfig{OperatorNamespace: ns},
				client:           fake.NewClientBuilder().WithScheme(s).WithRuntimeObjects(cluster).Build(),
			}
			assert.NoError(t, r.setParams())
			assert.NoError(t, r.createOrUpdateOperatorConfig(*cluster))

			c := clienttest.CreateTestClusterInfo(3)
			c.Namespace = ns
			assert.NoError(t, r.createOrUpdateDriverResources(*cluster, c))

			check := func(meta *metav1.ObjectMeta, release string) {
				if !tt.expectHelm {
					assert.NotContains(t, meta.Labels, "app.kubernetes.io/managed-by")
					return
				}
				assert.Equal(t, release, meta.Annotations["meta.helm.sh/release-name"])
			}

			opConfig := &csiopv1.OperatorConfig{}
			assert.NoError(t, r.client.Get(context.TODO(), types.NamespacedName{Name: opConfigCRName, Namespace: ns}, opConfig))
			check(&opConfig.ObjectMeta, cephCsiHelmDriversReleaseName)

			cm := &v1.ConfigMap{}
			assert.NoError(t, r.client.Get(context.TODO(), types.NamespacedName{Name: "rook-csi-operator-image-set-configmap", Namespace: ns}, cm))
			check(&cm.ObjectMeta, operatorHelmReleaseName)

			driver := &csiopv1.Driver{}
			assert.NoError(t, r.client.Get(context.TODO(), types.NamespacedName{Name: fmt.Sprintf("%s.rbd.csi.ceph.com", ns), Namespace: ns}, driver))
			check(&driver.ObjectMeta, cephCsiHelmDriversReleaseName)
		})
	}
}

func TestReconcileCSI_createOrUpdateOperatorConfig(t *testing.T) {
	ns := "test"
	cephv1.SetEnforceHostNetwork(true)
	defer cephv1.SetEnforceHostNetwork(false)
	r := &ReconcileCSI{
		context: &clusterd.Context{
			Clientset:           testop.New(t, 1),
			RookClientset:       rookclient.NewSimpleClientset(),
			ApiExtensionsClient: apifake.NewClientset(),
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
			Network: cephv1.NetworkSpec{
				Connections: &cephv1.ConnectionsSpec{
					Encryption: &cephv1.EncryptionSpec{
						Enabled: true,
					},
				},
			},
		},
	}
	opConfig := &csiopv1.OperatorConfig{}

	// Register operator types with the runtime scheme.
	s := scheme.Scheme
	s.AddKnownTypes(cephv1.SchemeGroupVersion, opConfig, &cephv1.CephCluster{}, &v1.ConfigMap{})
	object := []runtime.Object{
		cluster,
	}
	cl := fake.NewClientBuilder().WithScheme(s).WithRuntimeObjects(object...).Build()
	r.client = cl

	err := r.setParams()
	assert.NoError(t, err)

	err = r.createOrUpdateOperatorConfig(*cluster)
	assert.NoError(t, err)

	err = cl.Get(context.TODO(), types.NamespacedName{Name: opConfigCRName, Namespace: r.opConfig.OperatorNamespace}, opConfig)
	assert.NoError(t, err)
	assert.True(t, *opConfig.Spec.DriverSpecDefaults.EnableMetadata)
	assert.True(t, *opConfig.Spec.DriverSpecDefaults.ControllerPlugin.HostNetwork)
}
