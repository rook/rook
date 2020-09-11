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

package object

import (
	"testing"

	cephv1 "github.com/rook/rook/pkg/apis/ceph.rook.io/v1"
	"github.com/stretchr/testify/assert"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestBuildStatusInfo(t *testing.T) {
	// Port enabled and SecurePort disabled
	cephObjectStore := &cephv1.CephObjectStore{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "my-store",
			Namespace: "rook-ceph",
		},
	}
	cephObjectStore.Spec.Gateway.Port = 80

	statusInfo := buildStatusInfo(cephObjectStore)
	assert.NotEmpty(t, statusInfo["endpoint"])
	assert.Empty(t, statusInfo["secureEndpoint"])
	assert.Equal(t, "http://rook-ceph-rgw-my-store.rook-ceph.svc:80", statusInfo["endpoint"])

	// SecurePort enabled and Port disabled
	cephObjectStore.Spec.Gateway.Port = 0
	cephObjectStore.Spec.Gateway.SecurePort = 443

	statusInfo = buildStatusInfo(cephObjectStore)
	assert.NotEmpty(t, statusInfo["endpoint"])
	assert.Empty(t, statusInfo["secureEndpoint"])
	assert.Equal(t, "https://rook-ceph-rgw-my-store.rook-ceph.svc:443", statusInfo["endpoint"])

	// Both Port and SecurePort enabled
	cephObjectStore.Spec.Gateway.Port = 80
	cephObjectStore.Spec.Gateway.SecurePort = 443

	statusInfo = buildStatusInfo(cephObjectStore)
	assert.NotEmpty(t, statusInfo["endpoint"])
	assert.NotEmpty(t, statusInfo["secureEndpoint"])
	assert.Equal(t, "http://rook-ceph-rgw-my-store.rook-ceph.svc:80", statusInfo["endpoint"])
	assert.Equal(t, "https://rook-ceph-rgw-my-store.rook-ceph.svc:443", statusInfo["secureEndpoint"])
}
