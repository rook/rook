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
	"github.com/stretchr/testify/assert"
)

func newConfig() *clusterConfig {
	return &clusterConfig{
		store: cephv1.CephObjectStore{
			Spec: cephv1.ObjectStoreSpec{
				Gateway: cephv1.GatewaySpec{},
			}}}
}

func TestPortString(t *testing.T) {
	// No port or secure port
	cfg := newConfig()
	result := cfg.portString()
	assert.Equal(t, "", result)

	// Insecure port
	cfg = newConfig()
	cfg.store.Spec.Gateway.Port = 80
	result = cfg.portString()
	assert.Equal(t, "80", result)

	// Secure port
	cfg = newConfig()
	cfg.store.Spec.Gateway.SecurePort = 443
	cfg.store.Spec.Gateway.SSLCertificateRef = "some-k8s-key-secret"
	result = cfg.portString()
	assert.Equal(t, "443s ssl_certificate=/etc/ceph/private/rgw-cert.pem", result)

	// Both ports
	cfg = newConfig()
	cfg.store.Spec.Gateway.Port = 80
	cfg.store.Spec.Gateway.SecurePort = 443
	cfg.store.Spec.Gateway.SSLCertificateRef = "some-k8s-key-secret"
	result = cfg.portString()
	assert.Equal(t, "80+443s ssl_certificate=/etc/ceph/private/rgw-cert.pem", result)

	// Secure port requires the cert
	cfg = newConfig()
	cfg.store.Spec.Gateway.SecurePort = 443
	result = cfg.portString()
	assert.Equal(t, "", result)
}
