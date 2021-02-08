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

// Package rbd for mirroring
package rbd

import (
	"context"
	"fmt"
	"testing"

	cephv1 "github.com/rook/rook/pkg/apis/ceph.rook.io/v1"
	rookclient "github.com/rook/rook/pkg/client/clientset/versioned/fake"
	"github.com/rook/rook/pkg/clusterd"
	"github.com/rook/rook/pkg/daemon/ceph/client"
	"github.com/rook/rook/pkg/operator/k8sutil"
	"github.com/rook/rook/pkg/operator/test"
	exectest "github.com/rook/rook/pkg/util/exec/test"
	"github.com/stretchr/testify/assert"
	apps "k8s.io/api/apps/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestRemoveExtraMirrors(t *testing.T) {

	name := "my-mirror"
	namespace := "rook-ceph"

	executor := &exectest.MockExecutor{
		MockExecuteCommandWithOutputFile: func(command, outfile string, args ...string) (string, error) {
			if args[0] == "auth" && args[1] == "del" {
				return "success", nil
			}
			return "", nil
		},
	}
	clientset := test.New(t, 3)
	c := &clusterd.Context{
		Executor:      executor,
		RookClientset: rookclient.NewSimpleClientset(),
		Clientset:     clientset,
	}

	r := &ReconcileCephRBDMirror{
		context:     c,
		clusterInfo: &client.ClusterInfo{Namespace: namespace},
	}

	rbdMirror := &cephv1.CephRBDMirror{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
		Spec: cephv1.RBDMirroringSpec{
			Count: 1,
		},
		TypeMeta: controllerTypeMeta,
	}

	labels := map[string]string{
		k8sutil.AppAttr:     AppName,
		k8sutil.ClusterAttr: r.clusterInfo.Namespace,
	}

	ctx := context.TODO()
	for _, d := range []string{"a", "b"} {
		deployment := &apps.Deployment{
			ObjectMeta: metav1.ObjectMeta{
				Name:      fmt.Sprintf("rbd-mirror-%s", d),
				Namespace: r.clusterInfo.Namespace,
				Labels:    labels,
			},
		}
		deployment.Labels["rbd-mirror"] = d
		_, err := r.context.Clientset.AppsV1().Deployments(r.clusterInfo.Namespace).Create(ctx, deployment, metav1.CreateOptions{})
		assert.NoError(t, err)
	}
	deps, err := r.context.Clientset.AppsV1().Deployments(r.clusterInfo.Namespace).List(ctx, metav1.ListOptions{LabelSelector: fmt.Sprintf("app=%s", AppName)})
	assert.NoError(t, err)
	assert.Equal(t, 2, len(deps.Items))

	err = r.removeExtraMirrors(rbdMirror)
	assert.NoError(t, err)

	deps, err = r.context.Clientset.AppsV1().Deployments(r.clusterInfo.Namespace).List(ctx, metav1.ListOptions{LabelSelector: fmt.Sprintf("app=%s", AppName)})
	assert.NoError(t, err)
	assert.Equal(t, 1, len(deps.Items))
}
