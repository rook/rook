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

package osd

import (
	"context"

	cephv1 "github.com/rook/rook/pkg/apis/ceph.rook.io/v1"
	"github.com/rook/rook/pkg/clusterd"
	cephclient "github.com/rook/rook/pkg/daemon/ceph/client"
	"k8s.io/client-go/kubernetes/fake"
)

func newTestReplaceCluster(clientset *fake.Clientset) *Cluster {
	return newTestReplaceClusterWithSpec(clientset, cephv1.ClusterSpec{})
}

func newTestReplaceClusterWithSpec(clientset *fake.Clientset, spec cephv1.ClusterSpec) *Cluster {
	clusterInfo := &cephclient.ClusterInfo{
		Namespace: "rook-ceph",
		Context:   context.TODO(),
		OwnerInfo: cephclient.NewMinimumOwnerInfoWithOwnerRef(),
	}
	clusterInfo.SetName("test")
	c := &clusterd.Context{Clientset: clientset}
	return &Cluster{
		context:     c,
		clusterInfo: clusterInfo,
		rookVersion: "rook/ceph:test",
		spec:        spec,
	}
}
