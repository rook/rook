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
	"testing"

	"github.com/rook/rook/pkg/daemon/ceph/client"
	"github.com/rook/rook/pkg/operator/test"

	"github.com/stretchr/testify/assert"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func Test_checkCsiCephConfigMapExists(t *testing.T) {
	namespace := "test-namespace"
	ctx := context.TODO()
	// create mocked client
	clientset := test.New(t, 3)

	ok, err := checkCsiCephConfigMapExists(ctx, clientset, namespace)
	assert.NoError(t, err)
	assert.False(t, ok)

	configMap := &v1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      configName,
			Namespace: namespace,
		},
	}
	configMap.Data = map[string]string{
		client.DefaultConfigFile: "",
	}
	_, err = clientset.CoreV1().ConfigMaps(namespace).Create(ctx, configMap, metav1.CreateOptions{})
	assert.NoError(t, err)

	ok, err = checkCsiCephConfigMapExists(ctx, clientset, namespace)
	assert.NoError(t, err)
	assert.True(t, ok)
}
