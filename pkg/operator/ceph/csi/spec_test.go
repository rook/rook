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

package csi

import (
	"context"
	_ "embed"
	"testing"

	cephv1 "github.com/rook/rook/pkg/apis/ceph.rook.io/v1"
	rookclient "github.com/rook/rook/pkg/client/clientset/versioned/fake"
	"github.com/rook/rook/pkg/clusterd"
	cephclient "github.com/rook/rook/pkg/daemon/ceph/client"
	opcontroller "github.com/rook/rook/pkg/operator/ceph/controller"
	"github.com/rook/rook/pkg/operator/k8sutil"
	testop "github.com/rook/rook/pkg/operator/test"
	"github.com/stretchr/testify/assert"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func TestReconcileCSI_configureHolders(t *testing.T) {
	r := &ReconcileCSI{
		context: &clusterd.Context{
			Clientset:     testop.New(t, 1),
			RookClientset: rookclient.NewSimpleClientset(),
		},
		opManagerContext:   context.TODO(),
		opConfig:           opcontroller.OperatorConfig{},
		clustersWithHolder: []ClusterDetail{},
	}

	t.Run("no clusters, noop", func(t *testing.T) {
		err := r.configureHolders([]driverDetails{}, templateParam{}, []v1.Toleration{}, nil)
		assert.NoError(t, err)
	})

	t.Run("one multus cluster", func(t *testing.T) {
		r.opConfig.OperatorNamespace = "rook-ceph"
		driverDetails := []driverDetails{
			{
				name:           "rbd",
				fullName:       "rbd.csi.ceph.com",
				holderTemplate: CephFSPluginHolderTemplatePath,
				nodeAffinity:   cephFSPluginNodeAffinityEnv,
				toleration:     cephFSPluginTolerationsEnv,
			},
		}
		tp := templateParam{
			Param:     CSIParam,
			Namespace: r.opConfig.OperatorNamespace,
		}

		r.clustersWithHolder = []ClusterDetail{
			{
				cluster: &cephv1.CephCluster{
					ObjectMeta: metav1.ObjectMeta{Namespace: "rook-ceph", Name: "my-cluster"},
					Spec:       cephv1.ClusterSpec{},
				},
				clusterInfo: &cephclient.ClusterInfo{Monitors: map[string]*cephclient.MonInfo{"a": {Name: "a", Endpoint: "10.0.0.1:6789"}}},
			},
		}

		t.Setenv(k8sutil.PodNamespaceEnvVar, "rook-ceph")
		_, err := r.context.Clientset.CoreV1().ConfigMaps("rook-ceph").Create(r.opManagerContext, &v1.ConfigMap{ObjectMeta: metav1.ObjectMeta{Name: ConfigName, Namespace: "rook-ceph"}, Data: map[string]string{}}, metav1.CreateOptions{})
		assert.NoError(t, err)

		err = r.configureHolders(driverDetails, tp, []v1.Toleration{}, nil)
		assert.NoError(t, err)
	})
}

func TestGenerateNetNamespaceFilePath(t *testing.T) {
	ctx := context.TODO()

	t.Run("generate with no op configmap available and non supported driver name", func(t *testing.T) {
		client := fake.NewClientBuilder().Build()
		netNsFilePath, err := GenerateNetNamespaceFilePath(ctx, client, "rook-ceph", "rook-ceph", "foo")
		assert.Error(t, err)
		assert.Empty(t, "", netNsFilePath)
	})

	t.Run("generate with no op configmap available for rbd", func(t *testing.T) {
		client := fake.NewClientBuilder().Build()
		netNsFilePath, err := GenerateNetNamespaceFilePath(ctx, client, "rook-ceph", "rook-ceph", "rbd")
		assert.NoError(t, err)
		assert.Equal(t, "/var/lib/kubelet/plugins/rook-ceph.rbd.csi.ceph.com/rook-ceph.net.ns", netNsFilePath)
	})

	t.Run("generate with no op configmap available for cephfs", func(t *testing.T) {
		client := fake.NewClientBuilder().Build()
		netNsFilePath, err := GenerateNetNamespaceFilePath(ctx, client, "rook-ceph", "rook-ceph", "cephfs")
		assert.NoError(t, err)
		assert.Equal(t, "/var/lib/kubelet/plugins/rook-ceph.cephfs.csi.ceph.com/rook-ceph.net.ns", netNsFilePath)
	})

	t.Run("generate with op configmap for cephfs", func(t *testing.T) {
		opCm := &v1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{
				Name:      opcontroller.OperatorSettingConfigMapName,
				Namespace: "rook-ceph",
			},
			Data: map[string]string{"ROOK_CSI_KUBELET_DIR_PATH": "/foo"},
		}
		object := []runtime.Object{
			opCm,
		}
		client := fake.NewClientBuilder().WithRuntimeObjects(object...).Build()
		netNsFilePath, err := GenerateNetNamespaceFilePath(ctx, client, "rook-ceph", "rook-ceph", "cephfs")
		assert.NoError(t, err)
		assert.Equal(t, "/foo/plugins/rook-ceph.cephfs.csi.ceph.com/rook-ceph.net.ns", netNsFilePath)
	})
}
