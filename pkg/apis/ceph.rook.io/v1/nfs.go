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
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"
)

// KerberosEnabled returns true if Kerberos is enabled from the spec.
func (n *NFSSecuritySpec) KerberosEnabled() bool {
	if n == nil {
		return false
	}
	if n.Kerberos != nil {
		return true
	}
	return false
}

// GetPrincipalName gets the principal name for the Kerberos spec or the default value if it is unset.
func (k *KerberosSpec) GetPrincipalName() string {
	if k.PrincipalName == "" {
		return "nfs"
	}
	return k.PrincipalName
}

func (n *CephNFS) IsHostNetwork(c *ClusterSpec) bool {
	if n.Spec.Server.HostNetwork != nil {
		return *n.Spec.Server.HostNetwork
	}
	return c.Network.IsHost()
}

func (n *CephNFS) ValidateCreate() (admission.Warnings, error) {
	return n.Spec.Security.Validate()
}

func (n *CephNFS) ValidateUpdate(old runtime.Object) (admission.Warnings, error) {
	return n.ValidateCreate()
}

func (n *CephNFS) ValidateDelete() error {
	return nil
}

func (sec *NFSSecuritySpec) Validate() (admission.Warnings, error) {
	if sec == nil {
		return nil, nil
	}

	if sec.SSSD != nil {
		sidecar := sec.SSSD.Sidecar
		if sidecar == nil {
			return nil, errors.New("System Security Services Daemon (SSSD) is enabled, but no runtime option is specified; supported: [runInSidecar]")
		}

		if sidecar.Image == "" {
			return nil, errors.New("System Security Services Daemon (SSSD) sidecar is enabled, but no image is specified")
		}

		if volSourceExistsAndIsEmpty(sidecar.SSSDConfigFile.VolumeSource.ToKubernetesVolumeSource()) {
			return nil, errors.New("System Security Services Daemon (SSSD) sidecar is enabled with config from a VolumeSource, but no source is specified")
		}

		subDirs := map[string]bool{}
		for _, additionalFile := range sidecar.AdditionalFiles {
			subDir := additionalFile.SubPath
			if subDir == "" {
				return nil, errors.New("System Security Services Daemon (SSSD) sidecar is enabled with additional file having no subPath specified")
			}

			if volSourceExistsAndIsEmpty(additionalFile.VolumeSource.ToKubernetesVolumeSource()) {
				return nil, errors.Errorf("System Security Services Daemon (SSSD) sidecar is enabled with additional file (subPath %q), but no source is specified", subDir)
			}

			if _, ok := subDirs[subDir]; ok {
				return nil, errors.Errorf("System Security Services Daemon (SSSD) sidecar is enabled with additional file containing duplicate subPath %q", subDir)
			}
			subDirs[subDir] = true
		}
	}

	krb := sec.Kerberos
	if krb != nil {
		if volSourceExistsAndIsEmpty(krb.ConfigFiles.VolumeSource.ToKubernetesVolumeSource()) {
			return nil, errors.New("Kerberos is enabled with config from a VolumeSource, but no source is specified")
		}

		if volSourceExistsAndIsEmpty(krb.KeytabFile.VolumeSource.ToKubernetesVolumeSource()) {
			return nil, errors.New("Kerberos is enabled with keytab from a VolumeSource, but no source is specified")
		}
	}

	return nil, nil
}

func volSourceExistsAndIsEmpty(v *v1.VolumeSource) bool {
	return v != nil && reflect.DeepEqual(*v, v1.VolumeSource{})
}
