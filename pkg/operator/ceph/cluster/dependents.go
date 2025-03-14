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
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// must use plural kinds
var cephClusterDependentListKinds []string = []string{
	"CephBlockPoolList",
	"CephRBDMirrorList",
	"CephFilesystemList",
	"CephFilesystemMirrorList",
	"CephObjectStoreList",
	"CephObjectStoreUserList",
	"CephObjectZoneList",
	"CephObjectZoneGroupList",
	"CephObjectRealmList",
	"CephNFSList",
	"CephClientList",
	"CephBucketTopic",
	"CephBucketNotification",
	"CephFilesystemSubVolumeGroup",
	"CephBlockPoolRadosNamespace",
}

// CephClusterDependents returns a DependentList of dependents of a CephCluster in the namespace.
func CephClusterDependents(c *clusterd.Context, namespace string) (*dependents.DependentList, error) {
	ctx := context.TODO()

	dependents := dependents.NewDependentList()
	errs := []error{}

	for _, listKind := range cephClusterDependentListKinds {
		list := &unstructured.UnstructuredList{}
		list.SetGroupVersionKind(schema.GroupVersionKind{
			Group:   cephv1.SchemeGroupVersion.Group,
			Version: cephv1.SchemeGroupVersion.Version,
			Kind:    listKind,
		})

		err := c.Client.List(ctx, list, client.InNamespace(namespace))
		if err != nil {
			errs = append(errs, errors.Wrapf(err, "failed to get %s", listKind))
			continue
		}
		for _, obj := range list.Items {
			dependents.Add(listKindToSingularKind(listKind), obj.GetName())
		}
	}
	// returns a nil error if there are no errors in the list
	outErr := util.AggregateErrors(errs, "failed to list some dependents for CephCluster in namespace %q", namespace)

	return dependents, outErr
}

func listKindToSingularKind(listKind string) string {
	return strings.TrimSuffix(listKind, "List")
}
