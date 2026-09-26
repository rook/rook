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

package realm

import (
	"context"
	"errors"
	"testing"
	"time"

	cephv1 "github.com/rook/rook/pkg/apis/ceph.rook.io/v1"
	rookclient "github.com/rook/rook/pkg/client/clientset/versioned/fake"
	"github.com/rook/rook/pkg/clusterd"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	k8stesting "k8s.io/client-go/testing"
)

func TestCephObjectRealmDependentZoneGroups(t *testing.T) {
	ctx := context.TODO()
	realm := &cephv1.CephObjectRealm{
		ObjectMeta: metav1.ObjectMeta{Name: "realm-a", Namespace: "rook-ceph"},
	}
	zoneGroup := func(zoneGroupName, zoneGroupNamespace, realmName string) *cephv1.CephObjectZoneGroup {
		return &cephv1.CephObjectZoneGroup{
			ObjectMeta: metav1.ObjectMeta{Name: zoneGroupName, Namespace: zoneGroupNamespace},
			Spec:       cephv1.ObjectZoneGroupSpec{Realm: realmName},
		}
	}
	deletingZoneGroup := zoneGroup("zonegroup-a", "rook-ceph", "realm-a")
	deletingZoneGroup.Finalizers = []string{"cephobjectzonegroup.ceph.rook.io"}
	deletingZoneGroup.DeletionTimestamp = &metav1.Time{Time: time.Now()}

	tests := []struct {
		name       string
		zoneGroups []runtime.Object
		want       []string
	}{
		{
			name: "no zone groups exist",
		},
		{
			name:       "one zone group references the realm",
			zoneGroups: []runtime.Object{zoneGroup("zonegroup-a", "rook-ceph", "realm-a")},
			want:       []string{"zonegroup-a"},
		},
		{
			name:       "zone group references a different realm",
			zoneGroups: []runtime.Object{zoneGroup("zonegroup-b", "rook-ceph", "realm-b")},
		},
		{
			name:       "zone group references a realm of the same name in another namespace",
			zoneGroups: []runtime.Object{zoneGroup("zonegroup-a", "other-namespace", "realm-a")},
		},
		{
			name: "several zone groups reference the realm",
			zoneGroups: []runtime.Object{
				zoneGroup("zonegroup-a", "rook-ceph", "realm-a"),
				zoneGroup("zonegroup-b", "rook-ceph", "realm-b"),
				zoneGroup("zonegroup-c", "rook-ceph", "realm-a"),
			},
			want: []string{"zonegroup-a", "zonegroup-c"},
		},
		{
			name:       "zone group that is being deleted still references the realm",
			zoneGroups: []runtime.Object{deletingZoneGroup},
			want:       []string{"zonegroup-a"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := &clusterd.Context{RookClientset: rookclient.NewSimpleClientset(tt.zoneGroups...)}

			deps, err := CephObjectRealmDependentZoneGroups(ctx, c, realm)
			require.NoError(t, err)
			assert.ElementsMatch(t, tt.want, deps.OfKind("CephObjectZoneGroups"))
			assert.Equal(t, len(tt.want) == 0, deps.Empty())
		})
	}

	t.Run("list error is returned", func(t *testing.T) {
		rookClient := rookclient.NewSimpleClientset()
		rookClient.PrependReactor("list", "cephobjectzonegroups", func(k8stesting.Action) (bool, runtime.Object, error) {
			return true, nil, errors.New("list failed")
		})

		_, err := CephObjectRealmDependentZoneGroups(ctx, &clusterd.Context{RookClientset: rookClient}, realm)
		assert.ErrorContains(t, err, "list failed")
	})
}
