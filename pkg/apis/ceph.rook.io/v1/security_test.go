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

import "testing"

func TestKeyManagementServiceSpec_IsK8sAuthEnabled(t *testing.T) {
	type fields struct {
		ConnectionDetails map[string]string
		TokenSecretName   string
	}
	tests := []struct {
		name   string
		fields fields
		want   bool
	}{
		{"k8s auth is disabled - everything is empty", fields{ConnectionDetails: map[string]string{}, TokenSecretName: ""}, false},
		{"k8s auth is disabled - token is populated", fields{ConnectionDetails: map[string]string{}, TokenSecretName: "foo"}, false},
		{"k8s auth is disabled since token is provided", fields{ConnectionDetails: map[string]string{"VAULT_AUTH_METHOD": "kubernetes"}, TokenSecretName: "rook-ceph-test-secret"}, false},
		{"k8s auth is disabled since VAULT_AUTH_METHOD is unknown", fields{ConnectionDetails: map[string]string{"VAULT_AUTH_METHOD": "foo"}, TokenSecretName: ""}, false},
		{"k8s auth is enabled", fields{ConnectionDetails: map[string]string{"VAULT_AUTH_METHOD": "kubernetes"}, TokenSecretName: ""}, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			kms := &KeyManagementServiceSpec{
				ConnectionDetails: tt.fields.ConnectionDetails,
				TokenSecretName:   tt.fields.TokenSecretName,
			}
			if got := kms.IsK8sAuthEnabled(); got != tt.want {
				t.Errorf("KeyManagementServiceSpec.IsK8sAuthEnabled() = %v, want %v", got, tt.want)
			}
		})
	}
}
