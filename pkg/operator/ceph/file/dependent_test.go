/*
Copyright 2021 The Rook Authors. All rights reserved.

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

package file

import (
	"context"
	"fmt"
	"testing"

	"github.com/pkg/errors"
	cephv1 "github.com/rook/rook/pkg/apis/ceph.rook.io/v1"
	rookclient "github.com/rook/rook/pkg/client/clientset/versioned/fake"
	"github.com/rook/rook/pkg/clusterd"
	"github.com/rook/rook/pkg/daemon/ceph/client"
	"github.com/stretchr/testify/assert"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
)

func TestCephFilesystemDependents(t *testing.T) {
	ctx := context.TODO()
	scheme := runtime.NewScheme()
	assert.NoError(t, cephv1.AddToScheme(scheme))
	ns := "test-ceph-filesystem-dependents"

	newClusterdCtx := func(objects ...runtime.Object) *clusterd.Context {
		return &clusterd.Context{
			RookClientset: rookclient.NewSimpleClientset(),
		}
	}

	clusterInfo := client.AdminTestClusterInfo(ns)
	// Create objectmeta with the given name in our test namespace
	meta := func(name string) v1.ObjectMeta {
		return v1.ObjectMeta{
			Name:      name,
			Namespace: ns,
		}
	}

	fs := &cephv1.CephFilesystem{
		ObjectMeta: v1.ObjectMeta{
			Name:      "myfs",
			Namespace: ns,
		},
	}

	oldListGroups := client.ListSubvolumeGroups
	oldListSubvols := client.ListSubvolumesInGroup
	defer func() {
		client.ListSubvolumeGroups = oldListGroups
		client.ListSubvolumesInGroup = oldListSubvols
	}()
	noSubvolumeGroups := func(
		context *clusterd.Context, clusterInfo *client.ClusterInfo, fsName string,
	) (client.SubvolumeGroupList, error) {
		return client.SubvolumeGroupList{}, nil
	}
	noSubvolumes := func(
		context *clusterd.Context, clusterInfo *client.ClusterInfo, fsName, groupName string,
	) (client.SubvolumeList, error) {
		return client.SubvolumeList{}, nil
	}

	t.Run("no blocking dependents", func(t *testing.T) {
		client.ListSubvolumeGroups = noSubvolumeGroups
		client.ListSubvolumesInGroup = noSubvolumes

		c := newClusterdCtx()
		deps, err := CephFilesystemDependents(c, clusterInfo, fs)
		assert.NoError(t, err)
		assert.True(t, deps.Empty())
	})

	t.Run("one CephFilesystemSubVolumeGroup but wrong fs", func(t *testing.T) {
		client.ListSubvolumeGroups = noSubvolumeGroups
		client.ListSubvolumesInGroup = noSubvolumes

		otherFs := &cephv1.CephFilesystem{
			ObjectMeta: v1.ObjectMeta{
				Name:      "otherfs",
				Namespace: ns,
			},
		}

		c := newClusterdCtx(&cephv1.CephFilesystemSubVolumeGroup{ObjectMeta: meta("subvolgroup1")})
		_, err := c.RookClientset.CephV1().CephFilesystemSubVolumeGroups(clusterInfo.Namespace).Create(ctx, &cephv1.CephFilesystemSubVolumeGroup{ObjectMeta: meta("subvolgroup1")}, v1.CreateOptions{})
		assert.NoError(t, err)
		assert.NoError(t, err)
		deps, err := CephFilesystemDependents(c, clusterInfo, otherFs)
		assert.NoError(t, err)
		assert.True(t, deps.Empty())
	})

	t.Run("one CephFilesystemSubVolumeGroup", func(t *testing.T) {
		client.ListSubvolumeGroups = noSubvolumeGroups
		client.ListSubvolumesInGroup = noSubvolumes

		c := newClusterdCtx(&cephv1.CephFilesystemSubVolumeGroup{ObjectMeta: meta("subvolgroup1")})
		_, err := c.RookClientset.CephV1().CephFilesystemSubVolumeGroups(clusterInfo.Namespace).Create(ctx, &cephv1.CephFilesystemSubVolumeGroup{ObjectMeta: meta("subvolgroup1"), Spec: cephv1.CephFilesystemSubVolumeGroupSpec{FilesystemName: "myfs"}}, v1.CreateOptions{})
		assert.NoError(t, err)
		assert.NoError(t, err)
		deps, err := CephFilesystemDependents(c, clusterInfo, fs)
		assert.NoError(t, err)
		assert.False(t, deps.Empty())
		assert.ElementsMatch(t, deps.PluralKinds(), []string{"CephFilesystemSubVolumeGroups"})
		assert.ElementsMatch(t, deps.OfKind("CephFilesystemSubVolumeGroups"), []string{"subvolgroup1"})
	})

	t.Run("one ceph subvolumegroup with no subvolumes", func(t *testing.T) {
		subvolumeGroupsToReturn := client.SubvolumeGroupList{
			client.SubvolumeGroup{Name: "csi"},
		}
		client.ListSubvolumeGroups = func(
			context *clusterd.Context, clusterInfo *client.ClusterInfo, fsName string,
		) (client.SubvolumeGroupList, error) {
			assert.Equal(t, "myfs", fsName)
			return subvolumeGroupsToReturn, nil
		}

		client.ListSubvolumesInGroup = func(
			context *clusterd.Context, clusterInfo *client.ClusterInfo, fsName, groupName string,
		) (client.SubvolumeList, error) {
			assert.Equal(t, "myfs", fsName)
			if groupName == client.NoSubvolumeGroup {
				return client.SubvolumeList{}, nil
			}
			if groupName == "csi" {
				return client.SubvolumeList{}, nil
			}
			panic(fmt.Sprintf("unknown groupName %q", groupName))
		}

		c := newClusterdCtx()
		deps, err := CephFilesystemDependents(c, clusterInfo, fs)
		assert.NoError(t, err)
		assert.True(t, deps.Empty())
	})

	t.Run("one ceph subvolumegroup with subvolumes", func(t *testing.T) {
		subvolumeGroupsToReturn := client.SubvolumeGroupList{
			client.SubvolumeGroup{Name: "csi"},
			client.SubvolumeGroup{Name: "other"},
		}
		client.ListSubvolumeGroups = func(
			context *clusterd.Context, clusterInfo *client.ClusterInfo, fsName string,
		) (client.SubvolumeGroupList, error) {
			assert.Equal(t, "myfs", fsName)
			return subvolumeGroupsToReturn, nil
		}

		csiSubvolumesToReturn := client.SubvolumeList{
			client.Subvolume{Name: "csi-vol-hash"},
			client.Subvolume{Name: "csi-nfs-vol-hash"},
		}
		client.ListSubvolumesInGroup = func(
			context *clusterd.Context, clusterInfo *client.ClusterInfo, fsName, groupName string,
		) (client.SubvolumeList, error) {
			assert.Equal(t, "myfs", fsName)
			if groupName == client.NoSubvolumeGroup {
				return client.SubvolumeList{}, nil
			}
			if groupName == "csi" {
				return csiSubvolumesToReturn, nil
			}
			if groupName == "other" {
				// "other" exists but does not have subvolumes so should not be listed as a dependent
				return client.SubvolumeList{}, nil
			}
			panic(fmt.Sprintf("unknown groupName %q", groupName))
		}

		c := newClusterdCtx()
		deps, err := CephFilesystemDependents(c, clusterInfo, fs)
		assert.NoError(t, err)
		assert.False(t, deps.Empty())
		assert.ElementsMatch(t, deps.PluralKinds(), []string{subvolumeGroupDependentType})
		assert.ElementsMatch(t, deps.OfKind(subvolumeGroupDependentType), []string{"csi"})
	})

	t.Run("one ceph subvolumegroup with error listing subvolumes", func(t *testing.T) {
		subvolumeGroupsToReturn := client.SubvolumeGroupList{
			client.SubvolumeGroup{Name: "csi"},
		}
		client.ListSubvolumeGroups = func(
			context *clusterd.Context, clusterInfo *client.ClusterInfo, fsName string,
		) (client.SubvolumeGroupList, error) {
			assert.Equal(t, "myfs", fsName)
			return subvolumeGroupsToReturn, nil
		}

		client.ListSubvolumesInGroup = func(
			context *clusterd.Context, clusterInfo *client.ClusterInfo, fsName, groupName string,
		) (client.SubvolumeList, error) {
			assert.Equal(t, "myfs", fsName)
			return client.SubvolumeList{}, errors.New("induced error")
		}

		c := newClusterdCtx()
		deps, err := CephFilesystemDependents(c, clusterInfo, fs)
		assert.Error(t, err)
		assert.Contains(t, err.Error(),
			`failed to list subvolumes in filesystem "myfs" for one or more subvolume groups; a timeout might indicate there are many subvolumes in a subvolume group:`)
		assert.Contains(t, err.Error(), `failed to list subvolumes in subvolume group "csi": induced error`)
		assert.True(t, deps.Empty())
	})

	t.Run("one CephFilesystemSubVolumeGroup and one ceph subvolumegroup with subvolumes", func(t *testing.T) {
		subvolumeGroupsToReturn := client.SubvolumeGroupList{
			client.SubvolumeGroup{Name: "csi"},
		}
		client.ListSubvolumeGroups = func(
			context *clusterd.Context, clusterInfo *client.ClusterInfo, fsName string,
		) (client.SubvolumeGroupList, error) {
			assert.Equal(t, "myfs", fsName)
			return subvolumeGroupsToReturn, nil
		}

		csiSubvolumesToReturn := client.SubvolumeList{
			client.Subvolume{Name: "csi-vol-hash"},
		}
		client.ListSubvolumesInGroup = func(
			context *clusterd.Context, clusterInfo *client.ClusterInfo, fsName, groupName string,
		) (client.SubvolumeList, error) {
			assert.Equal(t, "myfs", fsName)
			if groupName == client.NoSubvolumeGroup {
				return client.SubvolumeList{}, nil
			}
			if groupName == "csi" {
				return csiSubvolumesToReturn, nil
			}
			panic(fmt.Sprintf("unknown groupName %q", groupName))
		}

		c := newClusterdCtx(&cephv1.CephFilesystemSubVolumeGroup{ObjectMeta: meta("subvolgroup1")})
		_, err := c.RookClientset.CephV1().CephFilesystemSubVolumeGroups(clusterInfo.Namespace).Create(ctx, &cephv1.CephFilesystemSubVolumeGroup{ObjectMeta: meta("subvolgroup1"), Spec: cephv1.CephFilesystemSubVolumeGroupSpec{FilesystemName: "myfs"}}, v1.CreateOptions{})
		assert.NoError(t, err)
		assert.NoError(t, err)

		deps, err := CephFilesystemDependents(c, clusterInfo, fs)
		assert.NoError(t, err)
		assert.False(t, deps.Empty())
		assert.ElementsMatch(t, deps.PluralKinds(), []string{"CephFilesystemSubVolumeGroups", subvolumeGroupDependentType})
		assert.ElementsMatch(t, deps.OfKind("CephFilesystemSubVolumeGroups"), []string{"subvolgroup1"})
		assert.ElementsMatch(t, deps.OfKind(subvolumeGroupDependentType), []string{"csi"})
	})

	t.Run("empty csi subvolumegroup with non-empty ignored groups", func(t *testing.T) {
		subvolumeGroupsToReturn := client.SubvolumeGroupList{
			client.SubvolumeGroup{Name: "csi"},
			client.SubvolumeGroup{Name: "_index"},
			client.SubvolumeGroup{Name: "_legacy"},
			client.SubvolumeGroup{Name: "_deleting"},
		}
		client.ListSubvolumeGroups = func(
			context *clusterd.Context, clusterInfo *client.ClusterInfo, fsName string,
		) (client.SubvolumeGroupList, error) {
			assert.Equal(t, "myfs", fsName)
			return subvolumeGroupsToReturn, nil
		}

		indexSubvolumesToReturn := client.SubvolumeList{
			{Name: "clone"},
		}
		legacySubvolumesToReturn := client.SubvolumeList{
			{Name: "something"},
		}
		deletingSubvolumesToReturn := client.SubvolumeList{
			{Name: "something"},
		}
		client.ListSubvolumesInGroup = func(
			context *clusterd.Context, clusterInfo *client.ClusterInfo, fsName, groupName string,
		) (client.SubvolumeList, error) {
			assert.Equal(t, "myfs", fsName)
			if groupName == client.NoSubvolumeGroup {
				return client.SubvolumeList{}, nil
			}
			if groupName == "csi" {
				return client.SubvolumeList{}, nil
			}
			// any subvolumes in ignored groups should not create dependency entries
			if groupName == "_index" {
				return indexSubvolumesToReturn, nil
			}
			if groupName == "_legacy" {
				return legacySubvolumesToReturn, nil
			}
			if groupName == "_deleting" {
				return deletingSubvolumesToReturn, nil
			}
			panic(fmt.Sprintf("unknown groupName %q", groupName))
		}

		c := newClusterdCtx()
		deps, err := CephFilesystemDependents(c, clusterInfo, fs)
		assert.NoError(t, err)
		assert.True(t, deps.Empty())
	})

	t.Run("handle subvolumes in no group", func(t *testing.T) {
		subvolumeGroupsToReturn := client.SubvolumeGroupList{
			client.SubvolumeGroup{Name: "csi"},
			client.SubvolumeGroup{Name: "_nogroup"},
		}
		client.ListSubvolumeGroups = func(
			context *clusterd.Context, clusterInfo *client.ClusterInfo, fsName string,
		) (client.SubvolumeGroupList, error) {
			assert.Equal(t, "myfs", fsName)
			return subvolumeGroupsToReturn, nil
		}

		noGroupSubvolumesToReturn := client.SubvolumeList{
			{Name: "manually-created-subvol"},
		}
		client.ListSubvolumesInGroup = func(
			context *clusterd.Context, clusterInfo *client.ClusterInfo, fsName, groupName string,
		) (client.SubvolumeList, error) {
			assert.Equal(t, "myfs", fsName)
			if groupName == client.NoSubvolumeGroup {
				return noGroupSubvolumesToReturn, nil
			}
			if groupName == "_nogroup" {
				t.Error("rook should not list subvolumes in '_nogroup'")
				t.Fail()
			}
			if groupName == "csi" {
				return client.SubvolumeList{}, nil
			}
			panic(fmt.Sprintf("unknown groupName %q", groupName))
		}

		c := newClusterdCtx()
		deps, err := CephFilesystemDependents(c, clusterInfo, fs)
		assert.NoError(t, err)
		assert.False(t, deps.Empty())
		assert.ElementsMatch(t, deps.PluralKinds(), []string{subvolumeGroupDependentType})
		assert.ElementsMatch(t, deps.OfKind(subvolumeGroupDependentType), []string{noGroupDependentName})
	})
}
