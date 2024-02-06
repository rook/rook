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
	"github.com/pkg/errors"
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
	return nil
}

func (s *ObjectStoreSpec) GetServiceServingCert() string {
	if s.Gateway.Service != nil {
		return s.Gateway.Service.Annotations[ServiceServingCertKey]
	}
	return ""
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
