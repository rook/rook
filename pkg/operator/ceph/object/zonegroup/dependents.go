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
	"fmt"

	"github.com/pkg/errors"
	cephv1 "github.com/rook/rook/pkg/apis/ceph.rook.io/v1"
	"github.com/rook/rook/pkg/clusterd"
	opcontroller "github.com/rook/rook/pkg/operator/ceph/controller"
	"github.com/rook/rook/pkg/util/dependents"
	"github.com/rook/rook/pkg/util/log"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// CephObjectZoneGroupDependentZones returns the CephObjectZones that reference the zone group.
func CephObjectZoneGroupDependentZones(
	ctx context.Context,
	clusterdCtx *clusterd.Context,
	zoneGroup *cephv1.CephObjectZoneGroup,
) (*dependents.DependentList, error) {
	nsName := opcontroller.NsName(zoneGroup.Namespace, zoneGroup.Name)
	baseErrMsg := fmt.Sprintf("failed to get dependents of CephObjectZoneGroup %q", nsName)

	deps := dependents.NewDependentList()
	zones, err := clusterdCtx.RookClientset.CephV1().CephObjectZones(zoneGroup.Namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		return deps, errors.Wrapf(err, "%s. failed to list CephObjectZones", baseErrMsg)
	}
	for _, zone := range zones.Items {
		if zone.Spec.ZoneGroup == zoneGroup.Name {
			deps.Add("CephObjectZones", zone.Name)
			log.NamedDebug(nsName, logger, "found CephObjectZone %q that depends on CephObjectZoneGroup %q", zone.Name, nsName)
		}
	}

	return deps, nil
}
