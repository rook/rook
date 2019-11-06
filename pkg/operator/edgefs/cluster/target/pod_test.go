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

package target

import (
	"testing"

	edgefsv1 "github.com/rook/rook/pkg/apis/edgefs.rook.io/v1"
	rookalpha "github.com/rook/rook/pkg/apis/rook.io/v1alpha2"
	"github.com/rook/rook/pkg/clusterd"
	"github.com/rook/rook/pkg/operator/edgefs/cluster/target/config"
	exectest "github.com/rook/rook/pkg/util/exec/test"
	"github.com/stretchr/testify/assert"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"
)

func TestStorageSpecConfig(t *testing.T) {
	storageSpec := rookalpha.StorageScopeSpec{
		Config: map[string]string{
			"useAllSSD":   "false",
			"useBCacheWB": "true",
			"useBCache":   "true",
		},
		Nodes: []rookalpha.Node{
			{
				Name: "node1",
				Config: map[string]string{
					"rtTransport":        edgefsv1.DeploymentRtrd,
					"useAllSSD":          "true",
					"useMetadataOffload": "false",
				},
				Selection: rookalpha.Selection{
					Devices: []rookalpha.Device{{Name: "sda"}, {Name: "sdb"}},
				},
				Resources: v1.ResourceRequirements{
					Limits: v1.ResourceList{
						v1.ResourceCPU: *resource.NewQuantity(100.0, resource.BinarySI),
					},
					Requests: v1.ResourceList{
						v1.ResourceMemory: *resource.NewQuantity(1337.0, resource.BinarySI),
					},
				},
			},
		},
	}

	clientset := fake.NewSimpleClientset()
	deploymentConfig := edgefsv1.ClusterDeploymentConfig{}
	c := New(&clusterd.Context{Clientset: clientset, ConfigDir: "/var/lib/rook", Executor: &exectest.MockExecutor{}}, "ns", "rook/rook:myversion", "",
		storageSpec, "", *resource.NewQuantity(100000.0, resource.BinarySI),
		rookalpha.Annotations{}, rookalpha.Placement{}, rookalpha.NetworkSpec{}, v1.ResourceRequirements{}, "", *resource.NewQuantity(0.0, resource.BinarySI),
		metav1.OwnerReference{}, deploymentConfig, false)

	n := c.Storage.ResolveNode(storageSpec.Nodes[0].Name)
	storeConfig := config.ToStoreConfig(storageSpec.Nodes[0].Config)

	// First Node's useAllSSD should override global useAllSSD value, -> should be different
	assert.NotEqual(t, storageSpec.Config["useAllSSD"], storeConfig.UseAllSSD)
	// useBCacheWB global in StorageSpec and does override config' UseBCacheWB prop, -> should be the same
	assert.NotEqual(t, storageSpec.Config["useBCacheWB"], storeConfig.UseBCacheWB)
	// useBCache global in StorageSpec and does override config' UseBCache prop, -> should be the same
	assert.NotEqual(t, storageSpec.Config["useBCache"], storeConfig.UseBCache)
	// UseMetadataOffload will override default value of true
	assert.Equal(t, storeConfig.UseMetadataOffload, false)

	//check default config options
	assert.Equal(t, storeConfig.RtVerifyChid, 1)
	assert.Equal(t, storeConfig.LmdbPageSize, 16384)
	assert.Equal(t, storeConfig.UseMetadataMask, "0xff")
	assert.Equal(t, storeConfig.RtPLevelOverride, 0)
	assert.Equal(t, storeConfig.Sync, 1)

	logger.Infof("Node Config is %+v", n.Config)
	logger.Infof("storeConfig is %+v", storeConfig)
}
