/*
Copyright 2024 The Rook Authors. All rights reserved.

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

// Package osd for the Ceph OSDs.
package osd

import (
	"context"
	"testing"

	cephv1 "github.com/rook/rook/pkg/apis/ceph.rook.io/v1"
	"github.com/rook/rook/pkg/clusterd"
	cephclient "github.com/rook/rook/pkg/daemon/ceph/client"
	exectest "github.com/rook/rook/pkg/util/exec/test"
	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"
)

func TestMigrateForEncryption(t *testing.T) {
	namespace := "rook-ceph"
	namespace2 := "rook-ceph2"
	clientset := fake.NewClientset()
	ctx := &clusterd.Context{
		Clientset: clientset,
	}
	clusterInfo := &cephclient.ClusterInfo{
		Namespace: namespace,
		Context:   context.TODO(),
	}
	clusterInfo.SetName("mycluster")
	clusterInfo.OwnerInfo = cephclient.NewMinimumOwnerInfo(t)

	c := New(ctx, clusterInfo, cephv1.ClusterSpec{}, "rook/rook:master")

	t.Run("no OSD migration is required due to encryption", func(t *testing.T) {
		c.spec.Storage.StorageClassDeviceSets = []cephv1.StorageClassDeviceSet{
			{
				Name:      "set1",
				Encrypted: true,
			},

			{
				Name:      "set2",
				Encrypted: true,
			},
		}

		d1 := getDummyDeploymentOnNode(clientset, c, "node2", 1)
		d1.Labels["encrypted"] = "true"
		d1.Labels["ceph.rook.io/DeviceSet"] = "set1"
		createDeploymentOrPanic(clientset, d1)

		d2 := getDummyDeploymentOnNode(clientset, c, "node2", 2)
		d2.Labels["encrypted"] = "true"
		d2.Labels["ceph.rook.io/DeviceSet"] = "set1"
		createDeploymentOrPanic(clientset, d2)

		deployments, err := c.getOSDDeployments()
		assert.NoError(t, err)

		mc := migrationConfig{
			osds: map[int]*OSDInfo{},
		}

		err = mc.migrateForEncryption(c, deployments)
		assert.NoError(t, err)
		assert.Equal(t, 0, len(mc.osds))
	})
	t.Run("osd.1 on set1 needs migration", func(t *testing.T) {
		c.clusterInfo.Namespace = namespace2
		c.spec.Storage.StorageClassDeviceSets = []cephv1.StorageClassDeviceSet{
			{
				Name:      "set1",
				Encrypted: true,
			},
		}

		d1 := getDummyDeploymentOnNode(clientset, c, "node2", 1)
		d1.Labels["encrypted"] = "false"
		d1.Labels["ceph.rook.io/DeviceSet"] = "set1"
		createDeploymentOrPanic(clientset, d1)

		deployments, err := c.getOSDDeployments()
		assert.NoError(t, err)

		mc := migrationConfig{
			osds: map[int]*OSDInfo{},
		}

		err = mc.migrateForEncryption(c, deployments)
		assert.NoError(t, err)
		assert.Equal(t, 1, len(mc.osds))
		assert.Equal(t, 1, mc.osds[1].ID)
	})
}

func TestMigrationForOSDStore(t *testing.T) {
	namespace := "rook-ceph"
	namespace2 := "rook-ceph2"
	clientset := fake.NewClientset()
	ctx := &clusterd.Context{
		Clientset: clientset,
	}
	clusterInfo := &cephclient.ClusterInfo{
		Namespace: namespace,
		Context:   context.TODO(),
	}
	clusterInfo.SetName("mycluster")
	clusterInfo.OwnerInfo = cephclient.NewMinimumOwnerInfo(t)

	c := New(ctx, clusterInfo, cephv1.ClusterSpec{}, "rook/rook:master")

	t.Run("no OSD migration is required due to OSD store change", func(t *testing.T) {
		c.spec.Storage.Store.Type = "store1"

		d1 := getDummyDeploymentOnNode(clientset, c, "node2", 1)
		d1.Labels[osdStore] = "store1"
		createDeploymentOrPanic(clientset, d1)

		d2 := getDummyDeploymentOnNode(clientset, c, "node2", 2)
		d2.Labels[osdStore] = "store1"
		createDeploymentOrPanic(clientset, d2)

		deployments, err := c.getOSDDeployments()
		assert.NoError(t, err)

		mc := migrationConfig{
			osds: map[int]*OSDInfo{},
		}

		err = mc.migrateForEncryption(c, deployments)
		assert.NoError(t, err)
		assert.Equal(t, 0, len(mc.osds))
	})
	t.Run("osd.1 needs migration due to change is OSD store type", func(t *testing.T) {
		c.clusterInfo.Namespace = namespace2
		c.spec.Storage.Store.Type = "newStore"

		d1 := getDummyDeploymentOnNode(clientset, c, "node2", 1)
		d1.Labels[osdStore] = "store1" // store type is set to `store1` but spec has `newStore`
		createDeploymentOrPanic(clientset, d1)

		d2 := getDummyDeploymentOnNode(clientset, c, "node2", 2)
		d2.Labels[osdStore] = "newStore" // store type label matches with the spec
		createDeploymentOrPanic(clientset, d2)

		deployments, err := c.getOSDDeployments()
		assert.NoError(t, err)

		mc := migrationConfig{
			osds: map[int]*OSDInfo{},
		}

		err = mc.migrateForOSDStore(c, deployments)
		assert.NoError(t, err)
		assert.Equal(t, 1, len(mc.osds))
		assert.Equal(t, 1, mc.osds[1].ID)
	})
}

func createMigrationConfigmap(osdID, ns string, clientset *fake.Clientset) error {
	ctx := context.TODO()
	data := make(map[string]string, 1)
	data[OSDIdKey] = osdID
	cm := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      OSDMigrationConfigName,
			Namespace: ns,
		},
		Data: data,
	}
	_, err := clientset.CoreV1().ConfigMaps(ns).Create(ctx, cm, metav1.CreateOptions{})
	return err
}

func TestIsLastOSDMigrationComplete(t *testing.T) {
	namespace := "rook-ceph"
	clientset := fake.NewClientset()
	ctx := &clusterd.Context{
		Clientset: clientset,
	}
	clusterInfo := &cephclient.ClusterInfo{
		Namespace: namespace,
		Context:   context.TODO(),
	}
	clusterInfo.SetName("mycluster")
	clusterInfo.OwnerInfo = cephclient.NewMinimumOwnerInfo(t)

	c := New(ctx, clusterInfo, cephv1.ClusterSpec{}, "rook/rook:master")
	t.Run("no OSD migration config found", func(t *testing.T) {
		result, err := isLastOSDMigrationComplete(c)
		assert.NoError(t, err)
		assert.Equal(t, true, result)
	})

	t.Run("osd.1 was migrated but its not up yet", func(t *testing.T) {
		err := createMigrationConfigmap("1", namespace, clientset)
		assert.NoError(t, err)
		result, err := isLastOSDMigrationComplete(c)
		assert.NoError(t, err)
		assert.Equal(t, false, result)
	})

	t.Run("migrated osd.1 is up", func(t *testing.T) {
		d1 := getDummyDeploymentOnNode(clientset, c, "node2", 1)
		createDeploymentOrPanic(clientset, d1)
		result, err := isLastOSDMigrationComplete(c)
		assert.NoError(t, err)
		assert.Equal(t, true, result)
	})
}

func TestStartOSDMigration(t *testing.T) {
	namespace := "rook-ceph"
	clientset := fake.NewClientset()
	// PGs report as clean so migration is not blocked on PG health.
	executor := &exectest.MockExecutor{
		MockExecuteCommandWithOutput: func(command string, args ...string) (string, error) {
			if args[0] == "status" {
				return "{}", nil
			}
			return "", nil
		},
	}
	ctx := &clusterd.Context{
		Clientset: clientset,
		Executor:  executor,
	}
	clusterInfo := &cephclient.ClusterInfo{
		Namespace: namespace,
		Context:   context.TODO(),
	}
	clusterInfo.SetName("mycluster")
	clusterInfo.OwnerInfo = cephclient.NewMinimumOwnerInfo(t)

	c := New(ctx, clusterInfo, cephv1.ClusterSpec{}, "rook/rook:master")
	c.spec.Storage.Migration.Confirmation = OSDMigrationConfirmation
	c.spec.Storage.Store.Type = "newStore"

	t.Run("pending OSDs are returned without starting a new migration when the previous one is incomplete", func(t *testing.T) {
		// osd.2 still runs on the old store, so it is pending migration.
		d2 := getDummyDeploymentOnNode(clientset, c, "node2", 2)
		d2.Labels[osdStore] = "oldStore"
		createDeploymentOrPanic(clientset, d2)

		// osd.1 was already migrated but its deployment has not been recreated yet, so the
		// previous migration is still in progress.
		err := createMigrationConfigmap("1", namespace, clientset)
		assert.NoError(t, err)

		migrationConfig, err := c.startOSDMigration()
		// The reconcile must keep flowing (no error) so the interrupted OSD can be recreated
		// downstream, while still returning the pending OSDs so the caller removes them from
		// the update queue.
		assert.NoError(t, err)
		assert.NotNil(t, migrationConfig)
		assert.Equal(t, []int{2}, migrationConfig.getOSDIds())

		// No new migration must be started while the previous one is incomplete: osd.2's
		// deployment must still exist and no OSD may be recorded for migration.
		assert.Nil(t, c.migrateOSD)
		_, err = clientset.AppsV1().Deployments(namespace).Get(context.TODO(), "rook-ceph-osd-2", metav1.GetOptions{})
		assert.NoError(t, err)
	})
}
