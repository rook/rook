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

	"github.com/pkg/errors"
	cephv1 "github.com/rook/rook/pkg/apis/ceph.rook.io/v1"
	"github.com/rook/rook/pkg/clusterd"
	"github.com/stretchr/testify/assert"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	dynamicfake "k8s.io/client-go/dynamic/fake"
	k8stesting "k8s.io/client-go/testing"
)

func TestCephClusterDependents(t *testing.T) {
	scheme := runtime.NewScheme()
	assert.NoError(t, cephv1.AddToScheme(scheme))

	ns := "test-ceph-cluster-dependents"

	var c *clusterd.Context

	newClusterdCtx := func(objects ...runtime.Object) *clusterd.Context {
		dynInt := dynamicfake.NewSimpleDynamicClient(scheme, objects...)
		return &clusterd.Context{
			DynamicClientset: dynInt,
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
		assert.ElementsMatch(t, []string{"CephBlockPools"}, deps.PluralKinds())
		assert.ElementsMatch(t, []string{"block-pool-1", "block-pool-2"}, deps.OfPluralKind("CephBlockPools"))
	})

	t.Run("CephRBDMirrors", func(t *testing.T) {
		c = newClusterdCtx(
			&cephv1.CephRBDMirror{ObjectMeta: meta("rbdmirror")},
		)
		deps, err := CephClusterDependents(c, ns)
		assert.NoError(t, err)
		assert.False(t, deps.Empty())
		assert.ElementsMatch(t, []string{"CephRBDMirrors"}, deps.PluralKinds())
		assert.ElementsMatch(t, []string{"rbdmirror"}, deps.OfPluralKind("CephRBDMirrors"))
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
		assert.ElementsMatch(t, []string{"CephFilesystems"}, deps.PluralKinds())
		assert.ElementsMatch(t, []string{"filesystem-1", "filesystem-2", "filesystem-3"}, deps.OfPluralKind("CephFilesystems"))
	})

	t.Run("CephFilesystemMirrors", func(t *testing.T) {
		c = newClusterdCtx(
			&cephv1.CephFilesystemMirror{ObjectMeta: meta("fsmirror")},
		)
		deps, err := CephClusterDependents(c, ns)
		assert.NoError(t, err)
		assert.False(t, deps.Empty())
		assert.ElementsMatch(t, []string{"CephFilesystemMirrors"}, deps.PluralKinds())
		assert.ElementsMatch(t, []string{"fsmirror"}, deps.OfPluralKind("CephFilesystemMirrors"))
	})

	t.Run("CephObjectStores", func(t *testing.T) {
		c = newClusterdCtx(
			&cephv1.CephObjectStore{ObjectMeta: meta("objectstore-1")},
			&cephv1.CephObjectStore{ObjectMeta: meta("objectstore-2")},
		)
		deps, err := CephClusterDependents(c, ns)
		assert.NoError(t, err)
		assert.False(t, deps.Empty())
		assert.ElementsMatch(t, []string{"CephObjectStores"}, deps.PluralKinds())
		assert.ElementsMatch(t, []string{"objectstore-1", "objectstore-2"}, deps.OfPluralKind("CephObjectStores"))
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
		assert.ElementsMatch(t, []string{"CephObjectStoreUsers"}, deps.PluralKinds())
		assert.ElementsMatch(t, []string{"u1", "u2", "u3", "u4", "u5"}, deps.OfPluralKind("CephObjectStoreUsers"))
	})

	t.Run("CephObjectZones", func(t *testing.T) {
		c = newClusterdCtx(
			&cephv1.CephObjectZone{ObjectMeta: meta("zone-1")},
			&cephv1.CephObjectZone{ObjectMeta: meta("zone-2")},
		)
		deps, err := CephClusterDependents(c, ns)
		assert.NoError(t, err)
		assert.False(t, deps.Empty())
		assert.ElementsMatch(t, []string{"CephObjectZones"}, deps.PluralKinds())
		assert.ElementsMatch(t, []string{"zone-1", "zone-2"}, deps.OfPluralKind("CephObjectZones"))
	})

	t.Run("CephObjectZoneGroups", func(t *testing.T) {
		c = newClusterdCtx(
			&cephv1.CephObjectZoneGroup{ObjectMeta: meta("group-1")},
			&cephv1.CephObjectZoneGroup{ObjectMeta: meta("group-2")},
		)
		deps, err := CephClusterDependents(c, ns)
		assert.NoError(t, err)
		assert.False(t, deps.Empty())
		assert.ElementsMatch(t, []string{"CephObjectZoneGroups"}, deps.PluralKinds())
		assert.ElementsMatch(t, []string{"group-1", "group-2"}, deps.OfPluralKind("CephObjectZoneGroups"))
	})

	t.Run("CephObjectRealms", func(t *testing.T) {
		c = newClusterdCtx(
			&cephv1.CephObjectRealm{ObjectMeta: meta("realm-1")},
			&cephv1.CephObjectRealm{ObjectMeta: meta("realm-2")},
		)
		deps, err := CephClusterDependents(c, ns)
		assert.NoError(t, err)
		assert.False(t, deps.Empty())
		assert.ElementsMatch(t, []string{"CephObjectRealms"}, deps.PluralKinds())
		assert.ElementsMatch(t, []string{"realm-1", "realm-2"}, deps.OfPluralKind("CephObjectRealms"))
	})

	t.Run("CephNFSes", func(t *testing.T) {
		c = newClusterdCtx(
			&cephv1.CephNFS{ObjectMeta: meta("nfs-1")},
			&cephv1.CephNFS{ObjectMeta: meta("nfs-2")},
		)
		deps, err := CephClusterDependents(c, ns)
		assert.NoError(t, err)
		assert.False(t, deps.Empty())
		assert.ElementsMatch(t, []string{"CephNFSes"}, deps.PluralKinds())
		assert.ElementsMatch(t, []string{"nfs-1", "nfs-2"}, deps.OfPluralKind("CephNFSes"))
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
		assert.ElementsMatch(t, []string{"CephClients"}, deps.PluralKinds())
		assert.ElementsMatch(t, []string{"client-1", "client-2", "client-3"}, deps.OfPluralKind("CephClients"))
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
		)
		deps, err := CephClusterDependents(c, ns)
		assert.NoError(t, err)
		assert.False(t, deps.Empty())
		assert.ElementsMatch(t, []string{"CephBlockPools", "CephRBDMirrors", "CephFilesystems",
			"CephFilesystemMirrors", "CephObjectStores", "CephObjectStoreUsers", "CephObjectZones",
			"CephObjectZoneGroups", "CephObjectRealms", "CephNFSes", "CephClients"}, deps.PluralKinds())
		assert.ElementsMatch(t, []string{"pool-1"}, deps.OfPluralKind("CephBlockPools"))
		assert.ElementsMatch(t, []string{"rbdmirror-1", "rbdmirror-2"}, deps.OfPluralKind("CephRBDMirrors"))
		assert.ElementsMatch(t, []string{"filesystem-1"}, deps.OfPluralKind("CephFilesystems"))
		assert.ElementsMatch(t, []string{"fsmirror-1", "fsmirror-2"}, deps.OfPluralKind("CephFilesystemMirrors"))
		assert.ElementsMatch(t, []string{"objectstore-1"}, deps.OfPluralKind("CephObjectStores"))
		assert.ElementsMatch(t, []string{"u1"}, deps.OfPluralKind("CephObjectStoreUsers"))
		assert.ElementsMatch(t, []string{"zone-1"}, deps.OfPluralKind("CephObjectZones"))
		assert.ElementsMatch(t, []string{"group-1"}, deps.OfPluralKind("CephObjectZoneGroups"))
		assert.ElementsMatch(t, []string{"realm-1"}, deps.OfPluralKind("CephObjectRealms"))
		assert.ElementsMatch(t, []string{"nfs-1"}, deps.OfPluralKind("CephNFSes"))
		assert.ElementsMatch(t, []string{"client-1"}, deps.OfPluralKind("CephClients"))

		t.Run("and no dependencies in another namespace", func(t *testing.T) {
			deps, err := CephClusterDependents(c, "other-namespace")
			assert.NoError(t, err)
			assert.True(t, deps.Empty())
		})
	})

	t.Run("With errors", func(t *testing.T) {
		dynInt := dynamicfake.NewSimpleDynamicClient(scheme,
			&cephv1.CephBlockPool{ObjectMeta: meta("pool-1")},
			&cephv1.CephFilesystem{ObjectMeta: meta("filesystem-1")},
			&cephv1.CephObjectStore{ObjectMeta: meta("objectstore-1")},
		)
		// add reactor to cause failures when listing block and nfs (but not object, fs, or any others)
		var listReactor k8stesting.ReactionFunc = func(action k8stesting.Action) (handled bool, ret runtime.Object, err error) {
			r := action.GetResource().Resource
			if r == "cephblockpools" || r == "cephnfses" {
				return true, nil, errors.Errorf("fake error listing %q", r)
			}
			return false, nil, nil
		}
		dynInt.PrependReactor("list", "*", listReactor)
		c := &clusterd.Context{
			DynamicClientset: dynInt,
		}
		deps, err := CephClusterDependents(c, ns)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "CephBlockPools")
		assert.Contains(t, err.Error(), "CephNFSes")
		assert.False(t, deps.Empty())
		assert.ElementsMatch(t, []string{"CephFilesystems", "CephObjectStores"}, deps.PluralKinds())
		assert.ElementsMatch(t, []string{"filesystem-1"}, deps.OfPluralKind("CephFilesystems"))
		assert.ElementsMatch(t, []string{"objectstore-1"}, deps.OfPluralKind("CephObjectStores"))
	})
}
