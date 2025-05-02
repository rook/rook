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

package pool

import (
	"fmt"

	"github.com/pkg/errors"
	cephv1 "github.com/rook/rook/pkg/apis/ceph.rook.io/v1"
	"github.com/rook/rook/pkg/clusterd"
	"github.com/rook/rook/pkg/daemon/ceph/client"
	"github.com/rook/rook/pkg/util/dependents"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const radosNamespacesKeyName = "CephBlockPoolRadosNamespaces"

// cephBlockPoolDependents returns the rbd namespaces (s) which exist in the rbd pool that should block
// deletion.
func cephBlockPoolDependents(clusterdCtx *clusterd.Context, clusterInfo *client.ClusterInfo, blockpool *cephv1.CephBlockPool) (*dependents.DependentList, error) {
	nsName := fmt.Sprintf("%s/%s", blockpool.Namespace, blockpool.Name)
	baseErrMsg := fmt.Sprintf("failed to get dependents of CephBlockPool %q", nsName)

	deps := dependents.NewDependentList()

	// CepbBlockPoolNamespaces
	namespaces, err := clusterdCtx.RookClientset.CephV1().CephBlockPoolRadosNamespaces(blockpool.Namespace).List(clusterInfo.Context, metav1.ListOptions{})
	if err != nil {
		return deps, errors.Wrapf(err, "%s. failed to list CephBlockPoolRadosNamespace for CephBlockPool %q", baseErrMsg, nsName)
	}
	for i, namespace := range namespaces.Items {
		if namespace.Spec.BlockPoolName == blockpool.Name {
			deps.Add(radosNamespacesKeyName, cephv1.GetRadosNamespaceName(&namespaces.Items[i]))
		}
		logger.Debugf("found CephBlockPoolRadosNamespace %q that does not depend on CephBlockPool %q", namespace.Name, nsName)
	}

	return deps, nil
}
