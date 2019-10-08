/*
Copyright 2018 The Rook Authors. All rights reserved.

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

// Package cluster to manage a Ceph cluster.
package cluster

import (
	"github.com/pkg/errors"
	cephv1 "github.com/rook/rook/pkg/apis/ceph.rook.io/v1"
)

func getClusterObject(obj interface{}) (cluster *cephv1.CephCluster, err error) {
	var ok bool
	cluster, ok = obj.(*cephv1.CephCluster)
	if ok {
		// the cluster object is of the latest type, simply return it
		cluster = cluster.DeepCopy()
		return cluster, nil
	}

	return nil, errors.Errorf("not a known cluster object: %+v", obj)
}
