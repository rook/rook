/*
Copyright 2016 The Rook Authors. All rights reserved.

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
	"github.com/rook/rook/pkg/clusterd"
	cephclient "github.com/rook/rook/pkg/daemon/ceph/client"
	cephver "github.com/rook/rook/pkg/operator/ceph/version"
	"github.com/rook/rook/pkg/operator/test"
	"github.com/stretchr/testify/assert"
)

func newConfig(t *testing.T) *clusterConfig {
	clusterInfo := &cephclient.ClusterInfo{
		CephVersion: cephver.Squid,
	}
	clusterSpec := &cephv1.ClusterSpec{
		Network: cephv1.NetworkSpec{
			HostNetwork: false,
		},
	}
	return &clusterConfig{
		store: &cephv1.CephObjectStore{
			Spec: cephv1.ObjectStoreSpec{
				Gateway: cephv1.GatewaySpec{},
			}},
		clusterInfo: clusterInfo,
		clusterSpec: clusterSpec,
		context:     &clusterd.Context{Clientset: test.New(t, 3)},
	}
}

func TestPortString(t *testing.T) {
	// No port or secure port on beast
	cfg := newConfig(t)
	result := cfg.portString()
	assert.Equal(t, "", result)

	// Insecure port on beast
	cfg = newConfig(t)
	// Set host networking
	cfg.clusterSpec.Network.HostNetwork = true
	cfg.store.Spec.Gateway.Port = 80
	result = cfg.portString()
	assert.Equal(t, "port=80", result)

	// Secure port on beast
	cfg = newConfig(t)
	cfg.store.Spec.Gateway.SecurePort = 443
	cfg.store.Spec.Gateway.SSLCertificateRef = "some-k8s-key-secret"
	result = cfg.portString()
	assert.Equal(t, "ssl_port=443 ssl_certificate=/etc/ceph/private/rgw-cert.pem", result)

	// Both ports on beast
	cfg = newConfig(t)
	// Set host networking
	cfg.clusterSpec.Network.HostNetwork = true
	cfg.store.Spec.Gateway.Port = 80
	cfg.store.Spec.Gateway.SecurePort = 443
	cfg.store.Spec.Gateway.SSLCertificateRef = "some-k8s-key-secret"
	result = cfg.portString()
	assert.Equal(t, "port=80 ssl_port=443 ssl_certificate=/etc/ceph/private/rgw-cert.pem", result)

	// Secure port requires the cert on beast
	cfg = newConfig(t)
	cfg.store.Spec.Gateway.SecurePort = 443
	result = cfg.portString()
	assert.Equal(t, "", result)

	// Using SDN, no host networking so the rgw port internal is not the same
	cfg = newConfig(t)
	cfg.store.Spec.Gateway.Port = 80
	result = cfg.portString()
	assert.Equal(t, "port=8080", result)
}

func TestGenerateCephXUser(t *testing.T) {
	fakeUser := generateCephXUser("rook-ceph-rgw-fake-store-fake-user")
	assert.Equal(t, "client.rgw.fake.store.fake.user", fakeUser)
}
