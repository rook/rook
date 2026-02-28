/*
Copyright 2018 The Rook Authors. All rights reserved.

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
	"fmt"

	"github.com/pkg/errors"
	"k8s.io/apimachinery/pkg/util/validation"
)

const ServiceServingCertKey = "service.beta.openshift.io/serving-cert-secret-name"

// 38 is the max length of a ceph store name as total length of the resource name cannot be more than 63 characters limit
// and there is a configmap which is formed by appending `rook-ceph-rgw-<STORE-NAME>-mime-types`
// so over all it brings up to (63-14-11 = 38) characters for the store name
const objectStoreNameMaxLen = 38

func (s *ObjectStoreSpec) IsMultisite() bool {
	return s.Zone.Name != ""
}

func (s *ObjectStoreSpec) IsTLSEnabled() bool {
	return s.Gateway.SecurePort != 0 && (s.Gateway.SSLCertificateRef != "" || s.GetServiceServingCert() != "")
}

func (s *ObjectStoreSpec) IsRGWDashboardEnabled() bool {
	return s.Gateway.DashboardEnabled == nil || *s.Gateway.DashboardEnabled
}

func (s *ObjectStoreSpec) GetPort() (int32, error) {
	if s.IsTLSEnabled() {
		return s.Gateway.SecurePort, nil
	} else if s.Gateway.Port != 0 {
		return s.Gateway.Port, nil
	}
	return -1, errors.New("At least one of Port or SecurePort should be non-zero")
}

func (s *ObjectStoreSpec) IsExternal() bool {
	return len(s.Gateway.ExternalRgwEndpoints) != 0
}

func (s *ObjectStoreSpec) IsHostNetwork(c *ClusterSpec) bool {
	if s.Gateway.HostNetwork != nil {
		return *s.Gateway.HostNetwork
	}
	return c.Network.IsHost()
}

func (s *ObjectRealmSpec) IsPullRealm() bool {
	return s.Pull.Endpoint != ""
}

// ValidateObjectSpec validate the object store arguments
func ValidateObjectSpec(gs *CephObjectStore) error {
	if gs.Name == "" {
		return errors.New("missing name")
	}
	if gs.Namespace == "" {
		return errors.New("missing namespace")
	}

	// validate the object store name only if it is not an external cluster
	// as external cluster won't create the rgw daemon and it's other resources
	// and there is some legacy external cluster which has more length of objectstore
	// so to run them successfully we are not validating the objectstore name
	if !gs.Spec.IsExternal() {
		if len(gs.Name) > objectStoreNameMaxLen {
			return errors.New("object store name cannot be longer than 38 characters")
		}
	}
	securePort := gs.Spec.Gateway.SecurePort
	if securePort < 0 || securePort > 65535 {
		return errors.Errorf("securePort value of %d must be between 0 and 65535", securePort)
	}
	if gs.Spec.Gateway.Port <= 0 && gs.Spec.Gateway.SecurePort <= 0 {
		return errors.New("invalid create: either of port or securePort fields should be not be zero")
	}

	// check hosting spec
	if gs.Spec.Hosting != nil {
		if gs.Spec.Hosting.AdvertiseEndpoint != nil {
			ep := gs.Spec.Hosting.AdvertiseEndpoint
			errList := validation.IsDNS1123Subdomain(ep.DnsName)
			if len(errList) > 0 {
				return errors.Errorf("hosting.advertiseEndpoint.dnsName %q must be a valid DNS-1123 subdomain: %v", ep.DnsName, errList)
			}
			if ep.Port < 1 || ep.Port > 65535 {
				return errors.Errorf("hosting.advertiseEndpoint.port %d must be between 1 and 65535", ep.Port)
			}
		}
		dnsNameErrs := []string{}
		for _, dnsName := range gs.Spec.Hosting.DNSNames {
			errs := validation.IsDNS1123Subdomain(dnsName)
			if len(errs) > 0 {
				// errors do not report the domains that are errored; add them to help users
				errs = append(errs, fmt.Sprintf("error on dns name %q", dnsName))
				dnsNameErrs = append(dnsNameErrs, errs...)
			}
		}
		if len(dnsNameErrs) > 0 {
			return errors.Errorf("one or more hosting.dnsNames is not a valid DNS-1123 subdomain: %v", dnsNameErrs)
		}
	}

	return nil
}

func (s *ObjectStoreSpec) GetServiceServingCert() string {
	if s.Gateway.Service != nil {
		return s.Gateway.Service.Annotations[ServiceServingCertKey]
	}
	return ""
}

// GetServiceName gets the name of the Rook-created CephObjectStore service.
// This method helps ensure adherence to stable, documented behavior (API).
func (c *CephObjectStore) GetServiceName() string {
	return "rook-ceph-rgw-" + c.GetName()
}

// GetServiceDomainName gets the domain name of the Rook-created CephObjectStore service.
// This method helps ensure adherence to stable, documented behavior (API).
func (c *CephObjectStore) GetServiceDomainName() string {
	return fmt.Sprintf("%s.%s.svc", c.GetServiceName(), c.GetNamespace())
}

func (c *CephObjectStore) AdvertiseEndpointIsSet() bool {
	return c.Spec.Hosting != nil && c.Spec.Hosting.AdvertiseEndpoint != nil &&
		c.Spec.Hosting.AdvertiseEndpoint.DnsName != "" && c.Spec.Hosting.AdvertiseEndpoint.Port != 0
}

// GetAdvertiseEndpoint returns address, port, and isTls information about the advertised endpoint
// for the CephObjectStore. This method helps ensure adherence to stable, documented behavior (API).
func (c *CephObjectStore) GetAdvertiseEndpoint() (string, int32, bool, error) {
	port, err := c.Spec.GetPort()
	if err != nil {
		return "", 0, false, err
	}
	isTls := c.Spec.IsTLSEnabled()

	address := c.GetServiceDomainName() // service domain name is the default advertise address
	if c.Spec.IsExternal() {
		// for external clusters, the first external RGW endpoint is the default advertise address
		address = c.Spec.Gateway.ExternalRgwEndpoints[0].String()
	}

	// if users override the advertise endpoint themselves, these value take priority
	if c.AdvertiseEndpointIsSet() {
		address = c.Spec.Hosting.AdvertiseEndpoint.DnsName
		port = c.Spec.Hosting.AdvertiseEndpoint.Port
		isTls = c.Spec.Hosting.AdvertiseEndpoint.UseTls
	}

	return address, port, isTls, nil
}

// GetAdvertiseEndpointUrl gets the fully-formed advertised endpoint URL for the CephObjectStore.
// This method helps ensure adherence to stable, documented behavior (API).
func (c *CephObjectStore) GetAdvertiseEndpointUrl() (string, error) {
	address, port, isTls, err := c.GetAdvertiseEndpoint()
	if err != nil {
		return "", err
	}

	protocol := "http"
	if isTls {
		protocol = "https"
	}
	return fmt.Sprintf("%s://%s:%d", protocol, address, port), nil
}

func (c *CephObjectStore) GetStatusConditions() *[]Condition {
	return &c.Status.Conditions
}

func (z *CephObjectZone) GetStatusConditions() *[]Condition {
	return &z.Status.Conditions
}

// String returns an addressable string representation of the EndpointAddress.
func (e *EndpointAddress) String() string {
	// hostname is easier to read, and it is probably less likely to change, so prefer it over IP
	if e.Hostname != "" {
		return e.Hostname
	}
	return e.IP
}
