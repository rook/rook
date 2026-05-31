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

package nodedaemon

import (
	"context"
	"fmt"
	"testing"

	cephv1 "github.com/rook/rook/pkg/apis/ceph.rook.io/v1"
	rookclient "github.com/rook/rook/pkg/client/clientset/versioned/fake"
	"github.com/rook/rook/pkg/client/clientset/versioned/scheme"
	"github.com/rook/rook/pkg/clusterd"
	"github.com/rook/rook/pkg/operator/ceph/controller"
	cephver "github.com/rook/rook/pkg/operator/ceph/version"
	"github.com/rook/rook/pkg/operator/k8sutil"
	"github.com/rook/rook/pkg/operator/test"
	"github.com/stretchr/testify/assert"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

func TestGenerateCrashEnvVar(t *testing.T) {
	env := generateCrashEnvVar()
	assert.Equal(t, "CEPH_ARGS", env.Name)
	assert.Equal(t, "-m $(ROOK_CEPH_MON_HOST) -k /etc/ceph/crash-collector-keyring-store/keyring", env.Value)
}

func TestCreateOrUpdateCephCrash(t *testing.T) {
	cephCluster := cephv1.CephCluster{
		ObjectMeta: metav1.ObjectMeta{Namespace: "rook-ceph"},
	}
	cephCluster.Spec.Labels = cephv1.LabelsSpec{}
	cephCluster.Spec.PriorityClassNames = cephv1.PriorityClassNamesSpec{}
	cephVersion := &cephver.CephVersion{Major: 17, Minor: 2, Extra: 0}
	ctx := context.TODO()
	context := &clusterd.Context{
		Clientset:     test.New(t, 1),
		RookClientset: rookclient.NewSimpleClientset(),
	}

	s := scheme.Scheme
	err := appsv1.AddToScheme(s)
	if err != nil {
		assert.Fail(t, "failed to build scheme")
	}
	r := &ReconcileNode{
		scheme:  s,
		client:  fake.NewClientBuilder().WithScheme(s).WithRuntimeObjects().Build(),
		context: context,
	}

	node := corev1.Node{}
	nodeSelector := map[string]string{corev1.LabelHostname: "testnode"}
	node.SetLabels(nodeSelector)
	tolerations := []corev1.Toleration{{}}
	res, err := r.createOrUpdateCephCrash(node, tolerations, cephCluster, cephVersion)
	assert.NoError(t, err)
	assert.Equal(t, controllerutil.OperationResult("created"), res)
	name := k8sutil.TruncateNodeName(fmt.Sprintf("%s-%%s", CrashCollectorAppName), "testnode")
	deploy := appsv1.Deployment{}
	err = r.client.Get(ctx, types.NamespacedName{Namespace: "rook-ceph", Name: name}, &deploy)
	assert.NoError(t, err)
	podSpec := deploy.Spec.Template
	assert.Equal(t, nodeSelector, podSpec.Spec.NodeSelector)
	assert.Equal(t, "", podSpec.ObjectMeta.Labels["foo"])
	assert.Equal(t, tolerations, podSpec.Spec.Tolerations)
	assert.Equal(t, false, podSpec.Spec.HostNetwork)
	assert.Equal(t, "", podSpec.Spec.PriorityClassName)
	assert.Equal(t, controller.CephUserID, *podSpec.Spec.Containers[0].SecurityContext.RunAsUser)
	assert.Equal(t, controller.CephUserID, *podSpec.Spec.Containers[0].SecurityContext.RunAsGroup)

	cephCluster.Spec.Labels[cephv1.KeyCrashCollector] = map[string]string{"foo": "bar"}
	cephCluster.Spec.Network.HostNetwork = true
	cephCluster.Spec.PriorityClassNames[cephv1.KeyCrashCollector] = "test-priority-class"
	tolerations = []corev1.Toleration{{Key: "key", Operator: "Equal", Value: "value", Effect: "NoSchedule"}}
	res, err = r.createOrUpdateCephCrash(node, tolerations, cephCluster, cephVersion)
	assert.NoError(t, err)
	assert.Equal(t, controllerutil.OperationResult("updated"), res)
	err = r.client.Get(ctx, types.NamespacedName{Namespace: "rook-ceph", Name: name}, &deploy)
	assert.NoError(t, err)
	podSpec = deploy.Spec.Template
	assert.Equal(t, "bar", podSpec.ObjectMeta.Labels["foo"])
	assert.Equal(t, tolerations, podSpec.Spec.Tolerations)
	assert.Equal(t, true, podSpec.Spec.HostNetwork)
	assert.Equal(t, "test-priority-class", podSpec.Spec.PriorityClassName)
	assert.NotEqual(t, "", deploy.ObjectMeta.Labels["rook-version"])
	assert.Equal(t, "", podSpec.ObjectMeta.Labels["rook-version"])
}
