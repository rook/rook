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

package cluster

import (
	"testing"

	"github.com/stretchr/testify/assert"

	cephv1 "github.com/rook/rook/pkg/apis/ceph.rook.io/v1"
	rookfake "github.com/rook/rook/pkg/client/clientset/versioned/fake"
	"github.com/rook/rook/pkg/clusterd"
	"github.com/rook/rook/pkg/daemon/ceph/agent/flexvolume/attachment"
	testop "github.com/rook/rook/pkg/operator/test"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestCleanUpJobSpec(t *testing.T) {
	expectedHostPath := "var/lib/rook"
	expectedNamespace := "test-rook-ceph"
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
	clientset := testop.New(t, 3)
	context := &clusterd.Context{
		Clientset:     clientset,
		RookClientset: rookfake.NewSimpleClientset(),
	}
	operatorConfigCallbacks := []func() error{
		func() error {
			logger.Infof("test success callback")
			return nil
		},
	}
	addCallbacks := []func() error{
		func() error {
			logger.Infof("test success callback")
			return nil
		},
	}
	controller := NewClusterController(context, "", &attachment.MockAttachment{}, operatorConfigCallbacks, addCallbacks)
	podTemplateSpec := controller.cleanUpJobTemplateSpec(cluster, "monSecret", "28b87851-8dc1-46c8-b1ec-90ec51a47c89")
	assert.Equal(t, expectedHostPath, podTemplateSpec.Spec.Containers[0].Env[0].Value)
	assert.Equal(t, expectedNamespace, podTemplateSpec.Spec.Containers[0].Env[1].Value)
}
