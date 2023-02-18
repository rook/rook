/*
Copyright 2023 The Rook Authors. All rights reserved.

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

	monitoringv1 "github.com/prometheus-operator/prometheus-operator/pkg/apis/monitoring/v1"
	cephv1 "github.com/rook/rook/pkg/apis/ceph.rook.io/v1"
	rookclient "github.com/rook/rook/pkg/client/clientset/versioned/fake"
	"github.com/rook/rook/pkg/client/clientset/versioned/scheme"
	"github.com/rook/rook/pkg/clusterd"
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

func TestCreateOrUpdateCephExporter(t *testing.T) {
	cephCluster := cephv1.CephCluster{
		ObjectMeta: metav1.ObjectMeta{Namespace: "rook-ceph"},
	}
	cephCluster.Spec.Labels = cephv1.LabelsSpec{}
	cephCluster.Spec.PriorityClassNames = cephv1.PriorityClassNamesSpec{}
	cephVersion := &cephver.CephVersion{Major: 17, Minor: 2, Extra: 5}
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
	res, err := r.createOrUpdateCephExporter(node, tolerations, cephCluster, cephVersion)
	assert.NoError(t, err)
	assert.Equal(t, controllerutil.OperationResult("created"), res)
	name := k8sutil.TruncateNodeName(fmt.Sprintf("%s-%%s", cephExporterAppName), "testnode")
	deploy := appsv1.Deployment{}
	err = r.client.Get(ctx, types.NamespacedName{Namespace: "rook-ceph", Name: name}, &deploy)
	assert.NoError(t, err)
	podSpec := deploy.Spec.Template
	assert.Equal(t, nodeSelector, podSpec.Spec.NodeSelector)
	assert.Equal(t, "", podSpec.ObjectMeta.Labels["foo"])
	assert.Equal(t, tolerations, podSpec.Spec.Tolerations)
	assert.Equal(t, false, podSpec.Spec.HostNetwork)
	assert.Equal(t, "", podSpec.Spec.PriorityClassName)

	cephCluster.Spec.Labels[cephv1.KeyCephExporter] = map[string]string{"foo": "bar"}
	cephCluster.Spec.Network.HostNetwork = true
	cephCluster.Spec.PriorityClassNames[cephv1.KeyCephExporter] = "test-priority-class"
	tolerations = []corev1.Toleration{{Key: "key", Operator: "Equal", Value: "value", Effect: "NoSchedule"}}
	res, err = r.createOrUpdateCephExporter(node, tolerations, cephCluster, cephVersion)
	assert.NoError(t, err)
	assert.Equal(t, controllerutil.OperationResult("updated"), res)
	err = r.client.Get(ctx, types.NamespacedName{Namespace: "rook-ceph", Name: name}, &deploy)
	assert.NoError(t, err)
	podSpec = deploy.Spec.Template
	assert.Equal(t, "bar", podSpec.ObjectMeta.Labels["foo"])
	assert.Equal(t, tolerations, podSpec.Spec.Tolerations)
	assert.Equal(t, true, podSpec.Spec.HostNetwork)
	assert.Equal(t, "test-priority-class", podSpec.Spec.PriorityClassName)
}

func TestServiceSpec(t *testing.T) {
	cephCluster := cephv1.CephCluster{
		ObjectMeta: metav1.ObjectMeta{Namespace: "rook-ceph"},
	}
	cephCluster.Spec.Labels = cephv1.LabelsSpec{}
	cephCluster.Spec.PriorityClassNames = cephv1.PriorityClassNamesSpec{}

	s, err := MakeCephExporterMetricsService(cephCluster, exporterServiceMetricName, scheme.Scheme)
	assert.NoError(t, err)
	assert.NotNil(t, s)
	assert.Equal(t, "rook-ceph-exporter", s.Name)
	assert.Equal(t, 1, len(s.Spec.Ports))
	assert.Equal(t, 2, len(s.Labels))
	assert.Equal(t, 2, len(s.Spec.Selector))
}

func TestApplyCephExporterLabels(t *testing.T) {
	cephCluster := cephv1.CephCluster{
		ObjectMeta: metav1.ObjectMeta{Namespace: "rook-ceph"},
	}
	cephCluster.Spec.Labels = cephv1.LabelsSpec{}
	cephCluster.Spec.PriorityClassNames = cephv1.PriorityClassNamesSpec{}

	sm := &monitoringv1.ServiceMonitor{Spec: monitoringv1.ServiceMonitorSpec{
		Endpoints: []monitoringv1.Endpoint{{}}}}

	// Service Monitor RelabelConfigs updated when 'rook.io/managedBy' monitoring label is found
	monitoringLabels := cephv1.LabelsSpec{
		cephv1.KeyCephExporter: map[string]string{
			"rook.io/managedBy": "storagecluster"},
	}
	cephCluster.Spec.Labels = monitoringLabels
	applyCephExporterLabels(cephCluster, sm)
	fmt.Printf("Hello1")
	assert.Equal(t, "managedBy", sm.Spec.Endpoints[0].RelabelConfigs[0].TargetLabel)
	assert.Equal(t, "storagecluster", sm.Spec.Endpoints[0].RelabelConfigs[0].Replacement)

	// Service Monitor RelabelConfigs not updated when the required monitoring label is not found
	monitoringLabels = cephv1.LabelsSpec{
		cephv1.KeyCephExporter: map[string]string{
			"wrongLabelKey": "storagecluster"},
	}
	cephCluster.Spec.Labels = monitoringLabels
	sm.Spec.Endpoints[0].RelabelConfigs = nil
	applyCephExporterLabels(cephCluster, sm)
	assert.Nil(t, sm.Spec.Endpoints[0].RelabelConfigs)

	// Service Monitor RelabelConfigs not updated when no monitoring labels are found
	cephCluster.Spec.Labels = cephv1.LabelsSpec{}
	sm.Spec.Endpoints[0].RelabelConfigs = nil
	applyCephExporterLabels(cephCluster, sm)
	assert.Nil(t, sm.Spec.Endpoints[0].RelabelConfigs)
}
