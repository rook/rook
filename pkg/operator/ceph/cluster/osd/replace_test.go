/*
Copyright 2023 The Rook Authors. All rights reserved.

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

	"github.com/pkg/errors"
	cephv1 "github.com/rook/rook/pkg/apis/ceph.rook.io/v1"
	"github.com/rook/rook/pkg/clusterd"
	cephclient "github.com/rook/rook/pkg/daemon/ceph/client"
	"github.com/rook/rook/pkg/operator/k8sutil"
	exectest "github.com/rook/rook/pkg/util/exec/test"
	"github.com/stretchr/testify/assert"
	appsv1 "k8s.io/api/apps/v1"
	"k8s.io/client-go/kubernetes/fake"
)

func TestGetOSDWithNonMatchingStoreOnNodes(t *testing.T) {
	namespace := "rook-ceph"
	namespace2 := "rook-ceph2"
	clientset := fake.NewSimpleClientset()
	ctx := &clusterd.Context{
		Clientset: clientset,
	}
	clusterInfo := &cephclient.ClusterInfo{
		Namespace: namespace,
		Context:   context.TODO(),
	}
	clusterInfo.SetName("mycluster")
	clusterInfo.OwnerInfo = cephclient.NewMinimumOwnerInfo(t)
	spec := cephv1.ClusterSpec{
		Storage: cephv1.StorageScopeSpec{
			Store: cephv1.OSDStore{
				Type: "bluestore-rdr",
			},
		},
	}
	c := New(ctx, clusterInfo, spec, "rook/rook:master")

	var d *appsv1.Deployment

	t.Run("all osd deployments are running on bluestore-rdr osd store", func(t *testing.T) {
		d = getDummyDeploymentOnNode(clientset, c, "node2", 0)
		d.Labels[osdStore] = "bluestore-rdr"
		createDeploymentOrPanic(clientset, d)

		d = getDummyDeploymentOnNode(clientset, c, "node3", 1)
		d.Labels[osdStore] = "bluestore-rdr"
		createDeploymentOrPanic(clientset, d)

		d = getDummyDeploymentOnNode(clientset, c, "node4", 2)
		d.Labels[osdStore] = "bluestore-rdr"
		createDeploymentOrPanic(clientset, d)

		osdList, err := c.getOSDWithNonMatchingStore()
		assert.NoError(t, err)
		assert.Equal(t, 0, len(osdList))
	})

	t.Run("all osd deployments are not running on bluestore-rdr store", func(t *testing.T) {
		c.clusterInfo.Namespace = namespace2

		// osd.0 is still using bluestore
		d = getDummyDeploymentOnNode(clientset, c, "node2", 0)
		createDeploymentOrPanic(clientset, d)

		// osd.1 and osd.2 are using `bluestore-rdr`
		d = getDummyDeploymentOnNode(clientset, c, "node3", 1)
		d.Labels[osdStore] = "bluestore-rdr"
		createDeploymentOrPanic(clientset, d)

		d = getDummyDeploymentOnNode(clientset, c, "node4", 2)
		d.Labels[osdStore] = "bluestore-rdr"
		createDeploymentOrPanic(clientset, d)

		osdList, err := c.getOSDWithNonMatchingStore()
		assert.NoError(t, err)
		assert.Equal(t, 1, len(osdList))
		assert.Equal(t, 0, osdList[0].ID)
		assert.Equal(t, "node2", osdList[0].Node)
		assert.Equal(t, "/dev/vda", osdList[0].Path)
	})
}

func TestGetOSDWithNonMatchingStoreOnPVCs(t *testing.T) {
	namespace := "rook-ceph"
	namespace2 := "rook-ceph2"
	clientset := fake.NewSimpleClientset()
	ctx := &clusterd.Context{
		Clientset: clientset,
	}
	clusterInfo := &cephclient.ClusterInfo{
		Namespace: namespace,
		Context:   context.TODO(),
	}
	clusterInfo.SetName("mycluster")
	clusterInfo.OwnerInfo = cephclient.NewMinimumOwnerInfo(t)
	spec := cephv1.ClusterSpec{
		Storage: cephv1.StorageScopeSpec{
			Store: cephv1.OSDStore{
				Type: "bluestore-rdr",
			},
		},
	}
	c := New(ctx, clusterInfo, spec, "rook/rook:master")

	var d *appsv1.Deployment

	t.Run("all osd deployments are running on bluestore-rdr osd store", func(t *testing.T) {
		d = getDummyDeploymentOnPVC(clientset, c, "pvc0", 0)
		d.Labels[osdStore] = "bluestore-rdr"
		createDeploymentOrPanic(clientset, d)

		d = getDummyDeploymentOnPVC(clientset, c, "pvc1", 1)
		d.Labels[osdStore] = "bluestore-rdr"
		createDeploymentOrPanic(clientset, d)

		d = getDummyDeploymentOnPVC(clientset, c, "pvc2", 2)
		d.Labels[osdStore] = "bluestore-rdr"
		createDeploymentOrPanic(clientset, d)

		osdList, err := c.getOSDWithNonMatchingStore()
		assert.NoError(t, err)
		assert.Equal(t, 0, len(osdList))
	})

	t.Run("all osd deployments are not running on bluestore-rdr store", func(t *testing.T) {
		c.clusterInfo.Namespace = namespace2

		// osd.0 is still using `bluestore`
		d = getDummyDeploymentOnPVC(clientset, c, "pvc0", 0)
		createDeploymentOrPanic(clientset, d)

		d = getDummyDeploymentOnPVC(clientset, c, "pvc1", 1)
		d.Labels[osdStore] = "bluestore-rdr"
		createDeploymentOrPanic(clientset, d)

		d = getDummyDeploymentOnPVC(clientset, c, "pvc2", 2)
		d.Labels[osdStore] = "bluestore-rdr"
		createDeploymentOrPanic(clientset, d)

		osdList, err := c.getOSDWithNonMatchingStore()
		assert.NoError(t, err)
		assert.Equal(t, 1, len(osdList))
		assert.Equal(t, 0, osdList[0].ID)
		assert.Equal(t, "pvc0", osdList[0].Path)
	})
}

func TestGetOSDReplaceInfo(t *testing.T) {
	namespace := "rook-ceph"
	clientset := fake.NewSimpleClientset()
	executor := &exectest.MockExecutor{
		MockExecuteCommandWithOutput: func(command string, args ...string) (string, error) {
			logger.Infof("Command: %s %v", command, args)
			if args[0] == "status" {
				return healthyCephStatus, nil
			}
			return "", errors.Errorf("unexpected ceph command '%v'", args)
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

	spec := cephv1.ClusterSpec{}
	c := New(ctx, clusterInfo, spec, "rook/rook:master")

	t.Run("no OSD replace info available", func(t *testing.T) {
		actualOSDInfo, err := c.getOSDReplaceInfo()
		assert.NoError(t, err)
		assert.Nil(t, actualOSDInfo)
	})

	t.Run("ensure no OSD to replace if pgs are not healthy", func(t *testing.T) {
		executor := &exectest.MockExecutor{
			MockExecuteCommandWithOutput: func(command string, args ...string) (string, error) {
				logger.Infof("Command: %s %v", command, args)
				if args[0] == "status" {
					return unHealthyCephStatus, nil
				}
				return "", errors.Errorf("unexpected ceph command '%v'", args)
			},
		}

		ctx.Executor = executor
		c := New(ctx, clusterInfo, spec, "rook/rook:master")
		actualOSDInfo, err := c.getOSDReplaceInfo()
		assert.NoError(t, err)
		assert.Nil(t, actualOSDInfo)
	})

	t.Run("read OSD 0 replace info from config map", func(t *testing.T) {
		actualOSDInfo := &OSDReplaceInfo{ID: 0, Path: "/dev/sda", Node: "node1"}
		err := actualOSDInfo.saveAsConfig(c.context, c.clusterInfo)
		assert.NoError(t, err)
		expectedOSDInfo, err := c.getOSDReplaceInfo()
		assert.NoError(t, err)
		assert.NotNil(t, expectedOSDInfo)
		assert.Equal(t, 0, expectedOSDInfo.ID)
		err = k8sutil.DeleteConfigMap(clusterInfo.Context, ctx.Clientset, OSDReplaceConfigName, namespace, &k8sutil.DeleteOptions{})
		assert.NoError(t, err)
	})

	t.Run("ensure no OSD replace info if all OSDs using expected OSD store", func(t *testing.T) {
		spec := cephv1.ClusterSpec{
			Storage: cephv1.StorageScopeSpec{
				Store: cephv1.OSDStore{
					Type:        "bluestore-rdr",
					UpdateStore: "yes-really-update-store",
				},
			},
		}
		executor := &exectest.MockExecutor{
			MockExecuteCommandWithOutput: func(command string, args ...string) (string, error) {
				logger.Infof("Command: %s %v", command, args)
				if args[0] == "status" {
					return healthyCephStatus, nil
				}
				return "", errors.Errorf("unexpected ceph command '%v'", args)
			},
		}

		ctx.Executor = executor
		c := New(ctx, clusterInfo, spec, "myversion")
		d := getDummyDeploymentOnNode(clientset, c, "node2", 1)
		d.Labels[osdStore] = "bluestore-rdr"
		createDeploymentOrPanic(clientset, d)
		expectedOSDInfo, err := c.getOSDReplaceInfo()
		assert.NoError(t, err)
		assert.Nil(t, expectedOSDInfo)
	})

	t.Run("ensure OSD replace info if OSD store is not up to date", func(t *testing.T) {
		spec := cephv1.ClusterSpec{
			Storage: cephv1.StorageScopeSpec{
				Store: cephv1.OSDStore{
					Type:        "bluestore-rdr",
					UpdateStore: "yes-really-update-store",
				},
			},
		}
		executor := &exectest.MockExecutor{
			MockExecuteCommandWithOutput: func(command string, args ...string) (string, error) {
				logger.Infof("Command: %s %v", command, args)
				if args[0] == "status" {
					return healthyCephStatus, nil
				}
				return "", errors.Errorf("unexpected ceph command '%v'", args)
			},
		}

		ctx.Executor = executor
		c := New(ctx, clusterInfo, spec, "myversion")
		d := getDummyDeploymentOnNode(clientset, c, "node3", 2)
		createDeploymentOrPanic(clientset, d)
		expectedOSDInfo, err := c.getOSDReplaceInfo()
		assert.NoError(t, err)
		assert.NotNil(t, expectedOSDInfo)
		assert.Equal(t, 2, expectedOSDInfo.ID)
		assert.Equal(t, "node3", expectedOSDInfo.Node)
	})
}
