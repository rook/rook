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

package k8sutil

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	v1 "k8s.io/api/core/v1"
	storagev1 "k8s.io/api/storage/v1"
	apiresource "k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func TestExpandPVCIfRequired(t *testing.T) {
	testcases := []struct {
		label            string
		currentPVCSize   string
		desiredPVCSize   string
		expansionAllowed bool
	}{
		{
			label:            "case 1: size is equal",
			currentPVCSize:   "1Mi",
			desiredPVCSize:   "1Mi",
			expansionAllowed: true,
		},
		{
			label:            "case 2: current size is less",
			currentPVCSize:   "1Mi",
			desiredPVCSize:   "2Mi",
			expansionAllowed: true,
		},
		{
			label:            "case 3: current size is more",
			currentPVCSize:   "2Mi",
			desiredPVCSize:   "1Mi",
			expansionAllowed: true,
		},
		{
			label:            "case 4: storage class allows expansion",
			currentPVCSize:   "1Mi",
			desiredPVCSize:   "2Mi",
			expansionAllowed: true,
		},
		{
			label:            "case 5: storage class does not allow expansion",
			currentPVCSize:   "1Mi",
			desiredPVCSize:   "2Mi",
			expansionAllowed: false,
		},
	}

	storageClass := &storagev1.StorageClass{
		ObjectMeta: metav1.ObjectMeta{
			Name: "test",
		},
	}

	desiredPVC := &v1.PersistentVolumeClaim{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test",
			Namespace: "rook-ceph",
		},
		Spec: v1.PersistentVolumeClaimSpec{
			AccessModes: []v1.PersistentVolumeAccessMode{
				v1.ReadWriteOnce,
			},
			Resources: v1.VolumeResourceRequirements{
				Requests: v1.ResourceList{
					v1.ResourceName(v1.ResourceStorage): apiresource.MustParse("1Mi"),
				},
			},
			StorageClassName: &storageClass.ObjectMeta.Name,
		},
	}

	for _, tc := range testcases {

		desiredPVC.Spec.Resources.Requests[v1.ResourceStorage] = apiresource.MustParse(tc.currentPVCSize)
		expansionAllowed := tc.expansionAllowed
		storageClass.AllowVolumeExpansion = &expansionAllowed

		// create fake client with PVC
		cl := fake.NewClientBuilder().WithRuntimeObjects(desiredPVC, storageClass).Build()

		// get existing PVC
		existingPVC := &v1.PersistentVolumeClaim{}
		err := cl.Get(context.TODO(), client.ObjectKey{Name: "test", Namespace: "rook-ceph"}, existingPVC)
		assert.NoError(t, err)

		desiredPVC.Spec.Resources.Requests[v1.ResourceStorage] = apiresource.MustParse(tc.desiredPVCSize)

		ExpandPVCIfRequired(context.TODO(), cl, desiredPVC, existingPVC)

		// get existing PVC
		err = cl.Get(context.TODO(), client.ObjectKey{Name: "test", Namespace: "rook-ceph"}, existingPVC)
		assert.NoError(t, err)

		// verify size
		if tc.currentPVCSize <= tc.desiredPVCSize && tc.expansionAllowed {
			assert.Equal(t, desiredPVC.Spec.Resources.Requests[v1.ResourceStorage], existingPVC.Spec.Resources.Requests[v1.ResourceStorage])
		} else {
			assert.Equal(t, apiresource.MustParse(tc.currentPVCSize), existingPVC.Spec.Resources.Requests[v1.ResourceStorage])
		}
	}
}
