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

package fixture

import (
	storagev1 "k8s.io/api/storage/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	cephv1 "github.com/rook/rook/pkg/apis/ceph.rook.io/v1"
)

// StorageClass returns a StorageClass for the lib-bucket-provisioner bucket
// provisioner backed by objectStore, for use by ObjectBucketClaims.
func StorageClass(name string, objectStore *cephv1.CephObjectStore) *storagev1.StorageClass {
	return &storagev1.StorageClass{
		ObjectMeta: metav1.ObjectMeta{
			Name: name,
		},
		Provisioner: objectStore.Namespace + ".ceph.rook.io/bucket",
		Parameters: map[string]string{
			"objectStoreName":      objectStore.Name,
			"objectStoreNamespace": objectStore.Namespace,
		},
	}
}
