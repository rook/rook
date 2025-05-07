/*
Copyright 2020 The Rook Authors. All rights reserved.

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

package reporting

import (
	"context"
	"fmt"
	"testing"

	cephv1 "github.com/rook/rook/pkg/apis/ceph.rook.io/v1"
	"github.com/rook/rook/pkg/client/clientset/versioned/scheme"
	"github.com/stretchr/testify/assert"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func TestUpdateStatus(t *testing.T) {
	t.Run("status does not exist initially", func(t *testing.T) {
		fakeObject := &cephv1.CephBlockPool{
			ObjectMeta: metav1.ObjectMeta{
				Name:       "test",
				Namespace:  "rook-ceph",
				Finalizers: []string{},
			},
			// Status: unset
		}
		nsName := types.NamespacedName{
			Namespace: fakeObject.Namespace,
			Name:      fakeObject.Name,
		}

		s := scheme.Scheme
		s.AddKnownTypes(cephv1.SchemeGroupVersion, fakeObject)
		// have to use deepcopy to ensure the tracker doesn't have the pointer for our test object
		cl := fake.NewClientBuilder().WithScheme(s).WithRuntimeObjects(fakeObject.DeepCopy()).Build()

		// get version of the object in the fake object tracker
		getObj := &cephv1.CephBlockPool{}
		err := cl.Get(context.TODO(), nsName, getObj)
		assert.NoError(t, err)

		objCpy := getObj.DeepCopy()
		getObj.Status = &cephv1.CephBlockPoolStatus{
			Phase: cephv1.ConditionProgressing,
		}
		err = UpdateStatus(cl, getObj)
		assert.NoError(t, err)

		updObj := &cephv1.CephBlockPool{}
		err = cl.Get(context.TODO(), nsName, updObj)
		assert.NoError(t, err)

		fmt.Println(objCpy)
	})

	fakeObject := &cephv1.CephBlockPool{
		ObjectMeta: metav1.ObjectMeta{
			Name:       "test",
			Namespace:  "rook-ceph",
			Finalizers: []string{},
		},
		Status: &cephv1.CephBlockPoolStatus{
			Phase: "",
		},
	}
	nsName := types.NamespacedName{
		Namespace: fakeObject.Namespace,
		Name:      fakeObject.Name,
	}

	s := scheme.Scheme
	s.AddKnownTypes(cephv1.SchemeGroupVersion, fakeObject)
	// use deepcopy to ensure the tracker doesn't have the pointer for our test object
	cl := fake.NewClientBuilder().WithScheme(s).WithRuntimeObjects(fakeObject.DeepCopy()).Build()

	// get version of the object in the fake object tracker
	getObj := &cephv1.CephBlockPool{}
	err := cl.Get(context.TODO(), nsName, getObj)
	assert.NoError(t, err)

	getObj.Status.Phase = cephv1.ConditionReady
	err = UpdateStatus(cl, getObj)
	assert.NoError(t, err)

	err = cl.Get(context.TODO(), nsName, getObj)
	assert.NoError(t, err)
	assert.Equal(t, cephv1.ConditionReady, getObj.Status.Phase)
}

func TestUpdateStatusCondition(t *testing.T) {
	fakeObject := &cephv1.CephObjectStore{
		ObjectMeta: metav1.ObjectMeta{
			Name:       "test",
			Namespace:  "rook-ceph",
			Finalizers: []string{},
		},
		Status: &cephv1.ObjectStoreStatus{
			Phase: cephv1.ConditionDeleting,
		},
	}
	fakeBlock := &cephv1.CephBlockPool{
		ObjectMeta: metav1.ObjectMeta{
			Name:       "mypool",
			Namespace:  "rook-ceph",
			Finalizers: []string{},
		},
		Status: &cephv1.CephBlockPoolStatus{
			Phase: cephv1.ConditionDeleting,
		},
	}
	nsName := types.NamespacedName{
		Namespace: fakeObject.Namespace,
		Name:      fakeObject.Name,
	}
	poolName := types.NamespacedName{
		Namespace: fakeBlock.Namespace,
		Name:      fakeBlock.Name,
	}

	s := scheme.Scheme
	s.AddKnownTypes(cephv1.SchemeGroupVersion, fakeObject, fakeBlock)
	cl := fake.NewClientBuilder().WithScheme(s).WithRuntimeObjects(fakeObject.DeepCopy(), fakeBlock.DeepCopy()).Build()

	// get version of the objects in the fake object tracker
	getObj := &cephv1.CephObjectStore{}
	err := cl.Get(context.TODO(), nsName, getObj)
	assert.NoError(t, err)
	assert.Zero(t, len(getObj.Status.Conditions))
	getBlock := &cephv1.CephBlockPool{}
	err = cl.Get(context.TODO(), poolName, getBlock)
	assert.NoError(t, err)
	assert.Zero(t, len(getBlock.Status.Conditions))

	t.Run("add new status", func(t *testing.T) {
		getObj := &cephv1.CephObjectStore{}
		err := cl.Get(context.TODO(), nsName, getObj)
		assert.NoError(t, err)

		startCond := cephv1.Condition{
			Type:    cephv1.ConditionDeletionIsBlocked,
			Status:  v1.ConditionTrue, // changed
			Reason:  "start",          // changed
			Message: "start",          // changed
		}

		err = UpdateStatusCondition(cl, getObj, startCond)
		assert.NoError(t, err)

		err = cl.Get(context.TODO(), nsName, getObj)
		assert.NoError(t, err)
		cond := getObj.Status.Conditions[0]
		assert.Equal(t, v1.ConditionTrue, cond.Status)
		assert.Equal(t, cephv1.ConditionReason("start"), cond.Reason)
		assert.Equal(t, "start", cond.Message)
	})

	t.Run("add two statuses", func(t *testing.T) {
		getObj := &cephv1.CephBlockPool{}
		err := cl.Get(context.TODO(), poolName, getObj)
		assert.NoError(t, err)

		blockedCond := cephv1.Condition{
			Type:    cephv1.ConditionDeletionIsBlocked,
			Status:  v1.ConditionTrue,
			Reason:  cephv1.ConditionReason("dep"),
			Message: "there is a dependency",
		}
		emptyCond := cephv1.Condition{
			Type:    cephv1.ConditionPoolDeletionIsBlocked,
			Status:  v1.ConditionTrue,
			Reason:  cephv1.ConditionReason("notempty"),
			Message: "images are in the pool",
		}

		err = UpdateStatusCondition(cl, getObj, blockedCond, emptyCond)
		assert.NoError(t, err)

		err = cl.Get(context.TODO(), poolName, getObj)
		assert.NoError(t, err)
		cond := getObj.Status.Conditions[0]
		assert.Equal(t, v1.ConditionTrue, cond.Status)
		assert.Equal(t, blockedCond.Reason, cond.Reason)
		assert.Equal(t, blockedCond.Message, cond.Message)

		cond = getObj.Status.Conditions[1]
		assert.Equal(t, v1.ConditionTrue, cond.Status)
		assert.Equal(t, emptyCond.Reason, cond.Reason)
		assert.Equal(t, emptyCond.Message, cond.Message)
	})

	t.Run("update status", func(t *testing.T) {
		getObj := &cephv1.CephObjectStore{}
		err := cl.Get(context.TODO(), nsName, getObj)
		assert.NoError(t, err)

		updatedCond := cephv1.Condition{
			Type:    cephv1.ConditionDeletionIsBlocked,
			Status:  v1.ConditionFalse, // changed
			Reason:  "update",          // changed
			Message: "update",          // changed
		}

		err = UpdateStatusCondition(cl, getObj, updatedCond)
		assert.NoError(t, err)

		err = cl.Get(context.TODO(), nsName, getObj)
		assert.NoError(t, err)
		cond := getObj.Status.Conditions[0]
		assert.Equal(t, v1.ConditionFalse, cond.Status)
		assert.Equal(t, cephv1.ConditionReason("update"), cond.Reason)
		assert.Equal(t, "update", cond.Message)
	})
}
