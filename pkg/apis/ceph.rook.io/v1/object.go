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
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/webhook"
)

// compile-time assertions ensures CephObjectStore implements webhook.Validator so a webhook builder
// will be registered for the validating webhook.
var _ webhook.Validator = &CephObjectStore{}

const ServiceServingCertKey = "service.beta.openshift.io/serving-cert-secret-name"

func (s *ObjectStoreSpec) IsMultisite() bool {
	return s.Zone.Name != ""
}

func (s *ObjectStoreSpec) IsTLSEnabled() bool {
	return s.Gateway.SecurePort != 0 && (s.Gateway.SSLCertificateRef != "" || s.GetServiceServingCert() != "")
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

func (s *ObjectRealmSpec) IsPullRealm() bool {
	return s.Pull.Endpoint != ""
}

func (o *CephObjectStore) ValidateCreate() error {
	logger.Infof("validate create cephobjectstore %v", o)
	if err := ValidateObjectSpec(o); err != nil {
		return err
	}
	return nil
}

// ValidateObjectSpec validate the object store arguments
func ValidateObjectSpec(gs *CephObjectStore) error {
	if gs.Name == "" {
		return errors.New("missing name")
	}
	if gs.Namespace == "" {
		return errors.New("missing namespace")
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

func (o *CephObjectStore) ValidateUpdate(old runtime.Object) error {
	logger.Info("validate update cephobjectstore")
	err := ValidateObjectSpec(o)
	if err != nil {
		return err
	}
	return nil
}

func (o *CephObjectStore) ValidateDelete() error {
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
