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

package csi

import (
	"testing"

	cephclient "github.com/rook/rook/pkg/daemon/ceph/client"
	"github.com/stretchr/testify/assert"
)

func TestUpdateCsiClusterConfig(t *testing.T) {
	// initialize an empty list & add a simple mons list
	mons := map[string]*cephclient.MonInfo{
		"foo": {Name: "foo", Endpoint: "1.2.3.4:5000"},
	}
	s, err := updateCsiClusterConfig("[]", "alpha", mons)
	assert.NoError(t, err)
	assert.Equal(t, s,
		`[{"clusterID":"alpha","monitors":["1.2.3.4:5000"]}]`)

	// add a 2nd mon to the current cluster
	mons["bar"] = &cephclient.MonInfo{
		Name: "bar", Endpoint: "10.11.12.13:5000"}
	s, err = updateCsiClusterConfig(s, "alpha", mons)
	assert.NoError(t, err)
	cc, err := parseCsiClusterConfig(s)
	assert.NoError(t, err)
	assert.Equal(t, len(cc), 1)
	assert.Equal(t, cc[0].ClusterID, "alpha")
	assert.Contains(t, cc[0].Monitors, "1.2.3.4:5000")
	assert.Contains(t, cc[0].Monitors, "10.11.12.13:5000")
	assert.Equal(t, len(cc[0].Monitors), 2)

	// add a 2nd cluster with 3 mons
	mons2 := map[string]*cephclient.MonInfo{
		"flim": {Name: "flim", Endpoint: "20.1.1.1:5000"},
		"flam": {Name: "flam", Endpoint: "20.1.1.2:5000"},
		"blam": {Name: "blam", Endpoint: "20.1.1.3:5000"},
	}
	s, err = updateCsiClusterConfig(s, "beta", mons2)
	assert.NoError(t, err)
	cc, err = parseCsiClusterConfig(s)
	assert.NoError(t, err)
	assert.Equal(t, len(cc), 2)
	assert.Equal(t, cc[0].ClusterID, "alpha")
	assert.Contains(t, cc[0].Monitors, "1.2.3.4:5000")
	assert.Contains(t, cc[0].Monitors, "10.11.12.13:5000")
	assert.Equal(t, len(cc[0].Monitors), 2)
	assert.Equal(t, cc[1].ClusterID, "beta")
	assert.Contains(t, cc[1].Monitors, "20.1.1.1:5000")
	assert.Contains(t, cc[1].Monitors, "20.1.1.2:5000")
	assert.Contains(t, cc[1].Monitors, "20.1.1.3:5000")
	assert.Equal(t, len(cc[1].Monitors), 3)

	// remove a mon from the 2nd cluster
	delete(mons2, "blam")
	s, err = updateCsiClusterConfig(s, "beta", mons2)
	assert.NoError(t, err)
	cc, err = parseCsiClusterConfig(s)
	assert.NoError(t, err)
	assert.Equal(t, len(cc), 2)
	assert.Equal(t, cc[0].ClusterID, "alpha")
	assert.Contains(t, cc[0].Monitors, "1.2.3.4:5000")
	assert.Contains(t, cc[0].Monitors, "10.11.12.13:5000")
	assert.Equal(t, len(cc[0].Monitors), 2)
	assert.Equal(t, cc[1].ClusterID, "beta")
	assert.Contains(t, cc[1].Monitors, "20.1.1.1:5000")
	assert.Contains(t, cc[1].Monitors, "20.1.1.2:5000")
	assert.Equal(t, len(cc[1].Monitors), 2)

	// does it return error on garbage input?
	_, err = updateCsiClusterConfig("qqq", "beta", mons2)
	assert.Error(t, err)
}
