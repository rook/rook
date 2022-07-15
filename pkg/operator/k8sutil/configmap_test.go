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

package k8sutil

import (
	"context"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"
)

func TestDeleteConfigMap(t *testing.T) {
	k8s := fake.NewSimpleClientset()
	ctx := context.TODO()

	cm := &v1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-configmap",
			Namespace: "test-namespace",
		},
		Data: map[string]string{
			"test": "data",
		},
	}

	_, err := k8s.CoreV1().ConfigMaps("test-namespace").Create(ctx, cm, metav1.CreateOptions{})
	assert.NoError(t, err)

	// There is no need to test all permutations, as the `DeleteResource` function is already
	// tested. Setting Wait=true and ErrorOnTimeout=true will cause both the delete and verify
	// functions to be exercised, and it will return error if either fail with an unexpected error.
	opts := &DeleteOptions{}
	opts.Wait = true
	opts.ErrorOnTimeout = true
	err = DeleteConfigMap(ctx, k8s, "test-configmap", "test-namespace", opts)
	assert.NoError(t, err)

	_, err = k8s.CoreV1().ConfigMaps("test-namespace").Get(ctx, "test-configmap", metav1.GetOptions{})
	assert.Error(t, err)
	assert.True(t, errors.IsNotFound(err))
}

func TestGetOperatorSetting(t *testing.T) {
	k8s := fake.NewSimpleClientset()
	ctx := context.TODO()

	operatorSettingConfigMapName := "rook-operator-config"
	testNamespace := "test-namespace"
	cm := &v1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      operatorSettingConfigMapName,
			Namespace: testNamespace,
		},
		Data: map[string]string{
			"NODE_AFFINITY": "storage=rook, worker",
		},
	}

	nodeAffinity := "NODE_AFFINITY"
	podAffinity := "POD_AFFINITY"
	envSettingValue := "role=storage-node"
	cmSettingValue := "storage=rook, worker"
	defaultValue := ""

	// ConfigMap is not found
	setting, err := GetOperatorSetting(ctx, k8s, operatorSettingConfigMapName, nodeAffinity, defaultValue)
	assert.NoError(t, err)

	// Env Var doesn't exist
	assert.Equal(t, defaultValue, setting)
	// Env Var exists
	t.Setenv(nodeAffinity, envSettingValue)
	setting, err = GetOperatorSetting(ctx, k8s, operatorSettingConfigMapName, nodeAffinity, defaultValue)
	assert.NoError(t, err)
	assert.Equal(t, envSettingValue, setting)

	// ConfigMap is found
	t.Setenv("POD_NAMESPACE", testNamespace)
	_, err = k8s.CoreV1().ConfigMaps(testNamespace).Create(ctx, cm, metav1.CreateOptions{})
	assert.NoError(t, err)

	// Setting exists in ConfigMap
	setting, err = GetOperatorSetting(ctx, k8s, operatorSettingConfigMapName, nodeAffinity, defaultValue)
	assert.NoError(t, err)
	// Env Var exists
	assert.Equal(t, cmSettingValue, setting)
	// Env Var doesn't exist
	err = os.Unsetenv(nodeAffinity)
	assert.NoError(t, err)
	assert.Equal(t, cmSettingValue, setting)

	// Setting doesn't exist in ConfigMap
	setting, err = GetOperatorSetting(ctx, k8s, operatorSettingConfigMapName, podAffinity, defaultValue)
	assert.NoError(t, err)
	// Env Var doesn't exist
	assert.Equal(t, defaultValue, setting)
	// Env Var exists
	t.Setenv(podAffinity, envSettingValue)
	setting, err = GetOperatorSetting(ctx, k8s, operatorSettingConfigMapName, podAffinity, defaultValue)
	assert.NoError(t, err)
	assert.Equal(t, envSettingValue, setting)
}
