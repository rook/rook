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

package nodedaemon

import (
	"context"
	"testing"

	cephv1 "github.com/rook/rook/pkg/apis/ceph.rook.io/v1"
	rookclient "github.com/rook/rook/pkg/client/clientset/versioned/fake"
	"github.com/rook/rook/pkg/client/clientset/versioned/scheme"
	"github.com/rook/rook/pkg/clusterd"
<<<<<<< HEAD
	cephver "github.com/rook/rook/pkg/operator/ceph/version"
	"github.com/rook/rook/pkg/operator/test"
	"github.com/stretchr/testify/assert"
	v1 "k8s.io/api/batch/v1"
	"k8s.io/api/batch/v1beta1"
	kerrors "k8s.io/apimachinery/pkg/api/errors"
=======
	"github.com/rook/rook/pkg/operator/test"
	"github.com/stretchr/testify/assert"
	v1 "k8s.io/api/batch/v1"
>>>>>>> fc08e87d4 (Revert "object: create cosi user for each object store")
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

func TestCreateOrUpdateCephCron(t *testing.T) {
	cephCluster := cephv1.CephCluster{ObjectMeta: metav1.ObjectMeta{Namespace: "rook-ceph"}}
<<<<<<< HEAD
	cephVersion := &cephver.CephVersion{Major: 17, Minor: 2, Extra: 0}
=======
>>>>>>> fc08e87d4 (Revert "object: create cosi user for each object store")
	ctx := context.TODO()
	context := &clusterd.Context{
		Clientset:     test.New(t, 1),
		RookClientset: rookclient.NewSimpleClientset(),
	}

	s := scheme.Scheme
	err := v1.AddToScheme(s)
	if err != nil {
		assert.Fail(t, "failed to build scheme")
	}
<<<<<<< HEAD
	err = v1beta1.AddToScheme(s)
	if err != nil {
		assert.Fail(t, "failed to build scheme")
	}
=======
>>>>>>> fc08e87d4 (Revert "object: create cosi user for each object store")

	r := &ReconcileNode{
		scheme:  s,
		client:  fake.NewClientBuilder().WithScheme(s).WithRuntimeObjects().Build(),
		context: context,
	}

	cronV1 := &v1.CronJob{
		ObjectMeta: metav1.ObjectMeta{
			Name:      prunerName,
			Namespace: "rook-ceph",
		},
	}

<<<<<<< HEAD
	cronV1Beta1 := &v1beta1.CronJob{
		ObjectMeta: metav1.ObjectMeta{
			Name:      prunerName,
			Namespace: "rook-ceph",
		},
	}

	// check if v1beta1 cronJob is present and v1 cronJob is not
	cntrlutil, err := r.createOrUpdateCephCron(cephCluster, cephVersion, false)
	assert.NoError(t, err)
	assert.Equal(t, cntrlutil, controllerutil.OperationResult("created"))

	err = r.client.Get(ctx, types.NamespacedName{Namespace: "rook-ceph", Name: prunerName}, cronV1Beta1)
	assert.NoError(t, err)

	err = r.client.Get(ctx, types.NamespacedName{Namespace: "rook-ceph", Name: prunerName}, cronV1)
	assert.Error(t, err)
	assert.True(t, kerrors.IsNotFound(err))

	// check if v1 cronJob is present and v1beta1 cronJob is not
	cntrlutil, err = r.createOrUpdateCephCron(cephCluster, cephVersion, true)
=======
	// check if cronJob is created
	cntrlutil, err := r.createOrUpdateCephCron(cephCluster)
>>>>>>> fc08e87d4 (Revert "object: create cosi user for each object store")
	assert.NoError(t, err)
	assert.Equal(t, cntrlutil, controllerutil.OperationResult("created"))

	err = r.client.Get(ctx, types.NamespacedName{Namespace: "rook-ceph", Name: prunerName}, cronV1)
	assert.NoError(t, err)
<<<<<<< HEAD

	err = r.client.Get(ctx, types.NamespacedName{Namespace: "rook-ceph", Name: prunerName}, cronV1Beta1)
	assert.Error(t, err)
	assert.True(t, kerrors.IsNotFound(err))
=======
>>>>>>> fc08e87d4 (Revert "object: create cosi user for each object store")
}
