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
	"testing"

	cephv1 "github.com/rook/rook/pkg/apis/ceph.rook.io/v1"
	"github.com/stretchr/testify/assert"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestClusterMap(t *testing.T) {

	sharedClusterMap := &ClusterMap{}

	clusterInfo := sharedClusterMap.GetClusterInfo("rook-ceph-0")
	assert.Nil(t, clusterInfo)

	sharedClusterMap.UpdateClusterMap("rook-ceph-0", &cephv1.CephCluster{ObjectMeta: metav1.ObjectMeta{Name: "ceph-cluster-0"}})
	sharedClusterMap.UpdateClusterMap("rook-ceph-1", &cephv1.CephCluster{ObjectMeta: metav1.ObjectMeta{Name: "ceph-cluster-1"}})
	sharedClusterMap.UpdateClusterMap("rook-ceph-2", &cephv1.CephCluster{ObjectMeta: metav1.ObjectMeta{Name: "ceph-cluster-2"}})
	clusterInfo = sharedClusterMap.GetClusterInfo("rook-ceph-0")
	assert.NotNil(t, clusterInfo)
	assert.Equal(t, clusterInfo.NamespacedName().Name, "ceph-cluster-0")
	assert.Equal(t, clusterInfo.NamespacedName().Namespace, "rook-ceph-0")
	assert.Equal(t, clusterInfo.Namespace, "rook-ceph-0")

	clusterInfo = sharedClusterMap.GetClusterInfo("storage-namespace")
	assert.Nil(t, clusterInfo)

	namespaces := sharedClusterMap.GetClusterNamespaces()
	assert.Equal(t, 3, len(namespaces))
}
