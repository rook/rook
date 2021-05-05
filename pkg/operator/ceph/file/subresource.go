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

// Define CephFilesystem as a subresource of CephCluster

package file

import (
	"context"
	"fmt"

	"github.com/pkg/errors"
	cephv1 "github.com/rook/rook/pkg/apis/ceph.rook.io/v1"
	"github.com/rook/rook/pkg/clusterd"
	"github.com/rook/rook/pkg/operator/ceph/subresource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func init() {
	err := subresource.CephClusterRegistry.Register(&fileSubresource{})
	if err != nil {
		// Safe to panic because this should only ever happen during initialization, which will be
		// caught in development.
		panic(fmt.Sprintf("failed to register CephFilesystem as a subresource of CephCluster. %v", err))
	}
}

type fileSubresource struct{}

func (s *fileSubresource) Kind() string {
	return cephFilesystemKind
}

func (s *fileSubresource) DependentsOf(clusterdCtx *clusterd.Context, obj client.Object) ([]string, error) {
	cephCluster, ok := obj.(*cephv1.CephCluster)
	if !ok {
		return []string{}, errors.Errorf("failed to find CephFilesystem dependents. given object could not be converted to a CephCluster: %+v", obj)
	}
	ns := cephCluster.Namespace
	ctx := context.TODO()

	// all and only CephFilesystems in the CephCluster namespace are dependents of the cluster
	fsList, err := clusterdCtx.RookClientset.CephV1().CephFilesystems(ns).List(ctx, metav1.ListOptions{})
	if err != nil {
		return []string{}, errors.Wrapf(err, "failed to list CephFilesystem dependents of CephCluster in namespace %q", ns)
	}

	fsNames := make([]string, 0, len(fsList.Items))
	for _, fs := range fsList.Items {
		fsNames = append(fsNames, fs.Name)
	}
	return fsNames, nil
}
