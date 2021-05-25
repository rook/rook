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
	"context"
	"strings"

	"github.com/pkg/errors"
	cephv1 "github.com/rook/rook/pkg/apis/ceph.rook.io/v1"
	"github.com/rook/rook/pkg/clusterd"
	"github.com/rook/rook/pkg/util"
	"github.com/rook/rook/pkg/util/dependents"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var (
	// must use plural kinds
	cephClusterDependentPluralKinds []string = []string{
		"CephBlockPools",
		"CephRBDMirrors",
		"CephFilesystems",
		"CephFilesystemMirrors",
		"CephObjectStores",
		"CephObjectStoreUsers",
		"CephObjectZones",
		"CephObjectZoneGroups",
		"CephObjectRealms",
		"CephNFSes",
		"CephClients",
	}
)

// CephClusterDependents returns a DependentList of dependents of a CephCluster in the namespace.
func CephClusterDependents(c *clusterd.Context, namespace string) (*dependents.DependentList, error) {
	ctx := context.TODO()

	dependents := dependents.NewDependentList()
	errs := []error{}

	for _, pluralKind := range cephClusterDependentPluralKinds {
		resource := pluralKindToResource(pluralKind)
		gvr := cephv1.SchemeGroupVersion.WithResource(resource)
		list, err := c.DynamicClientset.Resource(gvr).Namespace(namespace).List(ctx, metav1.ListOptions{})
		if err != nil {
			errs = append(errs, errors.Wrapf(err, "failed to list %s", pluralKind))
			continue
		}
		if len(list.Items) > 0 {
			for _, obj := range list.Items {
				dependents.Add(pluralKind, obj.GetName())
			}
		}
	}
	// returns a nil error if there are no errors in the list
	outErr := util.AggregateErrors(errs, "failed to list some dependents for CephCluster in namespace %q", namespace)

	return dependents, outErr
}

func pluralKindToResource(pluralKind string) string {
	// The dynamic client wants resources which are lower-case versions of the plural Kinds for
	// Kubernetes CRDs in almost all cases.
	return strings.ToLower(pluralKind)
}
