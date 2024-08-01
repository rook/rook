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

	clientProfile, err := generateClientProfileMappingCR(fakeContext, clusterInfo, &fakeSinglePeerCephBlockPool)
	assert.NoError(t, err)

	assert.Equal(t, 1, len(clientProfile.Spec.Mappings))
	assert.Equal(t, "rook-ceph-primary", clientProfile.Spec.Mappings[0].LocalClientProfile)
	assert.Equal(t, "peer1", clientProfile.Spec.Mappings[0].RemoteClientProfile)
	assert.Equal(t, 1, len(clientProfile.Spec.Mappings[0].BlockPoolIdMapping))
	assert.Equal(t, 2, len(clientProfile.Spec.Mappings[0].BlockPoolIdMapping[0]))
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

	clientProfile, err := generateClientProfileMappingCR(fakeContext, clusterInfo, &fakeMultiPeerCephBlockPool)
	assert.NoError(t, err)

	assert.Equal(t, 3, len(clientProfile.Spec.Mappings))
	assert.Equal(t, "rook-ceph-primary", clientProfile.Spec.Mappings[0].LocalClientProfile)
	assert.Equal(t, "peer1", clientProfile.Spec.Mappings[0].RemoteClientProfile)
	assert.Equal(t, 1, len(clientProfile.Spec.Mappings[0].BlockPoolIdMapping))
	assert.Equal(t, 2, len(clientProfile.Spec.Mappings[0].BlockPoolIdMapping[0]))

	assert.Equal(t, "rook-ceph-primary", clientProfile.Spec.Mappings[1].LocalClientProfile)
	assert.Equal(t, "peer2", clientProfile.Spec.Mappings[1].RemoteClientProfile)
	assert.Equal(t, 1, len(clientProfile.Spec.Mappings[1].BlockPoolIdMapping))
	assert.Equal(t, 2, len(clientProfile.Spec.Mappings[1].BlockPoolIdMapping[0]))

	assert.Equal(t, "rook-ceph-primary", clientProfile.Spec.Mappings[2].LocalClientProfile)
	assert.Equal(t, "peer3", clientProfile.Spec.Mappings[2].RemoteClientProfile)
	assert.Equal(t, 1, len(clientProfile.Spec.Mappings[2].BlockPoolIdMapping))
	assert.Equal(t, 2, len(clientProfile.Spec.Mappings[2].BlockPoolIdMapping[0]))
}
