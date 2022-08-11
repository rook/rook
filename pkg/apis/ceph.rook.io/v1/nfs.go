/*
Copyright 2022 The Rook Authors. All rights reserved.

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
	"reflect"

	"github.com/pkg/errors"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
)

func (n *CephNFS) ValidateCreate() error {
	return n.Spec.Security.Validate()
}

func (n *CephNFS) ValidateUpdate(old runtime.Object) error {
	return n.ValidateCreate()
}

func (n *CephNFS) ValidateDelete() error {
	return nil
}

func (sec *NFSSecuritySpec) Validate() error {
	if sec == nil {
		return nil
	}

	if sec.SSSD != nil {
		sidecar := sec.SSSD.Sidecar
		if sidecar == nil {
			return errors.New("System Security Services Daemon (SSSD) is enabled, but no runtime option is specified; supported: [runInSidecar]")
		}

		if sidecar.Image == "" {
			return errors.New("System Security Services Daemon (SSSD) sidecar is enabled, but no image is specified")
		}

		volSource := sidecar.SSSDConfigFile.VolumeSource
		if volSource != nil && reflect.DeepEqual(*volSource, v1.VolumeSource{}) {
			return errors.New("System Security Services Daemon (SSSD) sidecar is enabled with config from a VolumeSource, but no source is specified")
		}
	}

	return nil
}
