/*
Copyright 2026 The Rook Authors. All rights reserved.

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
	"testing"

	"github.com/rook/rook/pkg/client/clientset/versioned/scheme"
	"github.com/rook/rook/pkg/operator/k8sutil"
	"github.com/stretchr/testify/assert"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

// When a node no longer exists (NotFound), the reconcile must delete the node
// daemons targeting it, while never touching the daemons of nodes that still exist.
func TestReconcileNodeNotFoundCleansUpNodeDaemons(t *testing.T) {
	const namespace = "rook-ceph"
	ctx := context.TODO()

	s := scheme.Scheme
	err := appsv1.AddToScheme(s)
	assert.NoError(t, err)
	err = corev1.AddToScheme(s)
	assert.NoError(t, err)

	makeNodeDaemonDeployment := func(appName, name, nodeName string) *appsv1.Deployment {
		return &appsv1.Deployment{
			ObjectMeta: metav1.ObjectMeta{
				Name:      name,
				Namespace: namespace,
				Labels: map[string]string{
					k8sutil.AppAttr: appName,
					NodeNameLabel:   nodeName,
				},
			},
		}
	}

	crashOrphaned := makeNodeDaemonDeployment(CrashCollectorAppName, "crashcollector-orphaned", "gone-node")
	exporterOrphaned := makeNodeDaemonDeployment(cephExporterAppName, "exporter-orphaned", "gone-node")
	crashHealthy := makeNodeDaemonDeployment(CrashCollectorAppName, "crashcollector-healthy", "live-node")
	exporterHealthy := makeNodeDaemonDeployment(cephExporterAppName, "exporter-healthy", "live-node")
	liveNode := &corev1.Node{ObjectMeta: metav1.ObjectMeta{Name: "live-node"}}

	r := &ReconcileNode{
		client:           fake.NewClientBuilder().WithScheme(s).WithRuntimeObjects(crashOrphaned, exporterOrphaned, crashHealthy, exporterHealthy, liveNode).Build(),
		opManagerContext: ctx,
	}

	request := reconcile.Request{NamespacedName: types.NamespacedName{Name: "gone-node"}}
	result, err := r.reconcile(request)
	assert.NoError(t, err)
	assert.Equal(t, reconcile.Result{}, result)

	remaining := &appsv1.DeploymentList{}
	assert.NoError(t, r.client.List(ctx, remaining, client.InNamespace(namespace)))
	assert.Len(t, remaining.Items, 2)
	remainingNames := []string{remaining.Items[0].Name, remaining.Items[1].Name}
	assert.ElementsMatch(t, []string{"crashcollector-healthy", "exporter-healthy"}, remainingNames)

	// a node with no daemons at all is still a no-op
	request = reconcile.Request{NamespacedName: types.NamespacedName{Name: "never-existed-node"}}
	result, err = r.reconcile(request)
	assert.NoError(t, err)
	assert.Equal(t, reconcile.Result{}, result)

	remaining = &appsv1.DeploymentList{}
	assert.NoError(t, r.client.List(ctx, remaining, client.InNamespace(namespace)))
	assert.Len(t, remaining.Items, 2)
}
