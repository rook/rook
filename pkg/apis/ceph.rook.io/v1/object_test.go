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

	t.Run("hosting", func(t *testing.T) {
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
				Hosting: &ObjectStoreHostingSpec{
					AdvertiseEndpoint: &ObjectEndpointSpec{
						DnsName: "valid.dns.addr",
						Port:    1,
					},
					DNSNames: []string{"valid.dns.addr", "valid.dns.com"},
				},
			},
		}
		err := ValidateObjectSpec(o)
		assert.NoError(t, err)

		// wildcard advertise dns name
		s := o.DeepCopy()
		s.Spec.Hosting.AdvertiseEndpoint.DnsName = "*.invalid.dns.addr"
		err = ValidateObjectSpec(s)
		assert.ErrorContains(t, err, `"*.invalid.dns.addr"`)

		// empty advertise dns name
		s = o.DeepCopy()
		s.Spec.Hosting.AdvertiseEndpoint.DnsName = ""
		err = ValidateObjectSpec(s)
		assert.ErrorContains(t, err, `""`)

		// zero port
		s = o.DeepCopy()
		s.Spec.Hosting.AdvertiseEndpoint.Port = 0
		err = ValidateObjectSpec(s)
		assert.ErrorContains(t, err, "0")

		// 65536 port
		s = o.DeepCopy()
		s.Spec.Hosting.AdvertiseEndpoint.Port = 65536
		err = ValidateObjectSpec(s)
		assert.ErrorContains(t, err, "65536")

		// first dnsName invalid
		s = o.DeepCopy()
		s.Spec.Hosting.DNSNames = []string{"-invalid.dns.name", "accepted.dns.name"}
		err = ValidateObjectSpec(s)
		assert.ErrorContains(t, err, `"-invalid.dns.name"`)
		assert.NotContains(t, err.Error(), "accepted.dns.name")

		// second dnsName invalid
		s = o.DeepCopy()
		s.Spec.Hosting.DNSNames = []string{"accepted.dns.name", "-invalid.dns.name"}
		err = ValidateObjectSpec(s)
		assert.ErrorContains(t, err, `"-invalid.dns.name"`)
		assert.NotContains(t, err.Error(), "accepted.dns.name")

		// both dnsNames invalid
		s = o.DeepCopy()
		s.Spec.Hosting.DNSNames = []string{"*.invalid.dns.name", "-invalid.dns.name"}
		err = ValidateObjectSpec(s)
		assert.ErrorContains(t, err, `"-invalid.dns.name"`)
		assert.ErrorContains(t, err, `"*.invalid.dns.name"`)
	})
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

func TestCephObjectStore_GetAdvertiseEndpointUrl(t *testing.T) {
	emptySpec := func() *CephObjectStore {
		return &CephObjectStore{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "my-store",
				Namespace: "my-ns",
			},
		}
	}

	httpSpec := func() *CephObjectStore {
		s := emptySpec()
		s.Spec.Gateway.Port = 8080
		return s
	}

	httpsSpec := func() *CephObjectStore {
		s := emptySpec()
		s.Spec.Gateway.SecurePort = 8443
		s.Spec.Gateway.SSLCertificateRef = "my-cert"
		return s
	}

	dualSpec := func() *CephObjectStore {
		s := emptySpec()
		s.Spec.Gateway.Port = 8080
		s.Spec.Gateway.SecurePort = 8443
		s.Spec.Gateway.SSLCertificateRef = "my-cert"
		return s
	}

	removeCert := func(s *CephObjectStore) *CephObjectStore {
		s.Spec.Gateway.SSLCertificateRef = ""
		return s
	}

	initHosting := func(s *CephObjectStore) *CephObjectStore {
		if s.Spec.Hosting == nil {
			s.Spec.Hosting = &ObjectStoreHostingSpec{}
		}
		return s
	}

	addExternalIPs := func(s *CephObjectStore) *CephObjectStore {
		s.Spec.Gateway.ExternalRgwEndpoints = []EndpointAddress{
			{IP: "192.168.1.1"},
			{IP: "192.168.1.2"},
		}
		return s
	}

	addExternalHostnames := func(s *CephObjectStore) *CephObjectStore {
		s.Spec.Gateway.ExternalRgwEndpoints = []EndpointAddress{
			{Hostname: "s3.external.com"},
			{Hostname: "s3.other.com"},
		}
		return s
	}

	addNilAdvertise := func(s *CephObjectStore) *CephObjectStore {
		s = initHosting(s)
		s.Spec.Hosting.AdvertiseEndpoint = nil
		return s
	}

	addAdvertiseHttp := func(s *CephObjectStore) *CephObjectStore {
		s = initHosting(s)
		s.Spec.Hosting.AdvertiseEndpoint = &ObjectEndpointSpec{
			DnsName: "my-endpoint.com",
			Port:    80,
			UseTls:  false,
		}
		return s
	}

	addAdvertiseHttps := func(s *CephObjectStore) *CephObjectStore {
		s = initHosting(s)
		s.Spec.Hosting.AdvertiseEndpoint = &ObjectEndpointSpec{
			DnsName: "my-endpoint.com",
			Port:    443,
			UseTls:  true,
		}
		return s
	}

	type test struct {
		name           string
		store          *CephObjectStore
		want           string
		wantErrContain string
	}

	// base level tests, internal mode
	tests := []test{
		{"nil hosting    : internal          : empty                     ", emptySpec(), "", "Port"},
		{"nil hosting    : internal          : port                      ", httpSpec(), "http://rook-ceph-rgw-my-store.my-ns.svc:8080", ""},
		{"nil hosting    : internal          : securePort                ", httpsSpec(), "https://rook-ceph-rgw-my-store.my-ns.svc:8443", ""},
		{"nil hosting    : internal          : port + securePort         ", dualSpec(), "https://rook-ceph-rgw-my-store.my-ns.svc:8443", ""},
		{"nil hosting    : internal          : securePort, no cert       ", removeCert(httpsSpec()), "", "Port"},
		{"nil hosting    : internal          : port + securePort, no cert", removeCert(dualSpec()), "http://rook-ceph-rgw-my-store.my-ns.svc:8080", ""},
		{"nil hosting    : external IPs      : empty                     ", addExternalIPs(emptySpec()), "", "Port"},
		{"nil hosting    : external IPs      : port                      ", addExternalIPs(httpSpec()), "http://192.168.1.1:8080", ""},
		{"nil hosting    : external IPs      : securePort                ", addExternalIPs(httpsSpec()), "https://192.168.1.1:8443", ""},
		{"nil hosting    : external IPs      : port + securePort         ", addExternalIPs(dualSpec()), "https://192.168.1.1:8443", ""},
		{"nil hosting    : external IPs      : securePort, no cert       ", addExternalIPs(removeCert(httpsSpec())), "", "Port"},
		{"nil hosting    : external IPs      : port + securePort, no cert", addExternalIPs(removeCert(dualSpec())), "http://192.168.1.1:8080", ""},
		{"nil hosting    : external Hostnames: empty                     ", addExternalHostnames(emptySpec()), "", "Port"},
		{"nil hosting    : external Hostnames: port                      ", addExternalHostnames(httpSpec()), "http://s3.external.com:8080", ""},
		{"nil hosting    : external Hostnames: securePort                ", addExternalHostnames(httpsSpec()), "https://s3.external.com:8443", ""},
		{"nil hosting    : external Hostnames: port + securePort         ", addExternalHostnames(dualSpec()), "https://s3.external.com:8443", ""},
		{"nil hosting    : external Hostnames: securePort, no cert       ", addExternalHostnames(removeCert(httpsSpec())), "", "Port"},
		{"nil hosting    : external Hostnames: port + securePort, no cert", addExternalHostnames(removeCert(dualSpec())), "http://s3.external.com:8080", ""},

		{"nil advertise  : internal          : empty                     ", addNilAdvertise(emptySpec()), "", "Port"},
		{"nil advertise  : internal          : port                      ", addNilAdvertise(httpSpec()), "http://rook-ceph-rgw-my-store.my-ns.svc:8080", ""},
		{"nil advertise  : internal          : securePort                ", addNilAdvertise(httpsSpec()), "https://rook-ceph-rgw-my-store.my-ns.svc:8443", ""},
		{"nil advertise  : internal          : port + securePort         ", addNilAdvertise(dualSpec()), "https://rook-ceph-rgw-my-store.my-ns.svc:8443", ""},
		{"nil advertise  : internal          : securePort, no cert       ", addNilAdvertise(removeCert(httpsSpec())), "", "Port"},
		{"nil advertise  : internal          : port + securePort, no cert", addNilAdvertise(removeCert(dualSpec())), "http://rook-ceph-rgw-my-store.my-ns.svc:8080", ""},
		{"nil advertise  : external IPs      : empty                     ", addNilAdvertise(addExternalIPs(emptySpec())), "", "Port"},
		{"nil advertise  : external IPs      : port                      ", addNilAdvertise(addExternalIPs(httpSpec())), "http://192.168.1.1:8080", ""},
		{"nil advertise  : external IPs      : securePort                ", addNilAdvertise(addExternalIPs(httpsSpec())), "https://192.168.1.1:8443", ""},
		{"nil advertise  : external IPs      : port + securePort         ", addNilAdvertise(addExternalIPs(dualSpec())), "https://192.168.1.1:8443", ""},
		{"nil advertise  : external IPs      : securePort, no cert       ", addNilAdvertise(addExternalIPs(removeCert(httpsSpec()))), "", "Port"},
		{"nil advertise  : external IPs      : port + securePort, no cert", addNilAdvertise(addExternalIPs(removeCert(dualSpec()))), "http://192.168.1.1:8080", ""},
		{"nil advertise  : external Hostnames: empty                     ", addNilAdvertise(addExternalHostnames(emptySpec())), "", "Port"},
		{"nil advertise  : external Hostnames: port                      ", addNilAdvertise(addExternalHostnames(httpSpec())), "http://s3.external.com:8080", ""},
		{"nil advertise  : external Hostnames: securePort                ", addNilAdvertise(addExternalHostnames(httpsSpec())), "https://s3.external.com:8443", ""},
		{"nil advertise  : external Hostnames: port + securePort         ", addNilAdvertise(addExternalHostnames(dualSpec())), "https://s3.external.com:8443", ""},
		{"nil advertise  : external Hostnames: securePort, no cert       ", addNilAdvertise(addExternalHostnames(removeCert(httpsSpec()))), "", "Port"},
		{"nil advertise  : external Hostnames: port + securePort, no cert", addNilAdvertise(addExternalHostnames(removeCert(dualSpec()))), "http://s3.external.com:8080", ""},

		{"HTTP advertise : internal          : empty                     ", addAdvertiseHttp(emptySpec()), "", "Port"},
		{"HTTP advertise : internal          : port                      ", addAdvertiseHttp(httpSpec()), "http://my-endpoint.com:80", ""},
		{"HTTP advertise : internal          : securePort                ", addAdvertiseHttp(httpsSpec()), "http://my-endpoint.com:80", ""},
		{"HTTP advertise : internal          : port + securePort         ", addAdvertiseHttp(dualSpec()), "http://my-endpoint.com:80", ""},
		{"HTTP advertise : internal          : securePort, no cert       ", addAdvertiseHttp(removeCert(httpsSpec())), "", "Port"},
		{"HTTP advertise : internal          : port + securePort, no cert", addAdvertiseHttp(removeCert(dualSpec())), "http://my-endpoint.com:80", ""},
		{"HTTP advertise : external IPs      : empty                     ", addAdvertiseHttp(addExternalIPs(emptySpec())), "", "Port"},
		{"HTTP advertise : external IPs      : port                      ", addAdvertiseHttp(addExternalIPs(httpSpec())), "http://my-endpoint.com:80", ""},
		{"HTTP advertise : external IPs      : securePort                ", addAdvertiseHttp(addExternalIPs(httpsSpec())), "http://my-endpoint.com:80", ""},
		{"HTTP advertise : external IPs      : port + securePort         ", addAdvertiseHttp(addExternalIPs(dualSpec())), "http://my-endpoint.com:80", ""},
		{"HTTP advertise : external IPs      : securePort, no cert       ", addAdvertiseHttp(addExternalIPs(removeCert(httpsSpec()))), "", "Port"},
		{"HTTP advertise : external IPs      : port + securePort, no cert", addAdvertiseHttp(addExternalIPs(removeCert(dualSpec()))), "http://my-endpoint.com:80", ""},
		{"HTTP advertise : external Hostnames: empty                     ", addAdvertiseHttp(addExternalHostnames(emptySpec())), "", "Port"},
		{"HTTP advertise : external Hostnames: port                      ", addAdvertiseHttp(addExternalHostnames(httpSpec())), "http://my-endpoint.com:80", ""},
		{"HTTP advertise : external Hostnames: securePort                ", addAdvertiseHttp(addExternalHostnames(httpsSpec())), "http://my-endpoint.com:80", ""},
		{"HTTP advertise : external Hostnames: port + securePort         ", addAdvertiseHttp(addExternalHostnames(dualSpec())), "http://my-endpoint.com:80", ""},
		{"HTTP advertise : external Hostnames: securePort, no cert       ", addAdvertiseHttp(addExternalHostnames(removeCert(httpsSpec()))), "", "Port"},
		{"HTTP advertise : external Hostnames: port + securePort, no cert", addAdvertiseHttp(addExternalHostnames(removeCert(dualSpec()))), "http://my-endpoint.com:80", ""},

		{"HTTPS advertise: internal          : empty                     ", addAdvertiseHttps(emptySpec()), "", "Port"},
		{"HTTPS advertise: internal          : port                      ", addAdvertiseHttps(httpSpec()), "https://my-endpoint.com:443", ""},
		{"HTTPS advertise: internal          : securePort                ", addAdvertiseHttps(httpsSpec()), "https://my-endpoint.com:443", ""},
		{"HTTPS advertise: internal          : port + securePort         ", addAdvertiseHttps(dualSpec()), "https://my-endpoint.com:443", ""},
		{"HTTPS advertise: internal          : securePort, no cert       ", addAdvertiseHttps(removeCert(httpsSpec())), "", "Port"},
		{"HTTPS advertise: internal          : port + securePort, no cert", addAdvertiseHttps(removeCert(dualSpec())), "https://my-endpoint.com:443", ""},
		{"HTTPS advertise: external IPs      : empty                     ", addAdvertiseHttps(addExternalIPs(emptySpec())), "", "Port"},
		{"HTTPS advertise: external IPs      : port                      ", addAdvertiseHttps(addExternalIPs(httpSpec())), "https://my-endpoint.com:443", ""},
		{"HTTPS advertise: external IPs      : securePort                ", addAdvertiseHttps(addExternalIPs(httpsSpec())), "https://my-endpoint.com:443", ""},
		{"HTTPS advertise: external IPs      : port + securePort         ", addAdvertiseHttps(addExternalIPs(dualSpec())), "https://my-endpoint.com:443", ""},
		{"HTTPS advertise: external IPs      : securePort, no cert       ", addAdvertiseHttps(addExternalIPs(removeCert(httpsSpec()))), "", "Port"},
		{"HTTPS advertise: external IPs      : port + securePort, no cert", addAdvertiseHttps(addExternalIPs(removeCert(dualSpec()))), "https://my-endpoint.com:443", ""},
		{"HTTPS advertise: external Hostnames: empty                     ", addAdvertiseHttps(addExternalHostnames(emptySpec())), "", "Port"},
		{"HTTPS advertise: external Hostnames: port                      ", addAdvertiseHttps(addExternalHostnames(httpSpec())), "https://my-endpoint.com:443", ""},
		{"HTTPS advertise: external Hostnames: securePort                ", addAdvertiseHttps(addExternalHostnames(httpsSpec())), "https://my-endpoint.com:443", ""},
		{"HTTPS advertise: external Hostnames: port + securePort         ", addAdvertiseHttps(addExternalHostnames(dualSpec())), "https://my-endpoint.com:443", ""},
		{"HTTPS advertise: external Hostnames: securePort, no cert       ", addAdvertiseHttps(addExternalHostnames(removeCert(httpsSpec()))), "", "Port"},
		{"HTTPS advertise: external Hostnames: port + securePort, no cert", addAdvertiseHttps(addExternalHostnames(removeCert(dualSpec()))), "https://my-endpoint.com:443", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := tt.store.GetAdvertiseEndpointUrl()
			assert.Equal(t, tt.want, got)
			if tt.wantErrContain != "" {
				assert.ErrorContains(t, err, tt.wantErrContain)
			} else {
				assert.NoError(t, err)
			}
		})

		if tt.store.Spec.Hosting != nil {
			t.Run("with DNS names: "+tt.name, func(t *testing.T) {
				// dnsNames shouldn't change the test result at all
				s := tt.store.DeepCopy()
				s.Spec.Hosting.DNSNames = []string{"should.not.show.up"}
				got, err := s.GetAdvertiseEndpointUrl()
				assert.Equal(t, tt.want, got)
				if tt.wantErrContain != "" {
					assert.ErrorContains(t, err, tt.wantErrContain)
				} else {
					assert.NoError(t, err)
				}
			})
		}
	}
}
