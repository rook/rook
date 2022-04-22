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
	"context"
	"os"
	"testing"

	"github.com/pkg/errors"
	cephv1 "github.com/rook/rook/pkg/apis/ceph.rook.io/v1"
	"github.com/rook/rook/pkg/daemon/ceph/client"
	"github.com/rook/rook/pkg/operator/k8sutil"
	"github.com/rook/rook/pkg/operator/test"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
)

func TestUpdateCsiClusterConfig(t *testing.T) {
	csiClusterConfigEntry := &CsiClusterConfigEntry{
		ClusterID: "alpha",
		Monitors:  []string{"1.2.3.4:5000"},
	}
	csiClusterConfigEntry2 := &CsiClusterConfigEntry{
		ClusterID: "beta",
		Monitors:  []string{"20.1.1.1:5000", "20.1.1.2:5000", "20.1.1.3:5000"},
	}
	clientset := test.New(t, 3)
	clusterInfo := &client.ClusterInfo{Context: context.TODO()}
	clusterNamespace := "ns"
	os.Setenv("POD_NAMESPACE", clusterNamespace)
	defer os.Unsetenv("POD_NAMESPACE")
	// Create the CSI config map
	ownerRef := &metav1.OwnerReference{}
	ownerInfo := k8sutil.NewOwnerInfoWithOwnerRef(ownerRef, "")
	err := CreateCsiConfigMap(clusterInfo.Context, clusterNamespace, clientset, ownerInfo)
	assert.NoError(t, err)

	t.Run("add a simple mons list", func(t *testing.T) {
		err := SaveClusterConfig(clientset, clusterInfo, csiClusterConfigEntry)
		assert.NoError(t, err)
		cc, err := getTestConfigmapContents(clientset, clusterInfo)
		assert.NoError(t, err)
		require.Equal(t, 1, len(cc))
		assert.Equal(t, "alpha", cc[0].ClusterID)
		assert.Contains(t, cc[0].Monitors, "1.2.3.4:5000")
	})

	t.Run("add a 2nd mon to the current cluster", func(t *testing.T) {
		csiClusterConfigEntry.Monitors = append(csiClusterConfigEntry.Monitors, "10.11.12.13:5000")
		err := SaveClusterConfig(clientset, clusterInfo, csiClusterConfigEntry)
		assert.NoError(t, err)
		cc, err := getTestConfigmapContents(clientset, clusterInfo)
		assert.NoError(t, err)
		require.Equal(t, 1, len(cc))
		assert.Equal(t, "alpha", cc[0].ClusterID)
		assert.Contains(t, cc[0].Monitors, "1.2.3.4:5000")
		assert.Contains(t, cc[0].Monitors, "10.11.12.13:5000")
		assert.Equal(t, 2, len(cc[0].Monitors))
	})

	t.Run("add a 2nd cluster with 3 mons", func(t *testing.T) {
		err := SaveClusterConfig(clientset, clusterInfo, csiClusterConfigEntry2)
		assert.NoError(t, err)
		cc, err := getTestConfigmapContents(clientset, clusterInfo)
		assert.NoError(t, err)
		require.Equal(t, 2, len(cc))
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
		err := SaveClusterConfig(clientset, clusterInfo, csiClusterConfigEntry2)
		assert.NoError(t, err)
		cc, err := getTestConfigmapContents(clientset, clusterInfo)
		assert.NoError(t, err)
		require.Equal(t, 2, len(cc))
		assert.Equal(t, "alpha", cc[0].ClusterID)
		assert.Contains(t, cc[0].Monitors, "1.2.3.4:5000")
		assert.Contains(t, cc[0].Monitors, "10.11.12.13:5000")
		require.Equal(t, 2, len(cc[0].Monitors))
		assert.Equal(t, "beta", cc[1].ClusterID)
		assert.Contains(t, cc[1].Monitors, "20.1.1.1:5000")
		assert.Contains(t, cc[1].Monitors, "20.1.1.2:5000")
		assert.Equal(t, 2, len(cc[1].Monitors))
	})

	t.Run("add a subvolumegroup", func(t *testing.T) {
		group := &cephv1.CephFilesystemSubVolumeGroup{
			ObjectMeta: metav1.ObjectMeta{Name: "mygroup", Namespace: csiClusterConfigEntry2.ClusterID},
			Spec: cephv1.CephFilesystemSubVolumeGroupSpec{
				FilesystemName: "testfs",
			},
		}
		err = AddSubvolumeGroup(clientset, clusterInfo, group)
		assert.NoError(t, err)

		cc, err := getTestConfigmapContents(clientset, clusterInfo)
		assert.NoError(t, err)
		assert.Equal(t, 2, len(cc))
		assert.Equal(t, "alpha", cc[0].ClusterID)
		assert.Equal(t, 2, len(cc[0].Monitors))
		assert.Equal(t, "beta", cc[1].ClusterID)
		assert.Equal(t, "20.1.1.1:5000", cc[1].Monitors[0])
		assert.Equal(t, 2, len(cc[1].Monitors))
		assert.Equal(t, "mygroup", cc[1].SubvolumeGroups[0].Name)
		assert.Equal(t, "testfs", cc[1].SubvolumeGroups[0].Filesystem)
	})

	t.Run("add another mon and subvolumegroup is preserved", func(t *testing.T) {
		csiClusterConfigEntry2.Monitors = append(csiClusterConfigEntry2.Monitors, "10.11.12.13:5000")
		err := SaveClusterConfig(clientset, clusterInfo, csiClusterConfigEntry2)
		assert.NoError(t, err)
		cc, err := getTestConfigmapContents(clientset, clusterInfo)
		assert.NoError(t, err)
		assert.Equal(t, 2, len(cc))
		assert.Equal(t, 3, len(cc[1].Monitors))
		assert.Equal(t, "10.11.12.13:5000", cc[1].Monitors[2])
		assert.Equal(t, "mygroup", cc[1].SubvolumeGroups[0].Name)
	})

	t.Run("add and remove subvolumegroup", func(t *testing.T) {
		group := &cephv1.CephFilesystemSubVolumeGroup{
			ObjectMeta: metav1.ObjectMeta{Name: "addedgroup", Namespace: csiClusterConfigEntry2.ClusterID},
			Spec: cephv1.CephFilesystemSubVolumeGroupSpec{
				FilesystemName: "fsname",
			},
		}
		err := AddSubvolumeGroup(clientset, clusterInfo, group)
		assert.NoError(t, err)
		cc, err := getTestConfigmapContents(clientset, clusterInfo)
		assert.NoError(t, err)
		assert.Equal(t, 2, len(cc))
		assert.Equal(t, 2, len(cc[1].SubvolumeGroups))
		logger.Infof("Subvolumegroups: %v", cc[1].SubvolumeGroups)
		assert.Equal(t, "mygroup", cc[1].SubvolumeGroups[0].Name)
		assert.Equal(t, "testfs", cc[1].SubvolumeGroups[0].Filesystem)
		assert.Equal(t, "addedgroup", cc[1].SubvolumeGroups[1].Name)
		assert.Equal(t, "fsname", cc[1].SubvolumeGroups[1].Filesystem)

		err = RemoveSubvolumeGroup(clientset, clusterInfo, group)
		assert.NoError(t, err)
		cc, err = getTestConfigmapContents(clientset, clusterInfo)
		assert.NoError(t, err)
		assert.Equal(t, 2, len(cc))
		assert.Equal(t, 1, len(cc[1].SubvolumeGroups))
		assert.Equal(t, "mygroup", cc[1].SubvolumeGroups[0].Name)
		assert.Equal(t, "testfs", cc[1].SubvolumeGroups[0].Filesystem)
	})
}

func getTestConfigmapContents(clientset kubernetes.Interface, clusterInfo *client.ClusterInfo) ([]CsiClusterConfigEntry, error) {
	// fetch current ConfigMap contents
	configMap, err := clientset.CoreV1().ConfigMaps(os.Getenv("POD_NAMESPACE")).Get(clusterInfo.Context, ConfigName, metav1.GetOptions{})
	if err != nil {
		return nil, errors.Wrap(err, "failed to fetch current csi config map")
	}
	return parseCsiClusterConfig(configMap.Data[ConfigKey])
}
