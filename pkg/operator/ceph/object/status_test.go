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
	baseStore := &cephv1.CephObjectStore{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "my-store",
			Namespace: "rook-ceph",
		},
	}

	// Port enabled and SecurePort disabled
	s := baseStore.DeepCopy()
	s.Spec.Gateway.Port = 80
	statusInfo := buildStatusInfo(s)

	assert.NotEmpty(t, statusInfo["endpoint"])
	assert.Empty(t, statusInfo["secureEndpoint"])
	assert.Equal(t, "http://rook-ceph-rgw-my-store.rook-ceph.svc:80", statusInfo["endpoint"])

	// SecurePort enabled and Port disabled
	s = baseStore.DeepCopy()
	s.Spec.Gateway.Port = 0
	s.Spec.Gateway.SecurePort = 443
	s.Spec.Gateway.SSLCertificateRef = "my-cert"

	statusInfo = buildStatusInfo(s)
	assert.NotEmpty(t, statusInfo["endpoint"])
	assert.Empty(t, statusInfo["secureEndpoint"])
	assert.Equal(t, "https://rook-ceph-rgw-my-store.rook-ceph.svc:443", statusInfo["endpoint"])

	// Both Port and SecurePort enabled
	s = baseStore.DeepCopy()
	s.Spec.Gateway.Port = 80
	s.Spec.Gateway.SecurePort = 443
	s.Spec.Gateway.SSLCertificateRef = "my-cert"

	statusInfo = buildStatusInfo(s)
	assert.NotEmpty(t, statusInfo["endpoint"])
	assert.NotEmpty(t, statusInfo["secureEndpoint"])
	assert.Equal(t, "http://rook-ceph-rgw-my-store.rook-ceph.svc:80", statusInfo["endpoint"])
	assert.Equal(t, "https://rook-ceph-rgw-my-store.rook-ceph.svc:443", statusInfo["secureEndpoint"])

	t.Run("advertiseEndpoint http", func(t *testing.T) {
		baseStore := &cephv1.CephObjectStore{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "my-store",
				Namespace: "rook-ceph",
			},
			Spec: cephv1.ObjectStoreSpec{
				Hosting: &cephv1.ObjectStoreHostingSpec{
					AdvertiseEndpoint: &cephv1.ObjectEndpointSpec{
						DnsName: "my.endpoint.com",
						Port:    80,
						UseTls:  false,
					},
				},
			},
		}

		// Port enabled and SecurePort disabled
		s := baseStore.DeepCopy()
		s.Spec.Gateway.Port = 80
		statusInfo := buildStatusInfo(s)

		assert.NotEmpty(t, statusInfo["endpoint"])
		assert.Empty(t, statusInfo["secureEndpoint"])
		assert.Equal(t, "http://my.endpoint.com:80", statusInfo["endpoint"])

		// SecurePort enabled and Port disabled
		s = baseStore.DeepCopy()
		s.Spec.Gateway.Port = 0
		s.Spec.Gateway.SecurePort = 443
		s.Spec.Gateway.SSLCertificateRef = "my-cert"

		statusInfo = buildStatusInfo(s)
		assert.NotEmpty(t, statusInfo["endpoint"])
		assert.Empty(t, statusInfo["secureEndpoint"])
		assert.Equal(t, "http://my.endpoint.com:80", statusInfo["endpoint"])

		// Both Port and SecurePort enabled
		s = baseStore.DeepCopy()
		s.Spec.Gateway.Port = 80
		s.Spec.Gateway.SecurePort = 443
		s.Spec.Gateway.SSLCertificateRef = "my-cert"

		statusInfo = buildStatusInfo(s)
		assert.NotEmpty(t, statusInfo["endpoint"])
		assert.Empty(t, statusInfo["secureEndpoint"])
		assert.Equal(t, "http://my.endpoint.com:80", statusInfo["endpoint"])
	})

	t.Run("advertiseEndpoint https", func(t *testing.T) {
		baseStore := &cephv1.CephObjectStore{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "my-store",
				Namespace: "rook-ceph",
			},
			Spec: cephv1.ObjectStoreSpec{
				Hosting: &cephv1.ObjectStoreHostingSpec{
					AdvertiseEndpoint: &cephv1.ObjectEndpointSpec{
						DnsName: "my.endpoint.com",
						Port:    443,
						UseTls:  true,
					},
				},
			},
		}

		// Port enabled and SecurePort disabled
		s := baseStore.DeepCopy()
		s.Spec.Gateway.Port = 80
		statusInfo := buildStatusInfo(s)

		assert.NotEmpty(t, statusInfo["endpoint"])
		assert.Empty(t, statusInfo["secureEndpoint"])
		assert.Equal(t, "https://my.endpoint.com:443", statusInfo["endpoint"])

		// SecurePort enabled and Port disabled
		s = baseStore.DeepCopy()
		s.Spec.Gateway.Port = 0
		s.Spec.Gateway.SecurePort = 443
		s.Spec.Gateway.SSLCertificateRef = "my-cert"

		statusInfo = buildStatusInfo(s)
		assert.NotEmpty(t, statusInfo["endpoint"])
		assert.Empty(t, statusInfo["secureEndpoint"])
		assert.Equal(t, "https://my.endpoint.com:443", statusInfo["endpoint"])

		// Both Port and SecurePort enabled
		s = baseStore.DeepCopy()
		s.Spec.Gateway.Port = 80
		s.Spec.Gateway.SecurePort = 443
		s.Spec.Gateway.SSLCertificateRef = "my-cert"

		statusInfo = buildStatusInfo(s)
		assert.NotEmpty(t, statusInfo["endpoint"])
		assert.Empty(t, statusInfo["secureEndpoint"])
		assert.Equal(t, "https://my.endpoint.com:443", statusInfo["endpoint"])
	})
}
