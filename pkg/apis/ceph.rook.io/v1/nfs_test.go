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
	"testing"

	"github.com/stretchr/testify/assert"
	v1 "k8s.io/api/core/v1"
)

func TestNFSSecuritySpec_Validate(t *testing.T) {
	isFailing := true
	isOkay := false

	withSSSD := func(sssd *SSSDSpec) *NFSSecuritySpec {
		return &NFSSecuritySpec{
			SSSD: sssd,
		}
	}

	configMapVolumeSource := &ConfigFileVolumeSource{
		ConfigMap: &v1.ConfigMapVolumeSource{},
	}

	tests := []struct {
		name     string
		security *NFSSecuritySpec
		wantErr  bool
	}{
		{"security = nil", nil, isOkay},
		{"security empty", &NFSSecuritySpec{}, isOkay},
		{"security.sssd empty", withSSSD(&SSSDSpec{}), isFailing},
		{"security.sssd.sidecar empty",
			withSSSD(&SSSDSpec{
				Sidecar: &SSSDSidecar{},
			}),
			isFailing},
		{"security.sssd.sidecar fully specified",
			withSSSD(&SSSDSpec{
				Sidecar: &SSSDSidecar{
					Image: "myimage",
					SSSDConfigFile: SSSDSidecarConfigFile{
						VolumeSource: configMapVolumeSource,
					},
				},
			}),
			isOkay},
		{"security.sssd.sidecar missing image",
			withSSSD(&SSSDSpec{
				Sidecar: &SSSDSidecar{
					Image: "",
					SSSDConfigFile: SSSDSidecarConfigFile{
						VolumeSource: configMapVolumeSource,
					},
				},
			}),
			isFailing},
		{"security.sssd.sidecar.sssdConfigFile empty",
			withSSSD(&SSSDSpec{
				Sidecar: &SSSDSidecar{
					Image:          "myimage",
					SSSDConfigFile: SSSDSidecarConfigFile{},
				},
			}),
			isOkay},
		{"security.sssd.sidecar.sssdConfigFile.volumeSource empty",
			withSSSD(&SSSDSpec{
				Sidecar: &SSSDSidecar{
					Image: "myimage",
					SSSDConfigFile: SSSDSidecarConfigFile{
						VolumeSource: &ConfigFileVolumeSource{},
					},
				},
			}),
			isFailing},
		{"security.sssd.sidecar.additionalFiles empty",
			withSSSD(&SSSDSpec{
				Sidecar: &SSSDSidecar{
					Image:           "myimage",
					AdditionalFiles: AdditionalVolumeMounts{},
				},
			}),
			isOkay},
		{"security.sssd.sidecar.additionalFiles multiple valid",
			withSSSD(&SSSDSpec{
				Sidecar: &SSSDSidecar{
					Image: "myimage",
					AdditionalFiles: AdditionalVolumeMounts{
						{SubPath: "one", VolumeSource: configMapVolumeSource},
						{SubPath: "two", VolumeSource: configMapVolumeSource},
						{SubPath: "three", VolumeSource: configMapVolumeSource},
					},
				},
			}),
			isOkay},
		{"security.sssd.sidecar.additionalFiles one empty subDir",
			withSSSD(&SSSDSpec{
				Sidecar: &SSSDSidecar{
					Image: "myimage",
					AdditionalFiles: AdditionalVolumeMounts{
						{SubPath: "one", VolumeSource: configMapVolumeSource},
						{SubPath: "", VolumeSource: configMapVolumeSource},
						{SubPath: "three", VolumeSource: configMapVolumeSource},
					},
				},
			}),
			isFailing},
		{"security.sssd.sidecar.additionalFiles duplicate subDirs",
			withSSSD(&SSSDSpec{
				Sidecar: &SSSDSidecar{
					Image: "myimage",
					AdditionalFiles: AdditionalVolumeMounts{
						{SubPath: "one", VolumeSource: configMapVolumeSource},
						{SubPath: "two", VolumeSource: configMapVolumeSource},
						{SubPath: "one", VolumeSource: configMapVolumeSource},
					},
				},
			}),
			isFailing},
		{"security.sssd.sidecar.additionalFiles one vol source empty",
			withSSSD(&SSSDSpec{
				Sidecar: &SSSDSidecar{
					Image: "myimage",
					AdditionalFiles: AdditionalVolumeMounts{
						{SubPath: "one", VolumeSource: configMapVolumeSource},
						{SubPath: "", VolumeSource: &ConfigFileVolumeSource{}},
						{SubPath: "three", VolumeSource: configMapVolumeSource},
					},
				},
			}),
			isFailing},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := tt.security.Validate(); (err != nil) != tt.wantErr {
				t.Errorf("NFSSecuritySpec.Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestNFSSecuritySpec_KerberosEnabled(t *testing.T) {
	t.Run("nil security spec", func(t *testing.T) {
		var sec *NFSSecuritySpec
		assert.False(t, sec.KerberosEnabled())
	})

	t.Run("empty security spec", func(t *testing.T) {
		sec := &NFSSecuritySpec{}
		assert.False(t, sec.KerberosEnabled())
	})

	t.Run("empty kerberos spec", func(t *testing.T) {
		sec := &NFSSecuritySpec{
			Kerberos: &KerberosSpec{},
		}
		assert.True(t, sec.KerberosEnabled())
	})

	t.Run("filled in kerberos spec", func(t *testing.T) {
		sec := &NFSSecuritySpec{
			Kerberos: &KerberosSpec{
				PrincipalName: "mom",
			},
		}
		assert.True(t, sec.KerberosEnabled())
	})
}

func TestKerberosSpec_GetPrincipalName(t *testing.T) {
	t.Run("empty kerberos spec", func(t *testing.T) {
		k := &KerberosSpec{}
		assert.Equal(t, "nfs", k.GetPrincipalName())
	})

	t.Run("principal name nfs", func(t *testing.T) {
		k := &KerberosSpec{
			PrincipalName: "nfs",
		}
		assert.Equal(t, "nfs", k.GetPrincipalName())
	})

	t.Run("principal name set", func(t *testing.T) {
		k := &KerberosSpec{
			PrincipalName: "set",
		}
		assert.Equal(t, "set", k.GetPrincipalName())
	})
}
