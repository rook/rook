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

package zonegroup

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

func TestCephObjectZoneGroupDependentZones(t *testing.T) {
	ctx := context.TODO()
	zoneGroup := &cephv1.CephObjectZoneGroup{
		ObjectMeta: metav1.ObjectMeta{Name: "zonegroup-a", Namespace: "rook-ceph"},
	}
	zone := func(zoneName, zoneNamespace, zoneGroupName string) *cephv1.CephObjectZone {
		return &cephv1.CephObjectZone{
			ObjectMeta: metav1.ObjectMeta{Name: zoneName, Namespace: zoneNamespace},
			Spec:       cephv1.ObjectZoneSpec{ZoneGroup: zoneGroupName},
		}
	}
	deletingZone := zone("zone-a", "rook-ceph", "zonegroup-a")
	deletingZone.Finalizers = []string{"cephobjectzone.ceph.rook.io"}
	deletingZone.DeletionTimestamp = &metav1.Time{Time: time.Now()}

	tests := []struct {
		name  string
		zones []runtime.Object
		want  []string
	}{
		{
			name: "no zones exist",
		},
		{
			name:  "one zone references the zone group",
			zones: []runtime.Object{zone("zone-a", "rook-ceph", "zonegroup-a")},
			want:  []string{"zone-a"},
		},
		{
			name:  "zone references a different zone group",
			zones: []runtime.Object{zone("zone-b", "rook-ceph", "zonegroup-b")},
		},
		{
			name:  "zone references a zone group of the same name in another namespace",
			zones: []runtime.Object{zone("zone-a", "other-namespace", "zonegroup-a")},
		},
		{
			name: "several zones reference the zone group",
			zones: []runtime.Object{
				zone("zone-a", "rook-ceph", "zonegroup-a"),
				zone("zone-b", "rook-ceph", "zonegroup-b"),
				zone("zone-c", "rook-ceph", "zonegroup-a"),
			},
			want: []string{"zone-a", "zone-c"},
		},
		{
			name:  "zone that is being deleted still references the zone group",
			zones: []runtime.Object{deletingZone},
			want:  []string{"zone-a"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := &clusterd.Context{RookClientset: rookclient.NewSimpleClientset(tt.zones...)}

			deps, err := CephObjectZoneGroupDependentZones(ctx, c, zoneGroup)
			require.NoError(t, err)
			assert.ElementsMatch(t, tt.want, deps.OfKind("CephObjectZones"))
			assert.Equal(t, len(tt.want) == 0, deps.Empty())
		})
	}

	t.Run("list error is returned", func(t *testing.T) {
		rookClient := rookclient.NewSimpleClientset()
		rookClient.PrependReactor("list", "cephobjectzones", func(k8stesting.Action) (bool, runtime.Object, error) {
			return true, nil, errors.New("list failed")
		})

		_, err := CephObjectZoneGroupDependentZones(ctx, &clusterd.Context{RookClientset: rookClient}, zoneGroup)
		assert.ErrorContains(t, err, "list failed")
	})
}
