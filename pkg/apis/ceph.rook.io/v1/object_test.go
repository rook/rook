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

package v1

import (
	"testing"

	"github.com/stretchr/testify/assert"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestValidateObjectStoreSpec(t *testing.T) {
	o := &CephObjectStore{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "my-store",
			Namespace: "rook-ceph",
		},
		Spec: ObjectStoreSpec{
			Gateway: GatewaySpec{
				Port:       1,
				SecurePort: 0,
			},
		},
	}
	err := ValidateObjectSpec(o)
	assert.NoError(t, err)

	// when both port and securePort are o
	o.Spec.Gateway.Port = 0
	err = ValidateObjectSpec(o)
	assert.Error(t, err)

	// when securePort is greater than 65535
	o.Spec.Gateway.SecurePort = 65536
	err = ValidateObjectSpec(o)
	assert.Error(t, err)

	// when name is empty
	o.ObjectMeta.Name = ""
	err = ValidateObjectSpec(o)
	assert.Error(t, err)

	// when namespace is empty
	o.ObjectMeta.Namespace = ""
	err = ValidateObjectSpec(o)
	assert.Error(t, err)
}
func TestIsTLSEnabled(t *testing.T) {
	objStore := &CephObjectStore{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "my-store",
			Namespace: "rook-ceph",
		},
		Spec: ObjectStoreSpec{
			Gateway: GatewaySpec{
				Port:       1,
				SecurePort: 0,
			},
		},
	}
	IsTLS := objStore.Spec.IsTLSEnabled()
	assert.False(t, IsTLS)

	// only securePort is set without certs
	objStore.Spec.Gateway.SecurePort = 443
	IsTLS = objStore.Spec.IsTLSEnabled()
	assert.False(t, IsTLS)

	// when SSLCertificateRef is set with securePort
	objStore.Spec.Gateway.SSLCertificateRef = "my-tls-cert"
	IsTLS = objStore.Spec.IsTLSEnabled()
	assert.True(t, IsTLS)

	// when service serving cert is used
	objStore.Spec.Gateway.SSLCertificateRef = ""
	objStore.Spec.Gateway.Service = &(RGWServiceSpec{Annotations: Annotations{ServiceServingCertKey: "rgw-cert"}})
	IsTLS = objStore.Spec.IsTLSEnabled()
	assert.True(t, IsTLS)

	// when cert are set but securePort unset
	objStore.Spec.Gateway.SecurePort = 0
	IsTLS = objStore.Spec.IsTLSEnabled()
	assert.False(t, IsTLS)
}
