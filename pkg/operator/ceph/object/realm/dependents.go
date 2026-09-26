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
	"fmt"

	"github.com/pkg/errors"
	cephv1 "github.com/rook/rook/pkg/apis/ceph.rook.io/v1"
	"github.com/rook/rook/pkg/clusterd"
	opcontroller "github.com/rook/rook/pkg/operator/ceph/controller"
	"github.com/rook/rook/pkg/util/dependents"
	"github.com/rook/rook/pkg/util/log"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// CephObjectRealmDependentZoneGroups returns the CephObjectZoneGroups that reference the realm.
func CephObjectRealmDependentZoneGroups(
	ctx context.Context,
	clusterdCtx *clusterd.Context,
	realm *cephv1.CephObjectRealm,
) (*dependents.DependentList, error) {
	nsName := opcontroller.NsName(realm.Namespace, realm.Name)
	baseErrMsg := fmt.Sprintf("failed to get dependents of CephObjectRealm %q", nsName)

	deps := dependents.NewDependentList()
	zoneGroups, err := clusterdCtx.RookClientset.CephV1().CephObjectZoneGroups(realm.Namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		return deps, errors.Wrapf(err, "%s. failed to list CephObjectZoneGroups", baseErrMsg)
	}
	for _, zoneGroup := range zoneGroups.Items {
		if zoneGroup.Spec.Realm == realm.Name {
			deps.Add("CephObjectZoneGroups", zoneGroup.Name)
			log.NamedDebug(nsName, logger, "found CephObjectZoneGroup %q that depends on CephObjectRealm %q", zoneGroup.Name, nsName)
		}
	}

	return deps, nil
}
