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

// Package cluster to manage Kubernetes storage.
package cluster

import (
	ctx "context"
	"testing"

	"github.com/rook/rook/pkg/clusterd"
	"github.com/rook/rook/pkg/operator/k8sutil"
	"github.com/rook/rook/pkg/operator/test"
	"github.com/stretchr/testify/assert"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestConfigmapOverrideUpdate(t *testing.T) {
	// create mocked cluster context and info
	clientset := test.New(t, 3)
	context := &clusterd.Context{
		Clientset: clientset,
	}
	ns := "test"
	controllerRef := &metav1.OwnerReference{UID: "test-id"}
	ownerInfo := k8sutil.NewOwnerInfoWithOwnerRef(controllerRef, "")
	clusterMetadata := metav1.ObjectMeta{
		Namespace: ns,
	}

	// The configmap should be created without any labels/annotations when helm not installed
	err := populateConfigOverrideConfigMap(context, ns, ownerInfo, clusterMetadata)
	assert.NoError(t, err)
	cm, err := clientset.CoreV1().ConfigMaps(ns).Get(ctx.TODO(), k8sutil.ConfigOverrideName, metav1.GetOptions{})
	assert.NoError(t, err)
	assert.Equal(t, 0, len(cm.Annotations))
	assert.Equal(t, 0, len(cm.Labels))

	// Set the labels and annotations to nil and confirm helm labels are still not added
	cm.Labels = nil
	cm.Annotations = nil
	_, err = clientset.CoreV1().ConfigMaps(ns).Update(ctx.TODO(), cm, metav1.UpdateOptions{})
	assert.NoError(t, err)
	err = populateConfigOverrideConfigMap(context, ns, ownerInfo, clusterMetadata)
	assert.NoError(t, err)
	cm, err = clientset.CoreV1().ConfigMaps(ns).Get(ctx.TODO(), k8sutil.ConfigOverrideName, metav1.GetOptions{})
	assert.NoError(t, err)
	assert.Equal(t, 0, len(cm.Annotations))
	assert.Equal(t, 0, len(cm.Labels))

	// The configmap should be created with the helm annotations
	clusterMetadata.Annotations = map[string]string{"meta.helm.sh/release-name": "my-test-cluster"}
	err = populateConfigOverrideConfigMap(context, ns, ownerInfo, clusterMetadata)
	assert.NoError(t, err)
	cm, err = clientset.CoreV1().ConfigMaps(ns).Get(ctx.TODO(), k8sutil.ConfigOverrideName, metav1.GetOptions{})
	assert.NoError(t, err)
	assert.Equal(t, 2, len(cm.Annotations))
	assert.Equal(t, 1, len(cm.Labels))
	assert.Equal(t, "my-test-cluster", cm.Annotations["meta.helm.sh/release-name"])

	// Remove the labels and annotations from the configmap and verify they are added back
	// This tests the upgrade scenario where the labels and annotations will not exist
	cm.Labels = nil
	cm.Annotations = nil
	_, err = clientset.CoreV1().ConfigMaps(ns).Update(ctx.TODO(), cm, metav1.UpdateOptions{})
	assert.NoError(t, err)
	err = populateConfigOverrideConfigMap(context, ns, ownerInfo, clusterMetadata)
	assert.NoError(t, err)
	cm, err = clientset.CoreV1().ConfigMaps(ns).Get(ctx.TODO(), k8sutil.ConfigOverrideName, metav1.GetOptions{})
	assert.NoError(t, err)
	assert.Equal(t, 2, len(cm.Annotations))
	assert.Equal(t, 1, len(cm.Labels))
	assert.Equal(t, "my-test-cluster", cm.Annotations["meta.helm.sh/release-name"])

	// The helm annotations should be added even if other non-helm properties exist
	cm.Labels = map[string]string{"foo": "bar"}
	cm.Annotations = map[string]string{"hello": "world"}
	_, err = clientset.CoreV1().ConfigMaps(ns).Update(ctx.TODO(), cm, metav1.UpdateOptions{})
	assert.NoError(t, err)

	err = populateConfigOverrideConfigMap(context, ns, ownerInfo, clusterMetadata)
	assert.NoError(t, err)
	cm, err = clientset.CoreV1().ConfigMaps(ns).Get(ctx.TODO(), k8sutil.ConfigOverrideName, metav1.GetOptions{})
	assert.NoError(t, err)
	assert.Equal(t, 3, len(cm.Annotations))
	assert.Equal(t, 2, len(cm.Labels))
	assert.Equal(t, "world", cm.Annotations["hello"])
	assert.Equal(t, "my-test-cluster", cm.Annotations["meta.helm.sh/release-name"])
	assert.Equal(t, ns, cm.Annotations["meta.helm.sh/release-namespace"])
	assert.Equal(t, "bar", cm.Labels["foo"])
	assert.Equal(t, "Helm", cm.Labels["app.kubernetes.io/managed-by"])
}
