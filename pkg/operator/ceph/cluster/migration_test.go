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

	cephv1 "github.com/rook/rook/pkg/apis/ceph.rook.io/v1"
	cephbeta "github.com/rook/rook/pkg/apis/ceph.rook.io/v1beta1"
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
	cluster, migrationNeeded, err := getClusterObject(&cephv1.CephCluster{})
	assert.NotNil(t, cluster)
	assert.False(t, migrationNeeded)
	assert.Nil(t, err)

	// get a legacy version cluster object, should return with no error and yes migration needed
	cluster, migrationNeeded, err = getClusterObject(&cephbeta.Cluster{})
	assert.NotNil(t, cluster)
	assert.True(t, migrationNeeded)
	assert.Nil(t, err)

	// try to get an object that isn't a cluster, should return with an error
	cluster, migrationNeeded, err = getClusterObject(&map[string]string{})
	assert.Nil(t, cluster)
	assert.False(t, migrationNeeded)
	assert.NotNil(t, err)
}

func TestDefaultClusterValues(t *testing.T) {
	// the default ceph version should be set
	cluster, _, err := getClusterObject(&cephv1.CephCluster{})
	assert.NotNil(t, cluster)
	assert.Nil(t, err)
	assert.Equal(t, cephv1.DefaultLuminousImage, cluster.Spec.CephVersion.Image)
}

func TestMigrateClusterObject(t *testing.T) {
	// create a legacy cluster that will get migrated
	legacyCluster := &cephbeta.Cluster{
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
	oldLegacyCluster := &cephbeta.Cluster{ObjectMeta: metav1.ObjectMeta{Name: "legacy-cluster-681", Namespace: "rook-159"}}
	newLegacyCluster := &cephbeta.Cluster{ObjectMeta: metav1.ObjectMeta{Name: "legacy-cluster-681", Namespace: "rook-159"}}

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
	oldLegacyCluster := &cephbeta.Cluster{ObjectMeta: metav1.ObjectMeta{Name: "legacy-cluster-428", Namespace: "rook-361"}}
	newLegacyCluster := &cephbeta.Cluster{
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
	deletedLegacyCluster, err := context.RookClientset.CephV1beta1().Clusters(newLegacyCluster.Namespace).Get(
		newLegacyCluster.Name, metav1.GetOptions{})
	assert.NotNil(t, deletedLegacyCluster)
	assert.Nil(t, err)
	assert.Equal(t, 0, len(deletedLegacyCluster.Finalizers))
}
func TestConvertLegacyCluster(t *testing.T) {
	f := false

	legacyCluster := cephbeta.Cluster{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "legacy-cluster-5283",
			Namespace: "rook-9837",
		},
		Spec: cephbeta.ClusterSpec{
			DataDirHostPath: "/var/lib/rook302",
			Mon: cephbeta.MonSpec{
				Count:                5,
				AllowMultiplePerNode: true,
			},
			Network: rookv1alpha2.NetworkSpec{HostNetwork: true},
			Placement: rookv1alpha2.PlacementSpec{
				rookv1alpha2.KeyAll: rookv1alpha2.Placement{Tolerations: []v1.Toleration{{Key: "storage-node", Operator: v1.TolerationOpExists}}},
				cephv1.KeyMon: rookv1alpha2.Placement{
					PodAntiAffinity: &v1.PodAntiAffinity{
						RequiredDuringSchedulingIgnoredDuringExecution: []v1.PodAffinityTerm{
							{LabelSelector: &metav1.LabelSelector{MatchLabels: map[string]string{"label": "value"}}},
						},
					},
				},
				cephv1.KeyMgr: rookv1alpha2.Placement{},
				cephv1.KeyOSD: rookv1alpha2.Placement{},
			},
			Resources: rookv1alpha2.ResourceSpec{
				cephv1.ResourcesKeyOSD: v1.ResourceRequirements{Limits: v1.ResourceList{v1.ResourceMemory: resource.MustParse("250Mi")}},
				cephv1.ResourcesKeyMon: v1.ResourceRequirements{},
				cephv1.ResourcesKeyMgr: v1.ResourceRequirements{},
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

	expectedCluster := cephv1.CephCluster{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "legacy-cluster-5283",
			Namespace: "rook-9837",
		},
		Spec: cephv1.ClusterSpec{
			DataDirHostPath: "/var/lib/rook302",
			CephVersion: cephv1.CephVersionSpec{
				Image:            cephv1.DefaultLuminousImage,
				AllowUnsupported: false,
			},
			Mon: cephv1.MonSpec{
				Count:                5,
				AllowMultiplePerNode: true,
			},
			Network: rookv1alpha2.NetworkSpec{HostNetwork: true},
			Placement: rookv1alpha2.PlacementSpec{
				rookv1alpha2.KeyAll: rookv1alpha2.Placement{Tolerations: []v1.Toleration{{Key: "storage-node", Operator: v1.TolerationOpExists}}},
				cephv1.KeyMon: rookv1alpha2.Placement{
					PodAntiAffinity: &v1.PodAntiAffinity{
						RequiredDuringSchedulingIgnoredDuringExecution: []v1.PodAffinityTerm{
							{LabelSelector: &metav1.LabelSelector{MatchLabels: map[string]string{"label": "value"}}},
						},
					},
				},
				cephv1.KeyMgr: rookv1alpha2.Placement{},
				cephv1.KeyOSD: rookv1alpha2.Placement{},
			},
			Resources: rookv1alpha2.ResourceSpec{
				cephv1.ResourcesKeyOSD: v1.ResourceRequirements{Limits: v1.ResourceList{v1.ResourceMemory: resource.MustParse("250Mi")}},
				cephv1.ResourcesKeyMon: v1.ResourceRequirements{},
				cephv1.ResourcesKeyMgr: v1.ResourceRequirements{},
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
}

func assertLegacyClusterMigrated(t *testing.T, context *clusterd.Context, legacyCluster *cephbeta.Cluster) {
	// assert that a current cluster object was created via the migration
	migratedCluster, err := context.RookClientset.CephV1().CephClusters(legacyCluster.Namespace).Get(legacyCluster.Name, metav1.GetOptions{})
	assert.NotNil(t, migratedCluster)
	assert.Nil(t, err)

	// assert that the legacy cluster object was deleted
	_, err = context.RookClientset.CephV1beta1().Clusters(legacyCluster.Namespace).Get(legacyCluster.Name, metav1.GetOptions{})
	assert.NotNil(t, err)
	assert.True(t, errors.IsNotFound(err))
}
