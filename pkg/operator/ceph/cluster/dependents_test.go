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

package cluster

import (
	"testing"

	cephv1 "github.com/rook/rook/pkg/apis/ceph.rook.io/v1"
	"github.com/rook/rook/pkg/clusterd"
	"github.com/stretchr/testify/assert"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	fakeclient "sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func TestCephClusterDependents(t *testing.T) {
	scheme := runtime.NewScheme()
	assert.NoError(t, cephv1.AddToScheme(scheme))

	ns := "test-ceph-cluster-dependents"

	var c *clusterd.Context

	newClusterdCtx := func(objects ...client.Object) *clusterd.Context {
		return &clusterd.Context{
			Client: fakeclient.NewClientBuilder().WithScheme(scheme).WithObjects(objects...).Build(),
		}
	}

	// Create objectmeta with the given name in our test namespace
	meta := func(name string) v1.ObjectMeta {
		return v1.ObjectMeta{
			Name:      name,
			Namespace: ns,
		}
	}

	t.Run("CephBlockPools", func(t *testing.T) {
		c = newClusterdCtx(
			&cephv1.CephBlockPool{ObjectMeta: meta("block-pool-1")},
			&cephv1.CephBlockPool{ObjectMeta: meta("block-pool-2")},
		)
		deps, err := CephClusterDependents(c, ns)
		assert.NoError(t, err)
		assert.False(t, deps.Empty())
		assert.ElementsMatch(t, []string{"CephBlockPool"}, deps.PluralKinds())
		assert.ElementsMatch(t, []string{"block-pool-1", "block-pool-2"}, deps.OfKind("CephBlockPool"))
	})

	t.Run("CephRBDMirrors", func(t *testing.T) {
		c = newClusterdCtx(
			&cephv1.CephRBDMirror{ObjectMeta: meta("rbdmirror")},
		)
		deps, err := CephClusterDependents(c, ns)
		assert.NoError(t, err)
		assert.False(t, deps.Empty())
		assert.ElementsMatch(t, []string{"CephRBDMirror"}, deps.PluralKinds())
		assert.ElementsMatch(t, []string{"rbdmirror"}, deps.OfKind("CephRBDMirror"))
	})

	t.Run("CephFilesystems", func(t *testing.T) {
		c = newClusterdCtx(
			&cephv1.CephFilesystem{ObjectMeta: meta("filesystem-1")},
			&cephv1.CephFilesystem{ObjectMeta: meta("filesystem-2")},
			&cephv1.CephFilesystem{ObjectMeta: meta("filesystem-3")},
		)
		deps, err := CephClusterDependents(c, ns)
		assert.NoError(t, err)
		assert.False(t, deps.Empty())
		assert.ElementsMatch(t, []string{"CephFilesystem"}, deps.PluralKinds())
		assert.ElementsMatch(t, []string{"filesystem-1", "filesystem-2", "filesystem-3"}, deps.OfKind("CephFilesystem"))
	})

	t.Run("CephFilesystemMirrors", func(t *testing.T) {
		c = newClusterdCtx(
			&cephv1.CephFilesystemMirror{ObjectMeta: meta("fsmirror")},
		)
		deps, err := CephClusterDependents(c, ns)
		assert.NoError(t, err)
		assert.False(t, deps.Empty())
		assert.ElementsMatch(t, []string{"CephFilesystemMirror"}, deps.PluralKinds())
		assert.ElementsMatch(t, []string{"fsmirror"}, deps.OfKind("CephFilesystemMirror"))
	})

	t.Run("CephObjectStores", func(t *testing.T) {
		c = newClusterdCtx(
			&cephv1.CephObjectStore{ObjectMeta: meta("objectstore-1")},
			&cephv1.CephObjectStore{ObjectMeta: meta("objectstore-2")},
		)
		deps, err := CephClusterDependents(c, ns)
		assert.NoError(t, err)
		assert.False(t, deps.Empty())
		assert.ElementsMatch(t, []string{"CephObjectStore"}, deps.PluralKinds())
		assert.ElementsMatch(t, []string{"objectstore-1", "objectstore-2"}, deps.OfKind("CephObjectStore"))
	})

	t.Run("CephObjectStoreUsers", func(t *testing.T) {
		c = newClusterdCtx(
			&cephv1.CephObjectStoreUser{ObjectMeta: meta("u1")},
			&cephv1.CephObjectStoreUser{ObjectMeta: meta("u2")},
			&cephv1.CephObjectStoreUser{ObjectMeta: meta("u3")},
			&cephv1.CephObjectStoreUser{ObjectMeta: meta("u4")},
			&cephv1.CephObjectStoreUser{ObjectMeta: meta("u5")},
		)
		deps, err := CephClusterDependents(c, ns)
		assert.NoError(t, err)
		assert.False(t, deps.Empty())
		assert.ElementsMatch(t, []string{"CephObjectStoreUser"}, deps.PluralKinds())
		assert.ElementsMatch(t, []string{"u1", "u2", "u3", "u4", "u5"}, deps.OfKind("CephObjectStoreUser"))
	})

	t.Run("CephObjectZones", func(t *testing.T) {
		c = newClusterdCtx(
			&cephv1.CephObjectZone{ObjectMeta: meta("zone-1")},
			&cephv1.CephObjectZone{ObjectMeta: meta("zone-2")},
		)
		deps, err := CephClusterDependents(c, ns)
		assert.NoError(t, err)
		assert.False(t, deps.Empty())
		assert.ElementsMatch(t, []string{"CephObjectZone"}, deps.PluralKinds())
		assert.ElementsMatch(t, []string{"zone-1", "zone-2"}, deps.OfKind("CephObjectZone"))
	})

	t.Run("CephObjectZoneGroups", func(t *testing.T) {
		c = newClusterdCtx(
			&cephv1.CephObjectZoneGroup{ObjectMeta: meta("group-1")},
			&cephv1.CephObjectZoneGroup{ObjectMeta: meta("group-2")},
		)
		deps, err := CephClusterDependents(c, ns)
		assert.NoError(t, err)
		assert.False(t, deps.Empty())
		assert.ElementsMatch(t, []string{"CephObjectZoneGroup"}, deps.PluralKinds())
		assert.ElementsMatch(t, []string{"group-1", "group-2"}, deps.OfKind("CephObjectZoneGroup"))
	})

	t.Run("CephObjectRealms", func(t *testing.T) {
		c = newClusterdCtx(
			&cephv1.CephObjectRealm{ObjectMeta: meta("realm-1")},
			&cephv1.CephObjectRealm{ObjectMeta: meta("realm-2")},
		)
		deps, err := CephClusterDependents(c, ns)
		assert.NoError(t, err)
		assert.False(t, deps.Empty())
		assert.ElementsMatch(t, []string{"CephObjectRealm"}, deps.PluralKinds())
		assert.ElementsMatch(t, []string{"realm-1", "realm-2"}, deps.OfKind("CephObjectRealm"))
	})

	t.Run("CephNFSes", func(t *testing.T) {
		c = newClusterdCtx(
			&cephv1.CephNFS{ObjectMeta: meta("nfs-1")},
			&cephv1.CephNFS{ObjectMeta: meta("nfs-2")},
		)
		deps, err := CephClusterDependents(c, ns)
		assert.NoError(t, err)
		assert.False(t, deps.Empty())
		assert.ElementsMatch(t, []string{"CephNFS"}, deps.PluralKinds())
		assert.ElementsMatch(t, []string{"nfs-1", "nfs-2"}, deps.OfKind("CephNFS"))
	})

	t.Run("CephClients", func(t *testing.T) {
		c = newClusterdCtx(
			&cephv1.CephClient{ObjectMeta: meta("client-1")},
			&cephv1.CephClient{ObjectMeta: meta("client-2")},
			&cephv1.CephClient{ObjectMeta: meta("client-3")},
		)
		deps, err := CephClusterDependents(c, ns)
		assert.NoError(t, err)
		assert.False(t, deps.Empty())
		assert.ElementsMatch(t, []string{"CephClient"}, deps.PluralKinds())
		assert.ElementsMatch(t, []string{"client-1", "client-2", "client-3"}, deps.OfKind("CephClient"))
	})

	t.Run("CephBucketTopics", func(t *testing.T) {
		c = newClusterdCtx(
			&cephv1.CephBucketTopic{ObjectMeta: meta("topic-1")},
			&cephv1.CephBucketTopic{ObjectMeta: meta("topic-2")},
			&cephv1.CephBucketTopic{ObjectMeta: meta("topic-3")},
		)
		deps, err := CephClusterDependents(c, ns)
		assert.NoError(t, err)
		assert.False(t, deps.Empty())
		assert.ElementsMatch(t, []string{"CephBucketTopic"}, deps.PluralKinds())
		assert.ElementsMatch(t, []string{"topic-1", "topic-2", "topic-3"}, deps.OfKind("CephBucketTopic"))
	})

	t.Run("CephBucketNotifications", func(t *testing.T) {
		c = newClusterdCtx(
			&cephv1.CephBucketNotification{ObjectMeta: meta("notif-1")},
			&cephv1.CephBucketNotification{ObjectMeta: meta("notif-2")},
			&cephv1.CephBucketNotification{ObjectMeta: meta("notif-3")},
		)
		deps, err := CephClusterDependents(c, ns)
		assert.NoError(t, err)
		assert.False(t, deps.Empty())
		assert.ElementsMatch(t, []string{"CephBucketNotification"}, deps.PluralKinds())
		assert.ElementsMatch(t, []string{"notif-1", "notif-2", "notif-3"}, deps.OfKind("CephBucketNotification"))
	})

	t.Run("All", func(t *testing.T) {
		c = newClusterdCtx(
			&cephv1.CephBlockPool{ObjectMeta: meta("pool-1")},
			&cephv1.CephRBDMirror{ObjectMeta: meta("rbdmirror-1")},
			&cephv1.CephRBDMirror{ObjectMeta: meta("rbdmirror-2")},
			&cephv1.CephFilesystem{ObjectMeta: meta("filesystem-1")},
			&cephv1.CephFilesystemMirror{ObjectMeta: meta("fsmirror-1")},
			&cephv1.CephFilesystemMirror{ObjectMeta: meta("fsmirror-2")},
			&cephv1.CephObjectStore{ObjectMeta: meta("objectstore-1")},
			&cephv1.CephObjectStoreUser{ObjectMeta: meta("u1")},
			&cephv1.CephObjectZone{ObjectMeta: meta("zone-1")},
			&cephv1.CephObjectZoneGroup{ObjectMeta: meta("group-1")},
			&cephv1.CephObjectRealm{ObjectMeta: meta("realm-1")},
			&cephv1.CephNFS{ObjectMeta: meta("nfs-1")},
			&cephv1.CephClient{ObjectMeta: meta("client-1")},
			&cephv1.CephBucketTopic{ObjectMeta: meta("topic-1")},
			&cephv1.CephBucketNotification{ObjectMeta: meta("notif-1")},
		)
		deps, err := CephClusterDependents(c, ns)
		assert.NoError(t, err)
		assert.False(t, deps.Empty())
		assert.ElementsMatch(t, []string{
			"CephBlockPool", "CephRBDMirror", "CephFilesystem",
			"CephFilesystemMirror", "CephObjectStore", "CephObjectStoreUser", "CephObjectZone",
			"CephObjectZoneGroup", "CephObjectRealm", "CephNFS", "CephClient", "CephBucketTopic", "CephBucketNotification",
		}, deps.PluralKinds())
		assert.ElementsMatch(t, []string{"pool-1"}, deps.OfKind("CephBlockPool"))
		assert.ElementsMatch(t, []string{"rbdmirror-1", "rbdmirror-2"}, deps.OfKind("CephRBDMirror"))
		assert.ElementsMatch(t, []string{"filesystem-1"}, deps.OfKind("CephFilesystem"))
		assert.ElementsMatch(t, []string{"fsmirror-1", "fsmirror-2"}, deps.OfKind("CephFilesystemMirror"))
		assert.ElementsMatch(t, []string{"objectstore-1"}, deps.OfKind("CephObjectStore"))
		assert.ElementsMatch(t, []string{"u1"}, deps.OfKind("CephObjectStoreUser"))
		assert.ElementsMatch(t, []string{"zone-1"}, deps.OfKind("CephObjectZone"))
		assert.ElementsMatch(t, []string{"group-1"}, deps.OfKind("CephObjectZoneGroup"))
		assert.ElementsMatch(t, []string{"realm-1"}, deps.OfKind("CephObjectRealm"))
		assert.ElementsMatch(t, []string{"nfs-1"}, deps.OfKind("CephNFS"))
		assert.ElementsMatch(t, []string{"client-1"}, deps.OfKind("CephClient"))
		assert.ElementsMatch(t, []string{"topic-1"}, deps.OfKind("CephBucketTopic"))
		assert.ElementsMatch(t, []string{"notif-1"}, deps.OfKind("CephBucketNotification"))

		t.Run("and no dependencies in another namespace", func(t *testing.T) {
			deps, err := CephClusterDependents(c, "other-namespace")
			assert.NoError(t, err)
			assert.True(t, deps.Empty())
		})
	})

	// TODO: how do we get this to return errors without a reactor? Do we need to add reactors
	// to controller-runtime?
	// Keep the below test commented-out until we can check off the above TODO. For now we will have
	// to assume the errors are handled properly.
	// t.Run("With errors", func(t *testing.T) {
	// 	// // add reactor to cause failures when listing block and nfs (but not object, fs, or any others)
	// 	// var listReactor k8stesting.ReactionFunc = func(action k8stesting.Action) (handled bool, ret runtime.Object, err error) {
	// 	// 	r := action.GetResource().Resource
	// 	// 	if r == "cephblockpools" || r == "cephnfses" {
	// 	// 		return true, nil, errors.Errorf("fake error listing %q", r)
	// 	// 	}
	// 	// 	return false, nil, nil
	// 	// }
	// 	// dynInt.PrependReactor("list", "*", listReactor)

	// 	client := fakeclient.NewClientBuilder().WithScheme(scheme).WithObjects(
	// 		&cephv1.CephBlockPool{ObjectMeta: meta("pool-1")},
	// 		&cephv1.CephFilesystem{ObjectMeta: meta("filesystem-1")},
	// 		&cephv1.CephObjectStore{ObjectMeta: meta("objectstore-1")},
	// 	).Build()

	// 	c := &clusterd.Context{
	// 		Client: client,
	// 	}

	// 	deps, err := CephClusterDependents(c, ns)
	// 	assert.Error(t, err)
	// 	assert.Contains(t, err.Error(), "CephBlockPool")
	// 	assert.Contains(t, err.Error(), "CephNFS")
	// 	assert.False(t, deps.Empty())
	// 	assert.ElementsMatch(t, []string{"CephFilesystem", "CephObjectStore"}, deps.PluralKinds())
	// 	assert.ElementsMatch(t, []string{"filesystem-1"}, deps.OfKind("CephFilesystem"))
	// 	assert.ElementsMatch(t, []string{"objectstore-1"}, deps.OfKind("CephObjectStore"))
	// })
}
