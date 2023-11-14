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
	"strings"
	"testing"

	cephv1 "github.com/rook/rook/pkg/apis/ceph.rook.io/v1"
	cephclient "github.com/rook/rook/pkg/daemon/ceph/client"
	"github.com/stretchr/testify/assert"
)

func TestUpdateCsiClusterConfig(t *testing.T) {
	csiClusterConfigEntry := CsiClusterConfigEntry{
		Namespace: "rook-ceph-1",
		Monitors:  []string{"1.2.3.4:5000"},
	}
	csiClusterConfigEntryMultus := CsiClusterConfigEntry{
		Namespace: "rook-ceph-1",
		Monitors:  []string{"1.2.3.4:5000"},
		RBD: &CsiRBDSpec{
			NetNamespaceFilePath: "/var/run/netns/rook-ceph-1",
			RadosNamespace:       "rook-ceph-1",
		},
	}
	csiClusterConfigEntry2 := CsiClusterConfigEntry{
		Namespace: "rook-ceph-2",
		Monitors:  []string{"20.1.1.1:5000", "20.1.1.2:5000", "20.1.1.3:5000"},
	}
	csiClusterConfigEntry3 := CsiClusterConfigEntry{
		Namespace: "rook-ceph-3",
		Monitors:  []string{"10.1.1.1:5000", "10.1.1.2:5000", "10.1.1.3:5000"},
		CephFS: &CsiCephFSSpec{
			SubvolumeGroup: "my-group",
		},
	}

	var s string
	var err error

	t.Run("add a simple mons list", func(t *testing.T) {
		s, err = updateCsiClusterConfig("[]", "rook-ceph-1", &csiClusterConfigEntry)
		assert.NoError(t, err)
		assert.Equal(t, `[{"clusterID":"rook-ceph-1","monitors":["1.2.3.4:5000"],"namespace":"rook-ceph-1"}]`, s)
	})

	t.Run("add a 2nd mon to the current cluster", func(t *testing.T) {
		csiClusterConfigEntry.Monitors = append(csiClusterConfigEntry.Monitors, "10.11.12.13:5000")
		s, err = updateCsiClusterConfig(s, "rook-ceph-1", &csiClusterConfigEntry)
		assert.NoError(t, err)
		cc, err := parseCsiClusterConfig(s)
		assert.NoError(t, err)
		assert.Equal(t, 1, len(cc))
		assert.Equal(t, "rook-ceph-1", cc[0].ClusterID)
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
		assert.Equal(t, "rook-ceph-1", cc[0].ClusterID)
		assert.Contains(t, cc[0].Monitors, "1.2.3.4:5000")
		assert.Contains(t, cc[0].Monitors, "10.11.12.13:5000")
		// check 1st cluster contains any of the 3 mons from 2nd cluster
		assert.NotContains(t, cc[0].Monitors, "20.1.1.1:5000")
		assert.Equal(t, 2, len(cc[0].Monitors))
		assert.Equal(t, "beta", cc[1].ClusterID)
		assert.Contains(t, cc[1].Monitors, "20.1.1.1:5000")
		assert.Contains(t, cc[1].Monitors, "20.1.1.2:5000")
		assert.Contains(t, cc[1].Monitors, "20.1.1.3:5000")
		// check 2nd cluster contains any of the mons from 1st cluster
		assert.NotContains(t, cc[1].Monitors, "10.11.12.13:5000")
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
		assert.Equal(t, "rook-ceph-1", cc[0].ClusterID)
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
		assert.Equal(t, "rook-ceph-1", cc[0].ClusterID)
		assert.Equal(t, 2, len(cc[0].Monitors))
		assert.Equal(t, "beta", cc[1].ClusterID)
		assert.Equal(t, "baba", cc[2].ClusterID)
		assert.Equal(t, "10.1.1.1:5000", cc[2].Monitors[0])
		assert.Equal(t, 3, len(cc[2].Monitors))
		assert.Equal(t, "my-group", cc[2].CephFS.SubvolumeGroup)

	})

	t.Run("add a 4th mon to the 3rd cluster and subvolumegroup is preserved", func(t *testing.T) {
		csiClusterConfigEntry3.Monitors = append(csiClusterConfigEntry3.Monitors, "10.11.12.13:5000")
		s, err = updateCsiClusterConfig(s, "baba", &csiClusterConfigEntry3)
		assert.NoError(t, err)
		cc, err := parseCsiClusterConfig(s)
		assert.NoError(t, err)
		assert.Equal(t, 3, len(cc))
		assert.Equal(t, 4, len(cc[2].Monitors))
		assert.Equal(t, "my-group", cc[2].CephFS.SubvolumeGroup)
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
				SubvolumeGroup: "my-group2",
			},
		}
		s, err = updateCsiClusterConfig(s, "quatre", &csiClusterConfigEntry4)
		assert.NoError(t, err)
		cc, err := parseCsiClusterConfig(s)
		assert.NoError(t, err)
		assert.Equal(t, 3, len(cc), cc)
		assert.Equal(t, 0, len(cc[2].Monitors))
		assert.Equal(t, "my-group2", cc[2].CephFS.SubvolumeGroup, cc)

		csiClusterConfigEntry4.Monitors = []string{"10.1.1.1:5000", "10.1.1.2:5000", "10.1.1.3:5000"}
		s, err = updateCsiClusterConfig(s, "quatre", &csiClusterConfigEntry4)
		assert.NoError(t, err)
		cc, err = parseCsiClusterConfig(s)
		assert.NoError(t, err)
		assert.Equal(t, "my-group2", cc[2].CephFS.SubvolumeGroup)
		assert.Equal(t, 3, len(cc[2].Monitors))
	})

	t.Run("does it return error on garbage input?", func(t *testing.T) {
		_, err = updateCsiClusterConfig("qqq", "beta", &csiClusterConfigEntry2)
		assert.Error(t, err)
	})

	t.Run("test mon IP's update across all clusterID's belong to same cluster", func(t *testing.T) {
		clusterIDofCluster1 := "rook-ceph"
		subvolGrpNameofCluster1 := "subvol-group"
		radosNSofCluster1 := "rados-ns"

		csiCluster1ConfigEntry := CsiClusterConfigEntry{
			Namespace: clusterIDofCluster1,
			Monitors:  []string{"1.2.3.4:5000"},
		}
		s, err := updateCsiClusterConfig("[]", clusterIDofCluster1, &csiCluster1ConfigEntry)
		assert.NoError(t, err)
		assert.Equal(t, s,
			`[{"clusterID":"rook-ceph","monitors":["1.2.3.4:5000"],"namespace":"rook-ceph"}]`)
		// add subvolumegroup to same cluster
		subVolCsiCluster1Config := CsiClusterConfigEntry{
			Namespace: clusterIDofCluster1,
			Monitors:  csiCluster1ConfigEntry.Monitors,
			CephFS: &CsiCephFSSpec{
				SubvolumeGroup: subvolGrpNameofCluster1,
			},
		}
		s, err = updateCsiClusterConfig(s, subvolGrpNameofCluster1, &subVolCsiCluster1Config)
		assert.NoError(t, err)
		cc, err := parseCsiClusterConfig(s)
		assert.NoError(t, err)
		assert.Equal(t, 2, len(cc), cc)
		assert.Equal(t, 1, len(cc[1].Monitors))
		assert.Equal(t, subvolGrpNameofCluster1, cc[1].CephFS.SubvolumeGroup, cc)

		// add rados to same cluster
		radosNsCsiCluster1Config := CsiClusterConfigEntry{
			Namespace:      clusterIDofCluster1,
			Monitors:       csiCluster1ConfigEntry.Monitors,
			RadosNamespace: radosNSofCluster1,
			RBD: &CsiRBDSpec{
				RadosNamespace: radosNSofCluster1,
			},
		}
		s, err = updateCsiClusterConfig(s, radosNSofCluster1, &radosNsCsiCluster1Config)
		assert.NoError(t, err)
		cc, err = parseCsiClusterConfig(s)
		assert.NoError(t, err)
		assert.Equal(t, 3, len(cc), cc)
		assert.Equal(t, 1, len(cc[2].Monitors))
		// Now the configuration of new entries goes into RBD.RadosNamespace so it should be empty
		assert.Empty(t, cc[2].RadosNamespace, cc)
		assert.Equal(t, radosNSofCluster1, cc[2].RBD.RadosNamespace, cc)

		// update mon IP's and check is it updating for all clusterID's
		csiCluster1ConfigEntry.Monitors = append(csiCluster1ConfigEntry.Monitors, "1.2.3.10:5000")
		s, err = updateCsiClusterConfig(s, clusterIDofCluster1, &csiCluster1ConfigEntry)
		assert.NoError(t, err)
		cc, err = parseCsiClusterConfig(s)
		assert.NoError(t, err)
		assert.Equal(t, 3, len(cc), cc)
		// check mon entries are updated in all configmap
		for _, i := range cc {
			assert.Equal(t, 2, len(i.Monitors))
		}

		clusterIDofCluster2 := "rook-ceph-2"
		subvolGrpNameofCluster2 := "subvol-group-2"
		radosNSofCluster2 := "rados-ns-2"

		cluster2Mons := []string{"192.168.0.2:5000"}
		csiCluster2ConfigEntry := CsiClusterConfigEntry{
			Namespace: clusterIDofCluster2,
			Monitors:  cluster2Mons,
		}
		subVolCsiCluster2Config := CsiClusterConfigEntry{
			Namespace: clusterIDofCluster2,
			Monitors:  cluster2Mons,
			CephFS: &CsiCephFSSpec{
				SubvolumeGroup: subvolGrpNameofCluster2,
			},
		}
		radosNsCsiCluster2Config := CsiClusterConfigEntry{
			Namespace:      clusterIDofCluster2,
			Monitors:       cluster2Mons,
			RadosNamespace: radosNSofCluster2,
		}
		s, err = updateCsiClusterConfig(s, clusterIDofCluster2, &csiCluster2ConfigEntry)
		assert.NoError(t, err)
		s, err = updateCsiClusterConfig(s, subvolGrpNameofCluster2, &subVolCsiCluster2Config)
		assert.NoError(t, err)
		s, err = updateCsiClusterConfig(s, radosNSofCluster2, &radosNsCsiCluster2Config)
		assert.NoError(t, err)
		cc, err = parseCsiClusterConfig(s)
		assert.NoError(t, err)
		assert.Equal(t, 6, len(cc), cc)
		// check mon entries of 1st cluster amd mon overlapping with 2nd cluster
		for i := 0; i < 3; i++ {
			assert.Equal(t, 2, len(cc[i].Monitors))
			assert.False(t, contains(cc[i].Monitors, cluster2Mons))

		}
		// check mon entries of 2nd cluster
		for i := 3; i < 6; i++ {
			assert.Equal(t, 1, len(cc[i].Monitors))
			assert.False(t, contains(cc[i].Monitors, csiCluster1ConfigEntry.Monitors))
		}
		// update mon on 2nd cluster and check is it updating for all clusterID's
		// of 2nd cluster
		csiCluster2ConfigEntry.Monitors = append(csiCluster2ConfigEntry.Monitors, "192.168.0.3:5000")
		s, err = updateCsiClusterConfig(s, clusterIDofCluster2, &csiCluster2ConfigEntry)
		assert.NoError(t, err)
		cc, err = parseCsiClusterConfig(s)
		assert.NoError(t, err)
		assert.Equal(t, 6, len(cc), cc)
		// check mon entries are updated
		for _, i := range cc {
			assert.Equal(t, 2, len(i.Monitors))
		}
		// check for overlapping
		// check mon entries of 1st cluster amd mon overlapping with 2nd cluster
		for i := 0; i < 3; i++ {
			assert.False(t, contains(cc[i].Monitors, cluster2Mons))
		}
		// check mon entries of 2nd cluster
		for i := 3; i < 6; i++ {
			assert.False(t, contains(cc[i].Monitors, csiCluster1ConfigEntry.Monitors))
		}
		// Remove one mon from 2nd cluster and check is it updating for all
		// clusterID's of 2nd cluster
		i := 1
		csiCluster2ConfigEntry.Monitors = append(csiCluster2ConfigEntry.Monitors[:i], csiCluster2ConfigEntry.Monitors[i+1:]...)
		s, err = updateCsiClusterConfig(s, clusterIDofCluster2, &csiCluster2ConfigEntry)
		assert.NoError(t, err)
		cc, err = parseCsiClusterConfig(s)
		assert.NoError(t, err)
		assert.Equal(t, 6, len(cc), cc)
		// check for overlapping
		// check mon entries of 1st cluster amd mon overlapping with 2nd cluster
		for i := 0; i < 3; i++ {
			assert.False(t, contains(cc[i].Monitors, cluster2Mons))
			assert.Equal(t, 2, len(cc[i].Monitors))
		}
		// check mon entries of 2nd cluster
		for i := 3; i < 6; i++ {
			assert.False(t, contains(cc[i].Monitors, csiCluster1ConfigEntry.Monitors))
			assert.Equal(t, 1, len(cc[i].Monitors))
		}
	})

	t.Run("test multus cluster", func(t *testing.T) {
		s, err = updateCsiClusterConfig("[]", "rook-ceph-1", &csiClusterConfigEntryMultus)
		assert.NoError(t, err)
		assert.Equal(t, `[{"clusterID":"rook-ceph-1","monitors":["1.2.3.4:5000"],"namespace":"rook-ceph-1","rbd":{"netNamespaceFilePath":"/var/run/netns/rook-ceph-1","radosNamespace":"rook-ceph-1"}}]`, s)
	})

}

func contains(src, dest []string) bool {
	for _, s := range src {
		for _, d := range dest {
			if s == d {
				return true
			}
		}
	}

	return false
}

func TestMonEndpoints(t *testing.T) {
	monInfo := map[string]*cephclient.MonInfo{
		"a": {Name: "a", Endpoint: "1.2.3.4:6789"},
		"b": {Name: "b", Endpoint: "1.2.3.5:6789"},
		"c": {Name: "c", Endpoint: "1.2.3.6:6789"},
	}

	t.Run("msgrv1 when not require msgr2", func(t *testing.T) {
		endpoints := MonEndpoints(monInfo, false)
		assert.Equal(t, 3, len(endpoints))
		verifyEndpointPort(t, endpoints, "6789")
	})

	t.Run("convert to msgr2", func(t *testing.T) {
		endpoints := MonEndpoints(monInfo, true)
		assert.Equal(t, 3, len(endpoints))
		verifyEndpointPort(t, endpoints, "3300")
	})

	t.Run("remains msgr2", func(t *testing.T) {
		monInfo := map[string]*cephclient.MonInfo{
			"a": {Name: "a", Endpoint: "1.2.3.4:3300"},
			"b": {Name: "b", Endpoint: "1.2.3.5:3300"},
			"c": {Name: "c", Endpoint: "1.2.3.6:3300"},
		}
		endpoints := MonEndpoints(monInfo, false)
		assert.Equal(t, 3, len(endpoints))
		verifyEndpointPort(t, endpoints, "3300")
	})

	t.Run("ipv6 endpoint conversion", func(t *testing.T) {
		monInfo := map[string]*cephclient.MonInfo{
			"a": {Name: "a", Endpoint: "[fd07:aaaa:bbbb:cccc::11]:6789"},
			"b": {Name: "a", Endpoint: "[1234:6789:bbbb:cccc::11]:6789"},
		}
		endpoints := MonEndpoints(monInfo, true)
		assert.Equal(t, 2, len(endpoints))
		verifyEndpointPort(t, endpoints, "3300")
		for _, endpoint := range endpoints {
			// Verify that the v1 port inside the ipv6 address will not be replaced
			if strings.HasPrefix(endpoint, "[1234") {
				assert.True(t, strings.HasPrefix(endpoint, "[1234:6789"))
			}
		}
	})
}

func verifyEndpointPort(t *testing.T, endpoints []string, expectedPort string) {
	for _, endpoint := range endpoints {
		assert.True(t, strings.HasSuffix(endpoint, expectedPort))
	}
}

func TestUpdateCSIDriverOptions(t *testing.T) {
	type args struct {
		clusterConfig    csiClusterConfig
		clusterKey       string
		csiDriverOptions *cephv1.CSIDriverSpec
	}
	tests := []struct {
		name    string
		args    args
		want    csiClusterConfig
		wantErr bool
	}{
		{
			name: "empty current config",
			args: args{
				clusterConfig:    []CsiClusterConfigEntry{},
				clusterKey:       "rook-ceph",
				csiDriverOptions: &cephv1.CSIDriverSpec{},
			},
			want:    []CsiClusterConfigEntry{},
			wantErr: false,
		},
		{
			name: "single matching current config",
			args: args{
				clusterConfig: []CsiClusterConfigEntry{
					{
						ClusterID: "rook-ceph",
						Namespace: "rook-ceph",
						Monitors:  []string{"1.1.1.1"},
					},
				},
				clusterKey: "rook-ceph",
				csiDriverOptions: &cephv1.CSIDriverSpec{
					ReadAffinity: cephv1.ReadAffinitySpec{
						Enabled:             true,
						CrushLocationLabels: []string{"topology.rook.io/rack"},
					},
					CephFS: cephv1.CSICephFSSpec{
						KernelMountOptions: "rw,noatime",
						FuseMountOptions:   "debug",
					},
				},
			},
			want: []CsiClusterConfigEntry{
				{
					ClusterID: "rook-ceph",
					Namespace: "rook-ceph",
					Monitors:  []string{"1.1.1.1"},
					ReadAffinity: &cephv1.ReadAffinitySpec{
						Enabled:             true,
						CrushLocationLabels: []string{"topology.rook.io/rack"},
					},
					CephFS: &CsiCephFSSpec{
						KernelMountOptions: "rw,noatime",
						FuseMountOptions:   "debug",
					},
				},
			},
			wantErr: false,
		},
		{
			name: "multiple matching current config",
			args: args{
				clusterConfig: []CsiClusterConfigEntry{
					{
						ClusterID: "rook-ceph",
						Namespace: "rook-ceph",
						Monitors:  []string{"1.1.1.1"},
					},
					{
						ClusterID: "rook-ceph-2",
						Namespace: "rook-ceph",
						Monitors:  []string{"1.1.1.1"},
					},
					{
						ClusterID: "rook-ceph-3",
						Namespace: "rook-ceph-1",
						Monitors:  []string{"1.1.1.1"},
					},
				},
				clusterKey: "rook-ceph",
				csiDriverOptions: &cephv1.CSIDriverSpec{
					ReadAffinity: cephv1.ReadAffinitySpec{
						Enabled:             true,
						CrushLocationLabels: []string{"topology.rook.io/rack"},
					},
					CephFS: cephv1.CSICephFSSpec{
						KernelMountOptions: "rw,noatime",
						FuseMountOptions:   "debug",
					},
				},
			},
			want: []CsiClusterConfigEntry{
				{
					ClusterID: "rook-ceph",
					Namespace: "rook-ceph",
					Monitors:  []string{"1.1.1.1"},
					ReadAffinity: &cephv1.ReadAffinitySpec{
						Enabled:             true,
						CrushLocationLabels: []string{"topology.rook.io/rack"},
					},
					CephFS: &CsiCephFSSpec{
						KernelMountOptions: "rw,noatime",
						FuseMountOptions:   "debug",
					},
				},
				{
					ClusterID: "rook-ceph-2",
					Namespace: "rook-ceph",
					Monitors:  []string{"1.1.1.1"},
					ReadAffinity: &cephv1.ReadAffinitySpec{
						Enabled:             true,
						CrushLocationLabels: []string{"topology.rook.io/rack"},
					},
					CephFS: &CsiCephFSSpec{
						KernelMountOptions: "rw,noatime",
						FuseMountOptions:   "debug",
					},
				},
				{
					ClusterID: "rook-ceph-3",
					Namespace: "rook-ceph-1",
					Monitors:  []string{"1.1.1.1"},
				},
			},
			wantErr: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dataString, err := formatCsiClusterConfig(tt.args.clusterConfig)
			assert.NoError(t, err)
			got, err := updateCSIDriverOptions(dataString, tt.args.clusterKey, tt.args.csiDriverOptions)
			if (err != nil) != tt.wantErr {
				t.Errorf("updateCSIDriverOptions() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			expectedString, err := formatCsiClusterConfig(tt.want)
			assert.NoError(t, err)
			assert.Equal(t, expectedString, got)
		})
	}
}
