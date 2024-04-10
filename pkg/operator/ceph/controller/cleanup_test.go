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

package controller

import (
	"testing"

	cephv1 "github.com/rook/rook/pkg/apis/ceph.rook.io/v1"
	"github.com/stretchr/testify/assert"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestJobTemplateSpec(t *testing.T) {
	expectedHostPath := "var/lib/rook"
	expectedNamespace := "test-rook-ceph"
	rookImage := "test"
	cluster := &cephv1.CephCluster{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: expectedNamespace,
		},
		Spec: cephv1.ClusterSpec{
			DataDirHostPath: expectedHostPath,
			CleanupPolicy: cephv1.CleanupPolicySpec{
				Confirmation: "yes-really-destroy-data",
			},
		},
	}
	svgObj := &cephv1.CephFilesystemSubVolumeGroup{
		TypeMeta: metav1.TypeMeta{
			Kind: "CephFSSubvolumeGroup",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-svg",
			Namespace: expectedNamespace,
		},
	}
	testConfig := map[string]string{
		"config1": "value1",
		"config2": "value2",
	}
	cleanup := NewResourceCleanup(svgObj, cluster, rookImage, testConfig)
	podTemplateSpec := cleanup.jobTemplateSpec()
	assert.Equal(t, "CephFSSubvolumeGroup", podTemplateSpec.Spec.Containers[0].Args[2])
	assert.Equal(t, "config1", podTemplateSpec.Spec.Containers[0].Env[3].Name)
	assert.Equal(t, "value1", podTemplateSpec.Spec.Containers[0].Env[3].Value)
	assert.Equal(t, "config2", podTemplateSpec.Spec.Containers[0].Env[4].Name)
	assert.Equal(t, "value2", podTemplateSpec.Spec.Containers[0].Env[4].Value)
}

func TestForceDeleteRequested(t *testing.T) {
	svgObj := &cephv1.CephFilesystemSubVolumeGroup{
		TypeMeta: metav1.TypeMeta{
			Kind: "CephFSSubvolumeGroup",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:        "test-svg",
			Namespace:   "test",
			Annotations: map[string]string{},
		},
	}

	result := ForceDeleteRequested(svgObj.Annotations)
	assert.False(t, result)

	svgObj.Annotations[RESOURCE_CLEANUP_ANNOTATION] = "true"
	result = ForceDeleteRequested(svgObj.Annotations)
	assert.True(t, result)
}
