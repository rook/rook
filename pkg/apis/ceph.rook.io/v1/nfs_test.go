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
						VolumeSource: &v1.VolumeSource{
							ConfigMap: &v1.ConfigMapVolumeSource{},
						},
					},
				},
			}),
			isOkay},
		{"security.sssd.sidecar missing image",
			withSSSD(&SSSDSpec{
				Sidecar: &SSSDSidecar{
					Image: "",
					SSSDConfigFile: SSSDSidecarConfigFile{
						VolumeSource: &v1.VolumeSource{
							ConfigMap: &v1.ConfigMapVolumeSource{},
						},
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
						VolumeSource: &v1.VolumeSource{},
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
