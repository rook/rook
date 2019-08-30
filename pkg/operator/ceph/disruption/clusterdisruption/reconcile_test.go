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

package clusterdisruption

import (
	"strconv"
	"testing"

	cephv1 "github.com/rook/rook/pkg/apis/ceph.rook.io/v1"
	"github.com/stretchr/testify/assert"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestClusterMap(t *testing.T) {

	sharedClusterMap := &ClusterMap{}

	_, found := sharedClusterMap.GetClusterName("rook-ceph-0")
	assert.False(t, found)

	sharedClusterMap.UpdateClusterMap("rook-ceph-0", &cephv1.CephCluster{ObjectMeta: metav1.ObjectMeta{Name: "ceph-cluster-0"}})
	sharedClusterMap.UpdateClusterMap("rook-ceph-1", &cephv1.CephCluster{ObjectMeta: metav1.ObjectMeta{Name: "ceph-cluster-1"}})
	sharedClusterMap.UpdateClusterMap("rook-ceph-2", &cephv1.CephCluster{ObjectMeta: metav1.ObjectMeta{Name: "ceph-cluster-2"}})
	name, found := sharedClusterMap.GetClusterName("rook-ceph-0")
	assert.True(t, found)
	assert.Equal(t, name, "ceph-cluster-0")

	_, found = sharedClusterMap.GetClusterName("storage-namespace")
	assert.False(t, found)

	for namespace, cluster := range sharedClusterMap.GetClusterMap() {
		clusterName := cluster.ObjectMeta.GetName()
		nsNum, err := strconv.Atoi(string(namespace[len(namespace)-1]))
		assert.Nil(t, err)
		nameNum, err := strconv.Atoi(string(clusterName[len(clusterName)-1]))
		assert.Nil(t, err)
		assert.Equal(t, nsNum, nameNum)
	}

}
