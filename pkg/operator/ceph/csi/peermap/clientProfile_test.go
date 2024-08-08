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

package peermap

import (
	"context"
	"testing"

	"github.com/rook/rook/pkg/clusterd"
	cephclient "github.com/rook/rook/pkg/daemon/ceph/client"
	"github.com/rook/rook/pkg/operator/test"
	"github.com/stretchr/testify/assert"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestGenerateClientProfileMappings(t *testing.T) {
	clusterInfo := cephclient.AdminTestClusterInfo(ns)
	fakeContext := &clusterd.Context{
		Executor:  mockExecutor,
		Clientset: test.New(t, 3),
	}

	// create fake secret with "peer1" cluster token
	_, err := fakeContext.Clientset.CoreV1().Secrets(ns).Create(context.TODO(), &peer1Secret, metav1.CreateOptions{})
	assert.NoError(t, err)

	clientProfile, err := generateClientProfileMappingsCR(fakeContext, clusterInfo, &fakeSinglePeerCephBlockPool)
	assert.NoError(t, err)

	assert.Equal(t, 1, len(clientProfile.Spec.BlockPoolMapping))
	assert.Equal(t, "rook-ceph-primary", clientProfile.Spec.BlockPoolMapping[0].Local.ClientProfileName)
	assert.Equal(t, "peer1", clientProfile.Spec.BlockPoolMapping[0].Remote.ClientProfileName)
	assert.Equal(t, 1, clientProfile.Spec.BlockPoolMapping[0].Local.PoolId)
	assert.Equal(t, 2, clientProfile.Spec.BlockPoolMapping[0].Remote.PoolId)
}

func TestGenerateClientProfileMappingsWithMultiplePeers(t *testing.T) {
	clusterInfo := cephclient.AdminTestClusterInfo(ns)
	fakeContext := &clusterd.Context{
		Executor:  mockExecutor,
		Clientset: test.New(t, 3),
	}

	// create fake secret with "peer1" cluster token
	_, err := fakeContext.Clientset.CoreV1().Secrets(ns).Create(context.TODO(), &peer1Secret, metav1.CreateOptions{})
	assert.NoError(t, err)

	// create fake secret with "peer2" cluster token
	_, err = fakeContext.Clientset.CoreV1().Secrets(ns).Create(context.TODO(), &peer2Secret, metav1.CreateOptions{})
	assert.NoError(t, err)

	// create fake secret with "peer3" cluster token
	_, err = fakeContext.Clientset.CoreV1().Secrets(ns).Create(context.TODO(), &peer3Secret, metav1.CreateOptions{})
	assert.NoError(t, err)

	clientProfile, err := generateClientProfileMappingsCR(fakeContext, clusterInfo, &fakeMultiPeerCephBlockPool)
	assert.NoError(t, err)

	assert.Equal(t, 3, len(clientProfile.Spec.BlockPoolMapping))
	assert.Equal(t, "rook-ceph-primary", clientProfile.Spec.BlockPoolMapping[0].Local.ClientProfileName)
	assert.Equal(t, "peer1", clientProfile.Spec.BlockPoolMapping[0].Remote.ClientProfileName)
	assert.Equal(t, 1, clientProfile.Spec.BlockPoolMapping[0].Local.PoolId)
	assert.Equal(t, 2, clientProfile.Spec.BlockPoolMapping[0].Remote.PoolId)

	assert.Equal(t, "rook-ceph-primary", clientProfile.Spec.BlockPoolMapping[1].Local.ClientProfileName)
	assert.Equal(t, "peer2", clientProfile.Spec.BlockPoolMapping[1].Remote.ClientProfileName)
	assert.Equal(t, 1, clientProfile.Spec.BlockPoolMapping[1].Local.PoolId)
	assert.Equal(t, 3, clientProfile.Spec.BlockPoolMapping[1].Remote.PoolId)

	assert.Equal(t, "rook-ceph-primary", clientProfile.Spec.BlockPoolMapping[2].Local.ClientProfileName)
	assert.Equal(t, "peer3", clientProfile.Spec.BlockPoolMapping[2].Remote.ClientProfileName)
	assert.Equal(t, 1, clientProfile.Spec.BlockPoolMapping[2].Local.PoolId)
	assert.Equal(t, 4, clientProfile.Spec.BlockPoolMapping[2].Remote.PoolId)
}
