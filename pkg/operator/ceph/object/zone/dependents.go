/*
Copyright 2022 The Rook Authors. All rights reserved.

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

package zone

import (
	"fmt"

	"github.com/pkg/errors"
	v1 "github.com/rook/rook/pkg/apis/ceph.rook.io/v1"
	"github.com/rook/rook/pkg/clusterd"
	"github.com/rook/rook/pkg/daemon/ceph/client"
	"github.com/rook/rook/pkg/operator/ceph/object"
	"github.com/rook/rook/pkg/util/dependents"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func CephObjectZoneDependentStores(
	clusterdCtx *clusterd.Context,
	clusterInfo *client.ClusterInfo,
	zone *v1.CephObjectZone,
	objContext *object.Context,
) (*dependents.DependentList, error) {
	nsName := fmt.Sprintf("%s/%s", zone.Namespace, zone.Name)
	baseErrMsg := fmt.Sprintf("failed to get dependents of CephObjectZone %q", nsName)

	deps := dependents.NewDependentList()
	// CephObjectStores
	stores, err := clusterdCtx.RookClientset.CephV1().CephObjectStores(zone.Namespace).List(clusterInfo.Context, metav1.ListOptions{})
	if err != nil {
		return deps, errors.Wrapf(err, "%s. failed to list CephObjectStores for CephObjectZone %q", baseErrMsg, nsName)
	}
	for _, store := range stores.Items {
		if store.Spec.Zone.Name == zone.Name {
			deps.Add("CephObjectStore", store.Name)
			logger.Debugf("found CephObjectStore %q that depends on CephObjectZone %q", store.Name, nsName)

		} else {
			logger.Debugf("found CephObjectStore %q that does not depend on CephObjectZone %q", store.Name, nsName)
		}
	}

	return deps, nil
}
