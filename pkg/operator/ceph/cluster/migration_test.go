/*
Copyright 2018 The Rook Authors. All rights reserved.

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
package cluster

import (
	"testing"
	"time"

	cephv1beta1 "github.com/rook/rook/pkg/apis/ceph.rook.io/v1beta1"
	rookv1alpha1 "github.com/rook/rook/pkg/apis/rook.io/v1alpha1"
	rookv1alpha2 "github.com/rook/rook/pkg/apis/rook.io/v1alpha2"
	rookfake "github.com/rook/rook/pkg/client/clientset/versioned/fake"
	"github.com/rook/rook/pkg/clusterd"
	"github.com/rook/rook/pkg/daemon/ceph/agent/flexvolume/attachment"
	testop "github.com/rook/rook/pkg/operator/test"
	"github.com/stretchr/testify/assert"
	"k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestGetClusterObject(t *testing.T) {
	// get a current version cluster object, should return with no error and no migration needed
	cluster, migrationNeeded, err := getClusterObject(&cephv1beta1.Cluster{})
	assert.NotNil(t, cluster)
	assert.False(t, migrationNeeded)
	assert.Nil(t, err)

	// get a legacy version cluster object, should return with no error and yes migration needed
	cluster, migrationNeeded, err = getClusterObject(&rookv1alpha1.Cluster{})
	assert.NotNil(t, cluster)
	assert.True(t, migrationNeeded)
	assert.Nil(t, err)

	// try to get an object that isn't a cluster, should return with an error
	cluster, migrationNeeded, err = getClusterObject(&map[string]string{})
	assert.Nil(t, cluster)
	assert.False(t, migrationNeeded)
	assert.NotNil(t, err)
}

func TestMigrateClusterObject(t *testing.T) {
	// create a legacy cluster that will get migrated
	legacyCluster := &rookv1alpha1.Cluster{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "legacy-cluster-3093",
			Namespace: "rook-384",
		},
	}

	// create fake core and rook clientsets and a cluster controller
	clientset := testop.New(3)
	context := &clusterd.Context{
		Clientset:     clientset,
		RookClientset: rookfake.NewSimpleClientset(legacyCluster),
	}
	controller := NewClusterController(context, "", &attachment.MockAttachment{})

	// convert the legacy cluster object in memory and assert that a migration is needed
	convertedCluster, migrationNeeded, err := getClusterObject(legacyCluster)
	assert.NotNil(t, convertedCluster)
	assert.True(t, migrationNeeded)
	assert.Nil(t, err)

	// perform the migration of the converted legacy cluster
	err = controller.migrateClusterObject(convertedCluster, legacyCluster)
	assert.Nil(t, err)

	assertLegacyClusterMigrated(t, context, legacyCluster)
}

func TestOnUpdateLegacyClusterMigration(t *testing.T) {
	// create an old/new legacy cluster pair to simulate an update event on a legacy cluster
	oldLegacyCluster := &rookv1alpha1.Cluster{ObjectMeta: metav1.ObjectMeta{Name: "legacy-cluster-681", Namespace: "rook-159"}}
	newLegacyCluster := &rookv1alpha1.Cluster{ObjectMeta: metav1.ObjectMeta{Name: "legacy-cluster-681", Namespace: "rook-159"}}

	// create fake core and rook clientsets and a cluster controller
	clientset := testop.New(3)
	context := &clusterd.Context{
		Clientset: clientset,
		// the client set should only know about the "new" cluster because that is the only object that exists
		// in the API during an onUpdate event
		RookClientset: rookfake.NewSimpleClientset(newLegacyCluster),
	}
	controller := NewClusterController(context, "", &attachment.MockAttachment{})

	// call the onUpdate event with the old/new legacy cluster pair
	controller.onUpdate(oldLegacyCluster, newLegacyCluster)

	// the legacy cluster should have been migrated
	assertLegacyClusterMigrated(t, context, newLegacyCluster)
}

func TestOnUpdateLegacyClusterDeleted(t *testing.T) {
	// create an old/new legacy cluster pair to simulate an update event on a legacy cluster
	// simulate that the legacy cluster has been deleted by setting the deletion timestamp (the legacy cluster should also
	// have a legacy finalizer too)
	oldLegacyCluster := &rookv1alpha1.Cluster{ObjectMeta: metav1.ObjectMeta{Name: "legacy-cluster-428", Namespace: "rook-361"}}
	newLegacyCluster := &rookv1alpha1.Cluster{
		ObjectMeta: metav1.ObjectMeta{
			Name:              "legacy-cluster-428",
			Namespace:         "rook-361",
			DeletionTimestamp: &metav1.Time{Time: time.Now()},
			Finalizers:        []string{finalizerNameRookLegacy},
		},
	}

	// create fake core and rook clientsets and a cluster controller
	clientset := testop.New(3)
	context := &clusterd.Context{
		Clientset: clientset,
		// the client set should only know about the "new" cluster because that is the only object that exists
		// in the API during an onUpdate event
		RookClientset: rookfake.NewSimpleClientset(newLegacyCluster),
	}
	controller := NewClusterController(context, "", &attachment.MockAttachment{})

	// call the onUpdate event with the old/new legacy cluster pair, since the object has a deletion timestamp and a finalizer, this
	// onUpdate event is actually saying that the legacy cluster has been deleted (probably from a completed migration)
	controller.onUpdate(oldLegacyCluster, newLegacyCluster)

	// the finalizer should have been removed so that deletion of the legacy cluster object can proceed by the API
	deletedLegacyCluster, err := context.RookClientset.RookV1alpha1().Clusters(newLegacyCluster.Namespace).Get(
		newLegacyCluster.Name, metav1.GetOptions{})
	assert.NotNil(t, deletedLegacyCluster)
	assert.Nil(t, err)
	assert.Equal(t, 0, len(deletedLegacyCluster.Finalizers))
}
func TestConvertLegacyCluster(t *testing.T) {
	f := false

	legacyCluster := rookv1alpha1.Cluster{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "legacy-cluster-5283",
			Namespace: "rook-9837",
		},
		Spec: rookv1alpha1.ClusterSpec{
			Backend:         "ceph",
			DataDirHostPath: "/var/lib/rook302",
			HostNetwork:     true,
			MonCount:        5,
			Placement: rookv1alpha1.PlacementSpec{
				All: rookv1alpha1.Placement{Tolerations: []v1.Toleration{{Key: "storage-node", Operator: v1.TolerationOpExists}}},
				Mon: rookv1alpha1.Placement{
					PodAntiAffinity: &v1.PodAntiAffinity{
						RequiredDuringSchedulingIgnoredDuringExecution: []v1.PodAffinityTerm{
							{LabelSelector: &metav1.LabelSelector{MatchLabels: map[string]string{"label": "value"}}},
						},
					},
				},
			},
			Resources: rookv1alpha1.ResourceSpec{
				OSD: v1.ResourceRequirements{Limits: v1.ResourceList{v1.ResourceMemory: resource.MustParse("250Mi")}},
			},
			Storage: rookv1alpha1.StorageSpec{
				UseAllNodes: false,
				Selection: rookv1alpha1.Selection{
					UseAllDevices:  &f,
					DeviceFilter:   "dev1*",
					MetadataDevice: "nvme033",
					Directories: []rookv1alpha1.Directory{
						{Path: "/rook/dir1"},
					},
				},
				Config: rookv1alpha1.Config{
					Location: "datacenter=dc083",
					StoreConfig: rookv1alpha1.StoreConfig{
						StoreType:      "filestore",
						JournalSizeMB:  100,
						WalSizeMB:      200,
						DatabaseSizeMB: 300,
					},
				},
				Nodes: []rookv1alpha1.Node{
					{ // node with no node specific config
						Name: "node1",
					},
					{ // node with a lot of node specific config
						Name: "node2",
						Devices: []rookv1alpha1.Device{
							{Name: "vdx1"},
						},
						Selection: rookv1alpha1.Selection{
							UseAllDevices:  &f,
							DeviceFilter:   "dev2*",
							MetadataDevice: "nvme982",
							Directories: []rookv1alpha1.Directory{
								{Path: "/rook/dir2"},
							},
						},
						Config: rookv1alpha1.Config{
							Location: "datacenter=dc083,rack=rackA",
							StoreConfig: rookv1alpha1.StoreConfig{
								StoreType:      "bluestore",
								JournalSizeMB:  1000,
								WalSizeMB:      2000,
								DatabaseSizeMB: 3000,
							},
						},
					},
				},
			},
		},
	}

	expectedCluster := cephv1beta1.Cluster{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "legacy-cluster-5283",
			Namespace: "rook-9837",
		},
		Spec: cephv1beta1.ClusterSpec{
			DataDirHostPath: "/var/lib/rook302",
			Mon: cephv1beta1.MonSpec{
				Count:                5,
				AllowMultiplePerNode: true,
			},
			Network: rookv1alpha2.NetworkSpec{HostNetwork: true},
			Placement: rookv1alpha2.PlacementSpec{
				rookv1alpha2.PlacementKeyAll: rookv1alpha2.Placement{Tolerations: []v1.Toleration{{Key: "storage-node", Operator: v1.TolerationOpExists}}},
				cephv1beta1.PlacementKeyMon: rookv1alpha2.Placement{
					PodAntiAffinity: &v1.PodAntiAffinity{
						RequiredDuringSchedulingIgnoredDuringExecution: []v1.PodAffinityTerm{
							{LabelSelector: &metav1.LabelSelector{MatchLabels: map[string]string{"label": "value"}}},
						},
					},
				},
				cephv1beta1.PlacementKeyMgr: rookv1alpha2.Placement{},
				cephv1beta1.PlacementKeyOSD: rookv1alpha2.Placement{},
			},
			Resources: rookv1alpha2.ResourceSpec{
				cephv1beta1.ResourcesKeyOSD: v1.ResourceRequirements{Limits: v1.ResourceList{v1.ResourceMemory: resource.MustParse("250Mi")}},
				cephv1beta1.ResourcesKeyMon: v1.ResourceRequirements{},
				cephv1beta1.ResourcesKeyMgr: v1.ResourceRequirements{},
			},
			Storage: rookv1alpha2.StorageScopeSpec{
				UseAllNodes: false,
				Location:    "datacenter=dc083",
				Config: map[string]string{
					"storeType":      "filestore",
					"journalSizeMB":  "100",
					"walSizeMB":      "200",
					"databaseSizeMB": "300",
					"metadataDevice": "nvme033",
				},
				Selection: rookv1alpha2.Selection{
					UseAllDevices: &f,
					DeviceFilter:  "dev1*",
					Devices:       []rookv1alpha2.Device{},
					Directories: []rookv1alpha2.Directory{
						{Path: "/rook/dir1", Config: map[string]string{}},
					},
				},
				Nodes: []rookv1alpha2.Node{
					{
						Name:   "node1",
						Config: map[string]string{},
						Selection: rookv1alpha2.Selection{
							Devices:     []rookv1alpha2.Device{},
							Directories: []rookv1alpha2.Directory{},
						},
					},
					{
						Name:     "node2",
						Location: "datacenter=dc083,rack=rackA",
						Selection: rookv1alpha2.Selection{
							UseAllDevices: &f,
							DeviceFilter:  "dev2*",
							Devices: []rookv1alpha2.Device{
								{Name: "vdx1", Config: map[string]string{}},
							},
							Directories: []rookv1alpha2.Directory{
								{Path: "/rook/dir2", Config: map[string]string{}},
							},
						},
						Config: map[string]string{
							"metadataDevice": "nvme982",
							"storeType":      "bluestore",
							"journalSizeMB":  "1000",
							"walSizeMB":      "2000",
							"databaseSizeMB": "3000",
						},
					},
				},
			},
		},
	}

	// convert the legacy cluster and compare it to the expected cluster result
	assert.Equal(t, expectedCluster, *(convertRookLegacyCluster(&legacyCluster)))

	// check if legacy monCount is `0` that we default to `3`
	legacyCluster.Spec.MonCount = 0
	expectedCluster.Spec.Mon.Count = 3
	assert.Equal(t, expectedCluster, *(convertRookLegacyCluster(&legacyCluster)))
}

func assertLegacyClusterMigrated(t *testing.T, context *clusterd.Context, legacyCluster *rookv1alpha1.Cluster) {
	// assert that a current cluster object was created via the migration
	migratedCluster, err := context.RookClientset.CephV1beta1().Clusters(legacyCluster.Namespace).Get(legacyCluster.Name, metav1.GetOptions{})
	assert.NotNil(t, migratedCluster)
	assert.Nil(t, err)

	// assert that the legacy cluster object was deleted
	_, err = context.RookClientset.RookV1alpha1().Clusters(legacyCluster.Namespace).Get(legacyCluster.Name, metav1.GetOptions{})
	assert.NotNil(t, err)
	assert.True(t, errors.IsNotFound(err))
}
