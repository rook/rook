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
	"context"
	"testing"

	cephv1 "github.com/rook/rook/pkg/apis/ceph.rook.io/v1"
	rookclient "github.com/rook/rook/pkg/client/clientset/versioned/fake"
	"github.com/rook/rook/pkg/clusterd"
	"github.com/rook/rook/pkg/daemon/ceph/client"
	"github.com/stretchr/testify/assert"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
)

func Test_cephBlockPoolDependents(t *testing.T) {
	ctx := context.TODO()
	scheme := runtime.NewScheme()
	assert.NoError(t, cephv1.AddToScheme(scheme))
	ns := "test-ceph-blockpool-dependents"
	var c *clusterd.Context

	newClusterdCtx := func(objects ...runtime.Object) *clusterd.Context {
		return &clusterd.Context{
			RookClientset: rookclient.NewSimpleClientset(),
		}
	}

	clusterInfo := client.AdminTestClusterInfo(ns)
	// Create objectmeta with the given name in our test namespace
	meta := func(name string) v1.ObjectMeta {
		return v1.ObjectMeta{
			Name:      name,
			Namespace: ns,
		}
	}

	bp := &cephv1.CephBlockPool{
		ObjectMeta: v1.ObjectMeta{
			Name:      "replicapool",
			Namespace: ns,
		},
	}

	t.Run("no namespaces", func(t *testing.T) {
		c = newClusterdCtx()
		deps, err := cephBlockPoolDependents(c, clusterInfo, bp)
		assert.NoError(t, err)
		assert.True(t, deps.Empty())
	})

	t.Run("one namespace but wrong pool", func(t *testing.T) {
		otherFs := &cephv1.CephBlockPool{
			ObjectMeta: v1.ObjectMeta{
				Name:      "otherns",
				Namespace: ns,
			},
		}

		c = newClusterdCtx(&cephv1.CephBlockPoolRadosNamespace{ObjectMeta: meta("namespace1")})
		_, err := c.RookClientset.CephV1().CephBlockPoolRadosNamespaces(clusterInfo.Namespace).Create(ctx, &cephv1.CephBlockPoolRadosNamespace{ObjectMeta: meta("namespace1")}, v1.CreateOptions{})
		assert.NoError(t, err)
		assert.NoError(t, err)
		deps, err := cephBlockPoolDependents(c, clusterInfo, otherFs)
		assert.NoError(t, err)
		assert.True(t, deps.Empty())
	})

	t.Run("one namespace", func(t *testing.T) {
		c = newClusterdCtx(&cephv1.CephBlockPoolRadosNamespace{ObjectMeta: meta("namespace1")})
		_, err := c.RookClientset.CephV1().CephBlockPoolRadosNamespaces(clusterInfo.Namespace).Create(ctx, &cephv1.CephBlockPoolRadosNamespace{ObjectMeta: meta("namespace1"), Spec: cephv1.CephBlockPoolRadosNamespaceSpec{
			BlockPoolName: "replicapool"}}, v1.CreateOptions{})
		assert.NoError(t, err)
		assert.NoError(t, err)
		deps, err := cephBlockPoolDependents(c, clusterInfo, bp)
		assert.NoError(t, err)
		assert.False(t, deps.Empty())
	})

}
