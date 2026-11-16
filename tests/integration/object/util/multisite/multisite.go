/*
Copyright 2026 The Rook Authors. All rights reserved.

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

// Package multisite provides the realm, zone group and zone chain that the
// realm, zonegroup and zone packages build for their deletion tests, and the
// checks they share.
package multisite

import (
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"

	cephv1 "github.com/rook/rook/pkg/apis/ceph.rook.io/v1"
	"github.com/rook/rook/tests/integration/object/util/sharedstore"
	"github.com/rook/rook/tests/integration/object/util/wait4"
)

// NewChain returns a realm, a zone group in it, and a zone in that zone group,
// named <prefix>-realm, <prefix>-zonegroup and <prefix>-zone. The realm is not
// the default realm, so the shared store's realm keeps that role. The operator
// creates the zone's own pools from its specs. The CRD defaults
// PreservePoolsOnDelete to true, so it is set to false to make deleting the
// zone delete the pools as well.
func NewChain(prefix, namespace string) (*cephv1.CephObjectRealm, *cephv1.CephObjectZoneGroup, *cephv1.CephObjectZone) {
	realm := &cephv1.CephObjectRealm{
		ObjectMeta: metav1.ObjectMeta{Name: prefix + "-realm", Namespace: namespace},
	}
	zoneGroup := &cephv1.CephObjectZoneGroup{
		ObjectMeta: metav1.ObjectMeta{Name: prefix + "-zonegroup", Namespace: namespace},
		Spec:       cephv1.ObjectZoneGroupSpec{Realm: realm.Name},
	}
	zone := &cephv1.CephObjectZone{
		ObjectMeta: metav1.ObjectMeta{Name: prefix + "-zone", Namespace: namespace},
		Spec: cephv1.ObjectZoneSpec{
			ZoneGroup:             zoneGroup.Name,
			MetadataPool:          cephv1.PoolSpec{Replicated: cephv1.ReplicatedSpec{Size: 1, RequireSafeReplicaSize: false}},
			DataPool:              cephv1.PoolSpec{Replicated: cephv1.ReplicatedSpec{Size: 1, RequireSafeReplicaSize: false}},
			PreservePoolsOnDelete: false,
		},
	}
	return realm, zoneGroup, zone
}

// Conditioned is a Ceph CR whose status carries conditions.
type Conditioned interface {
	runtime.Object
	GetStatusConditions() *[]cephv1.Condition
}

// CheckDeletionBlocked runs a subtest that waits until the deletion of the
// named CR is reported as blocked, and checks that the one dependent of the
// given kind that blocks it is the named one.
func CheckDeletionBlocked[T Conditioned, L runtime.Object](
	t *testing.T,
	client wait4.NamespacedWatcher[T, L],
	kind, name string,
	blocked func(T) bool,
	dependentKind, dependentName string,
) {
	t.Run(fmt.Sprintf("deleting %s %q is blocked by %s %q", kind, name, dependentKind, dependentName), func(t *testing.T) {
		ctx := t.Context()

		live := wait4.RequireCondition(ctx, t, client, name, blocked, wait4.TimeoutMedium)

		cond := cephv1.FindStatusCondition(*live.GetStatusConditions(), cephv1.ConditionDeletionIsBlocked)
		require.NotNil(t, cond)
		assert.Equal(t, cephv1.ObjectHasDependentsReason, cond.Reason)
		assert.Contains(t, cond.Message, fmt.Sprintf("%s: [%s]", dependentKind, dependentName))
	})
}

// CheckPoolsDeleted runs a subtest that checks that none of the named zone's
// pools is left in Ceph. The zone deletes its pools before it releases its
// finalizer, so the check needs no wait once the zone is gone.
func CheckPoolsDeleted(t *testing.T, store *sharedstore.Sharedstore, zoneName string) {
	t.Run(fmt.Sprintf("the pools of CephObjectZone %q are deleted", zoneName), func(t *testing.T) {
		output, err := store.Installer().Execute("ceph", []string{"osd", "pool", "ls"}, store.ObjectStore().Namespace)
		require.NoError(t, err, "failed to list pools; output: %s", output)

		for pool := range strings.FieldsSeq(output) {
			assert.False(t, strings.HasPrefix(pool, zoneName+".rgw."), "pool %q of CephObjectZone %q is left", pool, zoneName)
		}
	})
}
