/*
Copyright 2016 The Rook Authors. All rights reserved.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

	http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.

Some of the code below came from https://github.com/coreos/etcd-operator
which also has the apache 2.0 license.
*/
package operator

import (
	"testing"

	"github.com/rook/rook/pkg/clusterd"
	"github.com/rook/rook/pkg/operator/cluster"
	"github.com/stretchr/testify/assert"
)

func TestTrackCluster(t *testing.T) {
	context := &clusterd.Context{}
	mgr := newClusterManager(context, []inclusterInitiator{})

	c1 := &cluster.Cluster{}
	c1.Name = "myname"
	c1.Namespace = "myns"
	c1.ResourceVersion = "23"

	// not tracked yet
	checkClusterTracked(t, mgr, c1, false, false)

	// track the cluster
	err := mgr.startTrack(c1)
	assert.Nil(t, err)
	checkClusterTracked(t, mgr, c1, true, true)

	// do not support muliple clusters in the same namespace
	c2 := &cluster.Cluster{}
	c2.Name = "myothername"
	c2.Namespace = "myns"
	c2.ResourceVersion = "24"
	err = mgr.startTrack(c2)
	assert.NotNil(t, err)
	checkClusterTracked(t, mgr, c2, false, true)

	// stop tracking the cluster
	mgr.stopTrack(c1)
	checkClusterTracked(t, mgr, c1, false, false)
}

func checkClusterTracked(t *testing.T, mgr *clusterManager, c *cluster.Cluster, nameTracked, namespaceTracked bool) {
	trackedCluster, clusterOK := mgr.clusters[c.Namespace]
	version, trackerOK := mgr.tracker.clusterRVs[c.Namespace]
	assert.Equal(t, namespaceTracked, clusterOK)
	assert.Equal(t, namespaceTracked, trackerOK)
	if nameTracked {
		assert.Equal(t, c.Name, trackedCluster.Name)
		assert.Equal(t, c.ResourceVersion, version)
	}
	if namespaceTracked && !nameTracked {
		assert.NotEqual(t, c.Name, trackedCluster.Name)
		assert.NotEqual(t, c.ResourceVersion, version)
	}
}
