/*
Copyright 2020 The Rook Authors. All rights reserved.

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

package operator

import (
	"context"
	"os"
	"testing"

	"github.com/rook/rook/pkg/operator/ceph/controller"
	"github.com/stretchr/testify/assert"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"
)

const (
	testNamespace = "test-namespace"
)

func TestGetTolerations(t *testing.T) {
	ctx := context.TODO()
	clientset := fake.NewSimpleClientset()
	os.Setenv("POD_NAMESPACE", testNamespace)

	// No setting results in the default value.
	cm := &v1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      controller.OperatorSettingConfigMapName,
			Namespace: testNamespace,
		},
		Data: map[string]string{},
	}
	_, err := clientset.CoreV1().ConfigMaps(testNamespace).Create(ctx, cm, metav1.CreateOptions{})
	assert.NoError(t, err)
	tolerations := getTolerations(clientset)
	expected := []v1.Toleration{}
	assert.Equal(t, expected, tolerations)

	// The invalid setting results in the default value.
	cm.Data = map[string]string{
		admissionControllerTolerationsEnv: "",
	}
	_, err = clientset.CoreV1().ConfigMaps(testNamespace).Update(ctx, cm, metav1.UpdateOptions{})
	assert.NoError(t, err)
	tolerations = getTolerations(clientset)
	assert.Equal(t, expected, tolerations)

	// Correct setting result in the desired value.
	cm.Data = map[string]string{
		admissionControllerTolerationsEnv: `
- effect: NoSchedule
  key: node-role.kubernetes.io/controlplane
  operator: Exists`,
	}
	_, err = clientset.CoreV1().ConfigMaps(testNamespace).Update(ctx, cm, metav1.UpdateOptions{})
	assert.NoError(t, err)
	tolerations = getTolerations(clientset)
	expected = []v1.Toleration{
		{
			Effect:   "NoSchedule",
			Key:      "node-role.kubernetes.io/controlplane",
			Operator: v1.TolerationOpExists,
		},
	}
	assert.Equal(t, expected, tolerations)
}

func TestGetNodeAffinity(t *testing.T) {
	ctx := context.TODO()
	clientset := fake.NewSimpleClientset()
	os.Setenv("POD_NAMESPACE", testNamespace)

	// No setting results in the default value.
	cm := &v1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      controller.OperatorSettingConfigMapName,
			Namespace: testNamespace,
		},
		Data: map[string]string{},
	}
	_, err := clientset.CoreV1().ConfigMaps(testNamespace).Create(ctx, cm, metav1.CreateOptions{})
	assert.NoError(t, err)
	nodeAffinity := getNodeAffinity(clientset)
	expected := &v1.NodeAffinity{}
	assert.Equal(t, expected, nodeAffinity)

	// The invalid setting results in the default value.
	cm.Data = map[string]string{
		admissionControllerNodeAffinityEnv: "",
	}
	_, err = clientset.CoreV1().ConfigMaps(testNamespace).Update(ctx, cm, metav1.UpdateOptions{})
	assert.NoError(t, err)
	nodeAffinity = getNodeAffinity(clientset)
	assert.Equal(t, expected, nodeAffinity)

	// Correct setting result in the desired value.
	cm.Data = map[string]string{
		admissionControllerNodeAffinityEnv: "role=storage-node",
	}
	_, err = clientset.CoreV1().ConfigMaps(testNamespace).Update(ctx, cm, metav1.UpdateOptions{})
	assert.NoError(t, err)
	nodeAffinity = getNodeAffinity(clientset)
	expected = &v1.NodeAffinity{
		RequiredDuringSchedulingIgnoredDuringExecution: &v1.NodeSelector{
			NodeSelectorTerms: []v1.NodeSelectorTerm{
				{
					MatchExpressions: []v1.NodeSelectorRequirement{
						{
							Key:      "role",
							Operator: v1.NodeSelectorOpIn,
							Values:   []string{"storage-node"},
						},
					},
				},
			},
		},
	}
	assert.Equal(t, expected, nodeAffinity)
}
