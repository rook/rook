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
package rgw

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestPortString(t *testing.T) {
	// No port or secure port
	cfg := &Config{}
	result := portString(cfg)
	assert.Equal(t, "", result)

	// Insecure port
	cfg = &Config{Port: 80}
	result = portString(cfg)
	assert.Equal(t, "80", result)

	// Secure port
	cfg = &Config{SecurePort: 443, CertificatePath: "/etc/rgw/cert.pem"}
	result = portString(cfg)
	assert.Equal(t, "443s ssl_certificate=/etc/rgw/cert.pem", result)

	// Both ports
	cfg = &Config{Port: 80, SecurePort: 443, CertificatePath: "/etc/rgw/cert.pem"}
	result = portString(cfg)
	assert.Equal(t, "80+443s ssl_certificate=/etc/rgw/cert.pem", result)

	// Secure port requires the cert
	cfg = &Config{SecurePort: 443}
	result = portString(cfg)
	assert.Equal(t, "", result)
}
