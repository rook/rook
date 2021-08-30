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

package kms

import (
	"testing"

	kv "github.com/hashicorp/vault-plugin-secrets-kv"
	"github.com/hashicorp/vault/api"
	vaulthttp "github.com/hashicorp/vault/http"
	"github.com/hashicorp/vault/sdk/logical"
	"github.com/hashicorp/vault/vault"
)

func TestBackendVersion(t *testing.T) {
	cluster := fakeVaultServer(t)
	cluster.Start()
	defer cluster.Cleanup()
	core := cluster.Cores[0].Core
	vault.TestWaitActive(t, core)
	client := cluster.Cores[0].Client

	// Mock the client here
	vaultClient = func(secretConfig map[string]string) (*api.Client, error) { return client, nil }

	// Set up the kv store
	if err := client.Sys().Mount("rook/", &api.MountInput{
		Type:    "kv",
		Options: map[string]string{"version": "1"},
	}); err != nil {
		t.Fatal(err)
	}
	if err := client.Sys().Mount("rookv2/", &api.MountInput{
		Type:    "kv-v2",
		Options: map[string]string{"version": "2"},
	}); err != nil {
		t.Fatal(err)
	}

	type args struct {
		secretConfig map[string]string
	}
	tests := []struct {
		name    string
		args    args
		want    string
		wantErr bool
	}{
		{"v1 is set explicitly", args{map[string]string{"VAULT_BACKEND": "v1"}}, "v1", false},
		{"v2 is set explicitly", args{map[string]string{"VAULT_BACKEND": "v2"}}, "v2", false},
		{"v1 is set auto-discovered", args{map[string]string{"VAULT_ADDR": client.Address(), "VAULT_BACKEND_PATH": "rook"}}, "v1", false},
		{"v2 is set auto-discovered", args{map[string]string{"VAULT_ADDR": client.Address(), "VAULT_BACKEND_PATH": "rookv2"}}, "v2", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := BackendVersion(tt.args.secretConfig)
			if (err != nil) != tt.wantErr {
				t.Errorf("BackendVersion() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if got != tt.want {
				t.Errorf("BackendVersion() = %v, want %v", got, tt.want)
			}
		})
	}
}

func fakeVaultServer(t *testing.T) *vault.TestCluster {
	cluster := vault.NewTestCluster(t, &vault.CoreConfig{
		DevToken:        "token",
		LogicalBackends: map[string]logical.Factory{"kv": kv.Factory},
	},
		&vault.TestClusterOptions{
			HandlerFunc: vaulthttp.Handler,
			NumCores:    1,
		})

	return cluster
}
