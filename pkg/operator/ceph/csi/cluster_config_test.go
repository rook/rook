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

	"github.com/stretchr/testify/assert"
)

func TestUpdateCsiClusterConfig(t *testing.T) {
	csiClusterConfigEntry := CsiClusterConfigEntry{
		Monitors: []string{"1.2.3.4:5000"},
	}
	csiClusterConfigEntry2 := CsiClusterConfigEntry{
		Monitors: []string{"20.1.1.1:5000", "20.1.1.2:5000", "20.1.1.3:5000"},
	}
	csiClusterConfigEntry3 := CsiClusterConfigEntry{
		Monitors: []string{"10.1.1.1:5000", "10.1.1.2:5000", "10.1.1.3:5000"},
		CephFS: &CsiCephFSSpec{
			SubvolumeGroup: "mygroup",
		},
	}
	var s string
	var err error

	t.Run("add a simple mons list", func(t *testing.T) {
		s, err = updateCsiClusterConfig("[]", "alpha", &csiClusterConfigEntry)
		assert.NoError(t, err)
		assert.Equal(t, s,
			`[{"clusterID":"alpha","monitors":["1.2.3.4:5000"]}]`)
	})

	t.Run("add a 2nd mon to the current cluster", func(t *testing.T) {
		csiClusterConfigEntry.Monitors = append(csiClusterConfigEntry.Monitors, "10.11.12.13:5000")
		s, err = updateCsiClusterConfig(s, "alpha", &csiClusterConfigEntry)
		assert.NoError(t, err)
		cc, err := parseCsiClusterConfig(s)
		assert.NoError(t, err)
		assert.Equal(t, 1, len(cc))
		assert.Equal(t, "alpha", cc[0].ClusterID)
		assert.Contains(t, cc[0].Monitors, "1.2.3.4:5000")
		assert.Contains(t, cc[0].Monitors, "10.11.12.13:5000")
		assert.Equal(t, 2, len(cc[0].Monitors))
	})

	t.Run("add a 2nd cluster with 3 mons", func(t *testing.T) {
		s, err = updateCsiClusterConfig(s, "beta", &csiClusterConfigEntry2)
		assert.NoError(t, err)
		cc, err := parseCsiClusterConfig(s)
		assert.NoError(t, err)
		assert.Equal(t, 2, len(cc))
		assert.Equal(t, "alpha", cc[0].ClusterID)
		assert.Contains(t, cc[0].Monitors, "1.2.3.4:5000")
		assert.Contains(t, cc[0].Monitors, "10.11.12.13:5000")
		assert.Equal(t, 2, len(cc[0].Monitors))
		assert.Equal(t, "beta", cc[1].ClusterID)
		assert.Contains(t, cc[1].Monitors, "20.1.1.1:5000")
		assert.Contains(t, cc[1].Monitors, "20.1.1.2:5000")
		assert.Contains(t, cc[1].Monitors, "20.1.1.3:5000")
		assert.Equal(t, 3, len(cc[1].Monitors))
	})

	t.Run("remove a mon from the 2nd cluster", func(t *testing.T) {
		i := 2
		// Remove last element of the slice
		csiClusterConfigEntry2.Monitors = append(csiClusterConfigEntry2.Monitors[:i], csiClusterConfigEntry2.Monitors[i+1:]...)
		s, err = updateCsiClusterConfig(s, "beta", &csiClusterConfigEntry2)
		assert.NoError(t, err)
		cc, err := parseCsiClusterConfig(s)
		assert.NoError(t, err)
		assert.Equal(t, 2, len(cc))
		assert.Equal(t, "alpha", cc[0].ClusterID)
		assert.Contains(t, cc[0].Monitors, "1.2.3.4:5000")
		assert.Contains(t, cc[0].Monitors, "10.11.12.13:5000")
		assert.Equal(t, 2, len(cc[0].Monitors))
		assert.Equal(t, "beta", cc[1].ClusterID)
		assert.Contains(t, cc[1].Monitors, "20.1.1.1:5000")
		assert.Contains(t, cc[1].Monitors, "20.1.1.2:5000")
		assert.Equal(t, 2, len(cc[1].Monitors))
	})

	t.Run("add a 3rd cluster with subvolumegroup", func(t *testing.T) {
		s, err = updateCsiClusterConfig(s, "baba", &csiClusterConfigEntry3)
		assert.NoError(t, err)
		cc, err := parseCsiClusterConfig(s)
		assert.NoError(t, err)
		assert.Equal(t, 3, len(cc))
		assert.Equal(t, "alpha", cc[0].ClusterID)
		assert.Equal(t, 2, len(cc[0].Monitors))
		assert.Equal(t, "beta", cc[1].ClusterID)
		assert.Equal(t, "baba", cc[2].ClusterID)
		assert.Equal(t, "10.1.1.1:5000", cc[2].Monitors[0])
		assert.Equal(t, 3, len(cc[2].Monitors))
		assert.Equal(t, "mygroup", cc[2].CephFS.SubvolumeGroup)

	})

	t.Run("add a 4th mon to the 3rd cluster and subvolumegroup is preserved", func(t *testing.T) {
		csiClusterConfigEntry3.Monitors = append(csiClusterConfigEntry3.Monitors, "10.11.12.13:5000")
		s, err = updateCsiClusterConfig(s, "baba", &csiClusterConfigEntry3)
		assert.NoError(t, err)
		cc, err := parseCsiClusterConfig(s)
		assert.NoError(t, err)
		assert.Equal(t, 3, len(cc))
		assert.Equal(t, 4, len(cc[2].Monitors))
		assert.Equal(t, "mygroup", cc[2].CephFS.SubvolumeGroup)
	})

	t.Run("remove subvolumegroup", func(t *testing.T) {
		csiClusterConfigEntry3.CephFS.SubvolumeGroup = ""
		s, err = updateCsiClusterConfig(s, "baba", nil)
		assert.NoError(t, err)
		cc, err := parseCsiClusterConfig(s)
		assert.NoError(t, err)
		assert.Equal(t, 2, len(cc)) // we only have 2 clusters now
	})

	t.Run("add subvolumegroup and mons after", func(t *testing.T) {
		csiClusterConfigEntry4 := CsiClusterConfigEntry{
			CephFS: &CsiCephFSSpec{
				SubvolumeGroup: "mygroup2",
			},
		}
		s, err = updateCsiClusterConfig(s, "quatre", &csiClusterConfigEntry4)
		assert.NoError(t, err)
		cc, err := parseCsiClusterConfig(s)
		assert.NoError(t, err)
		assert.Equal(t, 3, len(cc), cc)
		assert.Equal(t, 0, len(cc[2].Monitors))
		assert.Equal(t, "mygroup2", cc[2].CephFS.SubvolumeGroup, cc)

		csiClusterConfigEntry4.Monitors = []string{"10.1.1.1:5000", "10.1.1.2:5000", "10.1.1.3:5000"}
		s, err = updateCsiClusterConfig(s, "quatre", &csiClusterConfigEntry4)
		assert.NoError(t, err)
		cc, err = parseCsiClusterConfig(s)
		assert.NoError(t, err)
		assert.Equal(t, "mygroup2", cc[2].CephFS.SubvolumeGroup)
		assert.Equal(t, 3, len(cc[2].Monitors))
	})

	t.Run("does it return error on garbage input?", func(t *testing.T) {
		_, err = updateCsiClusterConfig("qqq", "beta", &csiClusterConfigEntry2)
		assert.Error(t, err)
	})
}
